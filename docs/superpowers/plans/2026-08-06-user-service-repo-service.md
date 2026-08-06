# user-service: слой repository + service — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Реализовать pgx-репозиторий `UserRepository` и сервисный слой user-service (`CreateProfile`/`GetByID`/`GetByHandle`/`UpdateProfile`), проверенные на живой БД, плюс установить `buf` для будущего gRPC-шага.

**Architecture:** Слоистая архитектура как в auth-service: `domain` (сущности + порты-интерфейсы) → `repository` (pgx поверх `*pgxpool.Pool`, транслирует ошибки БД в доменные) → `service` (генерация id/времени/стаб-хендла, бизнес-логика). Репозиторий «тупой» — вставляет готовую сущность; `service` ставит `time.Now()` и генерит хендл. Транспорт (gRPC) и consumer NATS — вне этого плана.

**Tech Stack:** Go 1.26.3, workspace-модули (`backend/go.work`), `jackc/pgx/v5`, `jackc/pgerrcode`, `google/uuid`, Postgres 16 (`user_db` на `localhost:5436`), goose (миграции уже прогнаны).

## Global Constraints

- **Git — только пользователь.** Claude НЕ выполняет `git add/commit/branch/push` даже по просьбе. Шаги «Commit» в этом плане — это подсказка пользователю с готовым текстом сообщения, а не действие агента.
- Версии зависимостей берём **ровно те же**, что в `backend/auth-service/go.mod`, чтобы workspace не ловил конфликт: `github.com/jackc/pgx/v5 v5.10.0`, `github.com/google/uuid v1.6.0`, `github.com/jackc/pgerrcode v0.0.0-20250907135507-afb5586c32a6`.
- Конструкторы репозиториев возвращают **конкретный тип** (`*UserRepo`), интерфейс объявлен в `domain` («return structs, accept interfaces»). Обязателен compile-time ассерт `var _ domain.UserRepository = (*UserRepo)(nil)`.
- Отзыв/удаление данных — вне объёма. Никаких `Delete` в репозитории.
- Валидацию полей (длины `name`/`bio`, формат `handle`) в этом плане **не делаем** — она ляжет на transport-слой, как `validate()` в `auth-service/internal/transport/web`.
- Все команды выполняются в PowerShell на Windows. `\` не работает как перенос строки — команды пишем одной строкой.
- DB URL для user_db: `postgres://user:user@localhost:5436/user_db?sslmode=disable`

## Файловая структура

| Файл | Ответственность | Действие |
|---|---|---|
| `backend/user-service/go.mod` | зависимости модуля | Modify |
| `backend/user-service/internal/domain/user.go` | сущность `User` | Modify (типы времени) |
| `backend/user-service/internal/repository/user.go` | pgx-реализация `UserRepository`, трансляция ошибок БД | Create |
| `backend/user-service/internal/service/user.go` | `UserService`: стаб-хендл, времена, вызовы repo | Create |
| `backend/user-service/cmd/devcheck/main.go` | **временный** прогон repo+service по живой БД | Create, затем Delete |

Почему `devcheck` внутри модуля, а не в scratchpad: Go запрещает импорт `internal/...` пакетов извне модуля, поэтому проверочный код обязан жить в дереве `user-service`. Он удаляется в Task 5 и до коммита не доживает.

---

### Task 1: Зависимости модуля + типы времени в domain

**Files:**
- Modify: `backend/user-service/go.mod`
- Modify: `backend/user-service/internal/domain/user.go:1-16`

**Interfaces:**
- Consumes: ничего (первая задача).
- Produces: `domain.User` с полями `CreatedAt time.Time`, `UpdatedAt time.Time` — на них опираются Task 2 (scan из TIMESTAMPTZ) и Task 3 (`time.Now()`).

- [ ] **Step 1: Добавить зависимости**

В PowerShell из `D:\social_network_golang\backend\user-service`:

```powershell
go get github.com/jackc/pgx/v5@v5.10.0
go get github.com/google/uuid@v1.6.0
go get github.com/jackc/pgerrcode@v0.0.0-20250907135507-afb5586c32a6
```

- [ ] **Step 2: Поменять типы времени в domain.User**

Заменить содержимое `backend/user-service/internal/domain/user.go`:

```go
package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID
	Name      string
	Handle    string
	Bio       string
	City      string
	Work      string
	Verified  bool
	AvatarURL *string
	CreatedAt time.Time
	UpdatedAt time.Time
}
```

Почему: колонки `created_at`/`updated_at` в миграции — `TIMESTAMPTZ`, pgx сканирует их в `time.Time` без конвертаций. `string` заставил бы парсить руками. В auth-service везде `time.Time`.

- [ ] **Step 3: Проверить, что модуль собирается**

Run: `go build ./...`
Expected: без вывода (успех). Если `go.mod` не подтянул зависимости — `go mod tidy` и повторить.

- [ ] **Step 4: Commit (⚠️ выполняет пользователь)**

```
user-service: add pgx deps, switch User timestamps to time.Time
```

---

### Task 2: pgx-реализация UserRepository

**Files:**
- Create: `backend/user-service/internal/repository/user.go`

**Interfaces:**
- Consumes: `domain.User` (Task 1), `domain.UserRepository`, `domain.ErrNotFound`, `domain.ErrHandleTaken` (уже существуют в `internal/domain`).
- Produces:
  - `repository.NewUser(pool *pgxpool.Pool) *repository.UserRepo`
  - `(*UserRepo).Create(ctx context.Context, user *domain.User) error`
  - `(*UserRepo).Update(ctx context.Context, user *domain.User) error`
  - `(*UserRepo).GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)`
  - `(*UserRepo).GetByHandle(ctx context.Context, handle string) (*domain.User, error)`

- [ ] **Step 1: Написать репозиторий**

Создать `backend/user-service/internal/repository/user.go`:

```go
package repository

import (
	"context"
	"errors"

	"user-service/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ domain.UserRepository = (*UserRepo)(nil)

const userColumns = `id, name, handle, bio, city, work, verified, avatar_url, created_at, updated_at`

type UserRepo struct {
	pool *pgxpool.Pool
}

func NewUser(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{
		pool: pool,
	}
}

// Create идемпотентен: повторная вставка того же id — не ошибка.
// Нужно для at-least-once доставки user.registered из JetStream.
func (u *UserRepo) Create(ctx context.Context, user *domain.User) error {
	query := `INSERT INTO users (` + userColumns + `)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	          ON CONFLICT (id) DO NOTHING`

	_, err := u.pool.Exec(ctx, query,
		user.ID, user.Name, user.Handle, user.Bio, user.City,
		user.Work, user.Verified, user.AvatarURL, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return mapUserErr(err)
	}
	return nil
}

func (u *UserRepo) Update(ctx context.Context, user *domain.User) error {
	query := `UPDATE users SET name = $1, handle = $2, bio = $3, city = $4, work = $5, updated_at = $6 WHERE id = $7`

	tag, err := u.pool.Exec(ctx, query,
		user.Name, user.Handle, user.Bio, user.City, user.Work, user.UpdatedAt, user.ID)
	if err != nil {
		return mapUserErr(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (u *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = $1`
	return scanUser(u.pool.QueryRow(ctx, query, id))
}

func (u *UserRepo) GetByHandle(ctx context.Context, handle string) (*domain.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE handle = $1`
	return scanUser(u.pool.QueryRow(ctx, query, handle))
}

func scanUser(row pgx.Row) (*domain.User, error) {
	var user domain.User

	err := row.Scan(&user.ID, &user.Name, &user.Handle, &user.Bio, &user.City,
		&user.Work, &user.Verified, &user.AvatarURL, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

func mapUserErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		if pgErr.ConstraintName == "users_handle_key" {
			return domain.ErrHandleTaken
		}
	}
	return err
}
```

Ключевые моменты, которые легко сделать неправильно:
- `ON CONFLICT (id) DO NOTHING` гасит конфликт **только по id**. Конфликт по `handle` (другой id, занятый хендл) всё равно вернёт `UniqueViolation` → `ErrHandleTaken`. Это желаемое поведение.
- `Update` меняет **только** `name/handle/bio/city/work/updated_at`. `verified`, `avatar_url`, `created_at` — не трогаем: они меняются другими сценариями (модерация, media-service).
- `RowsAffected() == 0` в `Update` — единственный способ отличить «нет такого id» от успеха, `Exec` в этом случае ошибку не вернёт.
- Имя constraint `users_handle_key` — автогенерённое Postgres для `handle CITEXT UNIQUE` в таблице `users`. В миграции оно не задано явно; если проверка в Task 5 покажет другое имя — поправить строку в `mapUserErr`.

- [ ] **Step 2: Проверить сборку и vet**

Run: `go build ./... ; go vet ./...`
Expected: обе команды без вывода. Ассерт `var _ domain.UserRepository = (*UserRepo)(nil)` не скомпилируется, если сигнатура разошлась с интерфейсом — это и есть проверка соответствия порту.

- [ ] **Step 3: Commit (⚠️ выполняет пользователь)**

```
user-service: add pgx UserRepository implementation
```

---

### Task 3: Сервисный слой

**Files:**
- Create: `backend/user-service/internal/service/user.go`

**Interfaces:**
- Consumes: `domain.UserRepository` (порт, принимаем интерфейс), `domain.User` (Task 1), поведение ошибок из Task 2.
- Produces:
  - `service.NewUserService(repo domain.UserRepository) *service.UserService`
  - `(*UserService).CreateProfile(ctx context.Context, id uuid.UUID) (*domain.User, error)`
  - `(*UserService).GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)`
  - `(*UserService).GetByHandle(ctx context.Context, handle string) (*domain.User, error)`
  - `(*UserService).UpdateProfile(ctx context.Context, user *domain.User) (*domain.User, error)`

  Эти сигнатуры — контракт для будущего NATS-consumer'а (зовёт `CreateProfile`) и gRPC-транспорта (зовёт остальные три).

- [ ] **Step 1: Написать сервис**

Создать `backend/user-service/internal/service/user.go`:

```go
package service

import (
	"context"
	"time"

	"user-service/internal/domain"

	"github.com/google/uuid"
)

type UserService struct {
	repo domain.UserRepository
}

func NewUserService(repo domain.UserRepository) *UserService {
	return &UserService{
		repo: repo,
	}
}

// CreateProfile заводит профиль по сквозному id из auth-service.
// Профиль-заготовка: пустое имя и стаб-хендл, пользователь заполнит их позже.
func (s *UserService) CreateProfile(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	now := time.Now()

	user := &domain.User{
		ID:        id,
		Handle:    stubHandle(id),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *UserService) GetByHandle(ctx context.Context, handle string) (*domain.User, error) {
	return s.repo.GetByHandle(ctx, handle)
}

func (s *UserService) UpdateProfile(ctx context.Context, user *domain.User) (*domain.User, error) {
	user.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// stubHandle: "u_" + первые 8 hex-символов uuid.
// uuid.String() имеет вид "xxxxxxxx-xxxx-...", первые 8 символов — hex первой группы.
func stubHandle(id uuid.UUID) string {
	return "u_" + id.String()[:8]
}
```

Решения, зафиксированные заранее — не менять по своему усмотрению:
- `CreateProfile` **не принимает** `email`, хотя событие `user.registered` его несёт. Имя по умолчанию — пустая строка (совпадает с `DEFAULT ''` в БД), из email оно не строится.
- При коллизии стаб-хендла (`ErrHandleTaken`) ретраев **нет** — ошибка пробрасывается наверх. Вероятность ~1/4·10⁹, а событие из JetStream всё равно передоставится.
- Валидации полей в `UpdateProfile` нет — она задача transport-слоя.
- `UpdateProfile` мутирует переданный `*domain.User` (ставит `UpdatedAt`) и возвращает его же — отдельный `GetByID` после апдейта не нужен.

- [ ] **Step 2: Проверить сборку и vet**

Run: `go build ./... ; go vet ./...`
Expected: обе команды без вывода.

- [ ] **Step 3: Commit (⚠️ выполняет пользователь)**

```
user-service: add service layer (CreateProfile, Get*, UpdateProfile)
```

---

### Task 4: Установить buf

**Files:** нет изменений в репозитории — только тулинг на машине.

**Interfaces:**
- Consumes: ничего.
- Produces: исполняемый `buf` в `%USERPROFILE%\go\bin` — понадобится в следующей сессии для генерации `pkg/gen` из proto.

- [ ] **Step 1: Установить**

Run: `go install github.com/bufbuild/buf/cmd/buf@latest`
Expected: команда завершается без вывода.

- [ ] **Step 2: Проверить**

Run: `buf --version`
Expected: печатает версию (напр. `1.5x.x`). Если «команда не найдена» — `%USERPROFILE%\go\bin` не в PATH; добавить его в переменные среды и перезапустить терминал.

Никаких `buf.yaml`/`.proto` в этой сессии не создаём — контракт `GetUser`/`GetByHandle`/`UpdateProfile` проектируем на gRPC-шаге.

---

### Task 5: Проверка на живой БД и уборка

**Files:**
- Create: `backend/user-service/cmd/devcheck/main.go`
- Delete: `backend/user-service/cmd/devcheck/` (в конце задачи)

**Interfaces:**
- Consumes: всё из Task 2 и Task 3.
- Produces: ничего для последующих задач — это проверочный прогон.

- [ ] **Step 1: Убедиться, что postgres-user поднят**

Run: `docker compose ps postgres-user`
Expected: контейнер в состоянии `Up` / `healthy`. Если нет — `docker compose up -d postgres-user` из корня проекта.

- [ ] **Step 2: Написать проверочный прогон**

Создать `backend/user-service/cmd/devcheck/main.go`:

```go
// ВРЕМЕННЫЙ ФАЙЛ: ручная проверка repository+service по живой user_db.
// Удаляется после прогона, в коммит не попадает.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"user-service/internal/domain"
	"user-service/internal/repository"
	"user-service/internal/service"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const dbURL = "postgres://user:user@localhost:5436/user_db?sslmode=disable"

var failed bool

func check(name string, ok bool, detail string) {
	if ok {
		fmt.Printf("OK   %s\n", name)
		return
	}
	failed = true
	fmt.Printf("FAIL %s: %s\n", name, detail)
}

func main() {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping: %v", err)
	}

	repo := repository.NewUser(pool)
	svc := service.NewUserService(repo)

	id := uuid.New()
	defer func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id); err != nil {
			fmt.Printf("cleanup failed: %v\n", err)
		}
	}()

	// 1. CreateProfile заводит профиль со стаб-хендлом
	created, err := svc.CreateProfile(ctx, id)
	check("CreateProfile", err == nil, fmt.Sprint(err))
	if err != nil {
		os.Exit(1)
	}
	check("stub handle", created.Handle == "u_"+id.String()[:8], created.Handle)
	check("empty name", created.Name == "", created.Name)

	// 2. Повторный вызов идемпотентен (at-least-once из JetStream)
	_, err = svc.CreateProfile(ctx, id)
	check("CreateProfile idempotent", err == nil, fmt.Sprint(err))

	// 3. Чтение по id и по хендлу
	byID, err := svc.GetByID(ctx, id)
	check("GetByID", err == nil && byID != nil && byID.ID == id, fmt.Sprint(err))
	byHandle, err := svc.GetByHandle(ctx, created.Handle)
	check("GetByHandle", err == nil && byHandle != nil && byHandle.ID == id, fmt.Sprint(err))

	// 4. CITEXT: хендл ищется без учёта регистра
	upper, err := svc.GetByHandle(ctx, "U_"+id.String()[:8])
	check("GetByHandle case-insensitive", err == nil && upper != nil, fmt.Sprint(err))

	// 5. Отсутствующие записи дают ErrNotFound
	_, err = svc.GetByID(ctx, uuid.New())
	check("GetByID missing -> ErrNotFound", errors.Is(err, domain.ErrNotFound), fmt.Sprint(err))
	_, err = svc.GetByHandle(ctx, "no_such_handle_"+id.String()[:8])
	check("GetByHandle missing -> ErrNotFound", errors.Is(err, domain.ErrNotFound), fmt.Sprint(err))

	// 6. UpdateProfile меняет поля и двигает updated_at
	byID.Name = "Тест Тестов"
	byID.Bio = "bio"
	byID.City = "Москва"
	byID.Work = "работа"
	updated, err := svc.UpdateProfile(ctx, byID)
	check("UpdateProfile", err == nil, fmt.Sprint(err))
	if err == nil {
		check("updated_at moved", updated.UpdatedAt.After(created.CreatedAt), updated.UpdatedAt.String())
		reread, err := svc.GetByID(ctx, id)
		check("update persisted", err == nil && reread.Name == "Тест Тестов" && reread.City == "Москва", fmt.Sprint(err))
		// byID тоже прочитан из БД, поэтому точность у обоих одинаковая (микросекунды)
		check("created_at preserved", err == nil && reread.CreatedAt.Equal(byID.CreatedAt), fmt.Sprint(err))
	}

	// 7. Update несуществующего id -> ErrNotFound (RowsAffected == 0)
	ghost := &domain.User{ID: uuid.New(), Handle: "ghost_" + uuid.New().String()[:8]}
	_, err = svc.UpdateProfile(ctx, ghost)
	check("UpdateProfile missing -> ErrNotFound", errors.Is(err, domain.ErrNotFound), fmt.Sprint(err))

	// 8. Занятый хендл -> ErrHandleTaken (проверяет имя constraint users_handle_key)
	other := &domain.User{
		ID:        uuid.New(),
		Handle:    created.Handle,
		CreatedAt: created.CreatedAt,
		UpdatedAt: created.UpdatedAt,
	}
	err = repo.Create(ctx, other)
	check("Create duplicate handle -> ErrHandleTaken", errors.Is(err, domain.ErrHandleTaken), fmt.Sprint(err))

	if failed {
		fmt.Println("\nЕСТЬ ПАДЕНИЯ")
		os.Exit(1)
	}
	fmt.Println("\nВСЁ ЗЕЛЁНОЕ")
}
```

- [ ] **Step 3: Прогнать**

Из `D:\social_network_golang\backend\user-service`:

Run: `go run ./cmd/devcheck`
Expected: список строк `OK ...` и финальное `ВСЁ ЗЕЛЁНОЕ`, код возврата 0.

Разбор типичных падений:
- `Create duplicate handle -> ErrHandleTaken` печатает сырую ошибку с `SQLSTATE 23505` → имя constraint в БД не `users_handle_key`. Узнать реальное: `docker compose exec postgres-user psql -U user -d user_db -c "\d users"`, затем поправить строку в `mapUserErr` (Task 2) и перезапустить прогон.
- `ping: ...connection refused` → контейнер не поднят или порт не 5436 (Step 1).
- Ошибка про `citext` при чтении — расширение не создано; значит миграция не прогонялась: `goose -dir ./migrations postgres "postgres://user:user@localhost:5436/user_db?sslmode=disable" up`.

- [ ] **Step 4: Удалить временный файл**

Run: `Remove-Item -Recurse -Force .\cmd\devcheck`
Затем: `go build ./...` — Expected: без вывода (модуль цел после удаления).

- [ ] **Step 5: Проверить, что рабочее дерево чистое от временных файлов**

Run: `git status --short`
Expected: в выводе нет `cmd/devcheck`. Присутствуют только `go.mod`, `go.sum`, `internal/domain/user.go`, `internal/repository/user.go`, `internal/service/user.go` (если пользователь ещё не коммитил предыдущие задачи).

---

## Что НЕ входит в этот план

Следующие шаги user-service, согласованные в CLAUDE.md, но отложенные:
- **consumer:** JetStream pull durable на `user.registered` → `svc.CreateProfile`, порядок «запись в БД → Ack».
- **transport/grpc:** proto-контракт через buf в `pkg/gen`, тесты через `grpcurl`.
- **cmd/user/main.go:** fx-склейка (pool, NATS, repo→service→consumer, gRPC-сервер) + Dockerfile.
- Переименование `user-service/internal/confing` → `config` — **уже сделано**, папка называется `config`, долг в CLAUDE.md устарел.
