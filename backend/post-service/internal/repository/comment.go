package repository

import (
	"context"
	"errors"

	"post-service/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ domain.CommentRepository = (*CommentRepo)(nil)

// selectComment — общая проекция комментария. liked считается подзапросом
// EXISTS относительно зрителя ($1 во всех запросах ниже), поэтому порядок
// параметров везде начинается с viewerID — как в selectPost.
const selectComment = `SELECT c.id, c.post_id, c.author_id, c.text, c.likes_count, c.created_at, c.updated_at,
                              EXISTS (SELECT 1 FROM comment_likes l WHERE l.comment_id = c.id AND l.user_id = $1) AS liked
                       FROM comments c `

type CommentRepo struct {
	pool *pgxpool.Pool
}

func NewComment(pool *pgxpool.Pool) *CommentRepo {
	return &CommentRepo{
		pool: pool,
	}
}

// Create вставляет комментарий и инкрементит posts.comments_count в той же
// транзакции: иначе счётчик разъедется с реальностью на первой же ошибке
// между двумя запросами, и чинить это будет нечем.
//
// Комментарий к несуществующему посту ловится FK (comments_post_id_fkey) —
// отдельный SELECT для проверки существования не нужен и был бы гонкой.
func (r *CommentRepo) Create(ctx context.Context, comment *domain.Comment) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback после успешного Commit — no-op, поэтому defer безопасен.
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`INSERT INTO comments (id, post_id, author_id, text, likes_count, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		comment.ID, comment.PostID, comment.AuthorID, comment.Text,
		comment.LikesCount, comment.CreatedAt, comment.UpdatedAt)
	if err != nil {
		return mapCommentErr(err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE posts SET comments_count = comments_count + 1 WHERE id = $1`, comment.PostID); err != nil {
		return mapCommentErr(err)
	}

	return mapCommentErr(tx.Commit(ctx))
}

// ListByPost — keyset-пагинация ПО ВОЗРАСТАНИЮ: сравнение кортежей `>`
// и ORDER BY без DESC. Направление обратно ленте, потому что обсуждение
// читается сверху вниз; индекс comments_post_idx построен под этот порядок.
func (r *CommentRepo) ListByPost(ctx context.Context, postID, viewerID uuid.UUID, cursor *domain.FeedCursor, limit int) ([]domain.Comment, error) {
	var (
		rows pgx.Rows
		err  error
	)

	if cursor == nil {
		rows, err = r.pool.Query(ctx,
			selectComment+`WHERE c.post_id = $2
			               ORDER BY c.created_at, c.id LIMIT $3`,
			viewerID, postID, limit)
	} else {
		rows, err = r.pool.Query(ctx,
			selectComment+`WHERE c.post_id = $2 AND (c.created_at, c.id) > ($3, $4)
			               ORDER BY c.created_at, c.id LIMIT $5`,
			viewerID, postID, cursor.CreatedAt, cursor.ID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	comments := make([]domain.Comment, 0, limit)
	for rows.Next() {
		var c domain.Comment
		if err := rows.Scan(&c.ID, &c.PostID, &c.AuthorID, &c.Text, &c.LikesCount,
			&c.CreatedAt, &c.UpdatedAt, &c.Liked); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// Like идемпотентен: повторный лайк — не ошибка и не +1.
func (r *CommentRepo) Like(ctx context.Context, commentID, userID uuid.UUID) (*domain.Comment, error) {
	return r.toggle(ctx, commentID, userID,
		`INSERT INTO comment_likes (comment_id, user_id, created_at) VALUES ($1, $2, now())
		 ON CONFLICT (comment_id, user_id) DO NOTHING`,
		`UPDATE comments SET likes_count = likes_count + 1 WHERE id = $1`)
}

// Unlike идемпотентен так же: снятие несуществующего лайка счётчик не трогает.
func (r *CommentRepo) Unlike(ctx context.Context, commentID, userID uuid.UUID) (*domain.Comment, error) {
	return r.toggle(ctx, commentID, userID,
		`DELETE FROM comment_likes WHERE comment_id = $1 AND user_id = $2`,
		`UPDATE comments SET likes_count = likes_count - 1 WHERE id = $1`)
}

// toggle — общий каркас лайка и анлайка: обе операции отличаются только
// первым запросом, а вся тонкая часть (счётчик в той же транзакции, чтение
// свежей сущности до Commit) у них одна и та же.
func (r *CommentRepo) toggle(ctx context.Context, commentID, userID uuid.UUID, mutate, counter string) (*domain.Comment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, mutate, commentID, userID)
	if err != nil {
		return nil, mapCommentErr(err)
	}

	if tag.RowsAffected() == 1 {
		if _, err := tx.Exec(ctx, counter, commentID); err != nil {
			return nil, mapCommentErr(err)
		}
	}

	comment, err := scanComment(tx.QueryRow(ctx, selectComment+`WHERE c.id = $2`, userID, commentID))
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, mapCommentErr(err)
	}
	return comment, nil
}

func scanComment(row pgx.Row) (*domain.Comment, error) {
	var c domain.Comment

	err := row.Scan(&c.ID, &c.PostID, &c.AuthorID, &c.Text, &c.LikesCount,
		&c.CreatedAt, &c.UpdatedAt, &c.Liked)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// mapCommentErr переводит нарушения FK в ErrNotFound: комментарий к
// несуществующему посту и лайк несуществующего комментария — это «не
// найдено», а не внутренняя ошибка. Сырые ошибки БД наружу не отдаются.
func mapCommentErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation {
		switch pgErr.ConstraintName {
		case "comments_post_id_fkey", "comment_likes_comment_id_fkey":
			return domain.ErrNotFound
		}
	}
	return err
}
