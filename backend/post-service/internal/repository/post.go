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

var _ domain.PostRepository = (*PostRepo)(nil)

// selectPost — общая проекция поста. liked считается подзапросом EXISTS
// относительно зрителя ($1 во всех запросах ниже), поэтому порядок
// параметров везде начинается с viewerID.
const selectPost = `SELECT p.id, p.author_id, p.text, p.likes_count, p.comments_count, p.created_at, p.updated_at,
                           EXISTS (SELECT 1 FROM post_likes l WHERE l.post_id = p.id AND l.user_id = $1) AS liked
                    FROM posts p `

type PostRepo struct {
	pool *pgxpool.Pool
}

func NewPost(pool *pgxpool.Pool) *PostRepo {
	return &PostRepo{
		pool: pool,
	}
}

// Create вставляет готовую сущность: id, время и валидация — забота
// сервиса (как в user-service).
func (r *PostRepo) Create(ctx context.Context, post *domain.Post) error {
	query := `INSERT INTO posts (id, author_id, text, likes_count, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.pool.Exec(ctx, query,
		post.ID, post.AuthorID, post.Text, post.LikesCount, post.CreatedAt, post.UpdatedAt)
	if err != nil {
		return mapPostErr(err)
	}
	return nil
}

func (r *PostRepo) GetByID(ctx context.Context, id, viewerID uuid.UUID) (*domain.Post, error) {
	return scanPost(r.pool.QueryRow(ctx, selectPost+`WHERE p.id = $2`, viewerID, id))
}

func (r *PostRepo) ListFeed(ctx context.Context, viewerID uuid.UUID, cursor *domain.FeedCursor, limit int) ([]domain.Post, error) {
	var (
		rows pgx.Rows
		err  error
	)

	if cursor == nil {
		rows, err = r.pool.Query(ctx,
			selectPost+`ORDER BY p.created_at DESC, p.id DESC LIMIT $2`,
			viewerID, limit)
	} else {
		// Сравнение кортежей, а не двух отдельных условий: оно совпадает с
		// ORDER BY один в один и умеет пользоваться индексом posts_feed_idx.
		// Через OR-условия на постах с одинаковым created_at страница
		// теряла бы записи.
		rows, err = r.pool.Query(ctx,
			selectPost+`WHERE (p.created_at, p.id) < ($2, $3)
			            ORDER BY p.created_at DESC, p.id DESC LIMIT $4`,
			viewerID, cursor.CreatedAt, cursor.ID, limit)
	}
	if err != nil {
		return nil, err
	}
	return scanPosts(rows, limit)
}

// ListByAuthor — лента одного автора. Условие по author_id идёт ПЕРВЫМ, под
// индекс posts_author_idx (author_id, created_at DESC, id DESC): сначала
// равенство, потом ключ сортировки.
func (r *PostRepo) ListByAuthor(ctx context.Context, authorID, viewerID uuid.UUID, cursor *domain.FeedCursor, limit int) ([]domain.Post, error) {
	var (
		rows pgx.Rows
		err  error
	)

	if cursor == nil {
		rows, err = r.pool.Query(ctx,
			selectPost+`WHERE p.author_id = $2
			            ORDER BY p.created_at DESC, p.id DESC LIMIT $3`,
			viewerID, authorID, limit)
	} else {
		rows, err = r.pool.Query(ctx,
			selectPost+`WHERE p.author_id = $2 AND (p.created_at, p.id) < ($3, $4)
			            ORDER BY p.created_at DESC, p.id DESC LIMIT $5`,
			viewerID, authorID, cursor.CreatedAt, cursor.ID, limit)
	}
	if err != nil {
		return nil, err
	}
	return scanPosts(rows, limit)
}

// scanPosts — общий разбор выборки постов. Вынесен, потому что проекция
// selectPost одна на все списки: разъехавшийся порядок Scan в двух копиях
// ловился бы только в рантайме.
func scanPosts(rows pgx.Rows, limit int) ([]domain.Post, error) {
	defer rows.Close()

	posts := make([]domain.Post, 0, limit)
	for rows.Next() {
		var post domain.Post
		if err := rows.Scan(&post.ID, &post.AuthorID, &post.Text, &post.LikesCount, &post.CommentsCount,
			&post.CreatedAt, &post.UpdatedAt, &post.Liked); err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	return posts, rows.Err()
}

// Like идемпотентен: повторный лайк — не ошибка и не +1. Счётчик двигается
// в той же транзакции, что и вставка в post_likes, иначе они разъедутся
// на первой же ошибке между ними.
func (r *PostRepo) Like(ctx context.Context, postID, userID uuid.UUID) (*domain.Post, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	// Rollback после успешного Commit — no-op, поэтому defer безопасен.
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`INSERT INTO post_likes (post_id, user_id, created_at) VALUES ($1, $2, now())
		 ON CONFLICT (post_id, user_id) DO NOTHING`, postID, userID)
	if err != nil {
		// Лайк несуществующего поста ловится FK, отдельный SELECT для
		// проверки существования не нужен (и был бы гонкой).
		return nil, mapPostErr(err)
	}

	if tag.RowsAffected() == 1 {
		if _, err := tx.Exec(ctx,
			`UPDATE posts SET likes_count = likes_count + 1 WHERE id = $1`, postID); err != nil {
			return nil, mapPostErr(err)
		}
	}

	post, err := scanPost(tx.QueryRow(ctx, selectPost+`WHERE p.id = $2`, userID, postID))
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, mapPostErr(err)
	}
	return post, nil
}

// Unlike идемпотентен так же: снятие несуществующего лайка счётчик не
// трогает. Несуществующий пост даёт ErrNotFound на финальном SELECT.
func (r *PostRepo) Unlike(ctx context.Context, postID, userID uuid.UUID) (*domain.Post, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`DELETE FROM post_likes WHERE post_id = $1 AND user_id = $2`, postID, userID)
	if err != nil {
		return nil, mapPostErr(err)
	}

	if tag.RowsAffected() == 1 {
		if _, err := tx.Exec(ctx,
			`UPDATE posts SET likes_count = likes_count - 1 WHERE id = $1`, postID); err != nil {
			return nil, mapPostErr(err)
		}
	}

	post, err := scanPost(tx.QueryRow(ctx, selectPost+`WHERE p.id = $2`, userID, postID))
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, mapPostErr(err)
	}
	return post, nil
}

func scanPost(row pgx.Row) (*domain.Post, error) {
	var post domain.Post

	err := row.Scan(&post.ID, &post.AuthorID, &post.Text, &post.LikesCount, &post.CommentsCount,
		&post.CreatedAt, &post.UpdatedAt, &post.Liked)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &post, nil
}

// mapPostErr — единая точка перевода pgx-ошибок в доменные на границе
// репозитория: сырые ошибки БД наружу не отдаются ни с одного мутирующего
// пути. Сейчас разбирается только один случай — нарушение FK
// post_likes_post_id_fkey (лайк несуществующего поста) → ErrNotFound.
// Нарушение CHECK likes_count >= 0 пока провалится в общий return err
// (нет подходящей доменной ошибки под нарушение инварианта счётчика —
// заводить её сейчас было бы преждевременно, вне рамок этой задачи).
func mapPostErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation {
		if pgErr.ConstraintName == "post_likes_post_id_fkey" {
			return domain.ErrNotFound
		}
	}
	return err
}
