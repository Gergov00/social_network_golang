# user-service: cmd/user/main.go + Dockerfile Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Собрать user-service в работающий сервис: fx-склейка (pool → repo → service → consumer + gRPC-сервер), идемпотентное создание стрима `EVENTS` при старте, контейнеризация и подъём в docker compose — после чего замкнётся полный цикл «регистрация в auth → профиль в `user_db` → чтение по gRPC».

**Architecture:** `cmd/user/main.go` повторяет паттерн `cmd/auth/main.go`: провайдеры в `fx.Provide`, запуск через `fx.Invoke` с `fx.Hook{OnStart, OnStop}`, логгер slog в JSON. Владение конфигом стрима `EVENTS` переезжает в общий модуль `ahishka/pkg/events` (функция `EnsureEventsStream`), чтобы любой сервис, трогающий стрим, звал одну и ту же идемпотентную функцию — вместо ручного шага в CLI и вместо расползания конфига по сервисам. gRPC-сервер и NATS-консьюмер живут в одном процессе, каждый со своим lifecycle-хуком.

**Tech Stack:** Uber `fx` (DI + lifecycle), `godotenv`, `pgx/v5`, `nats.go` + `jetstream`, `google.golang.org/grpc`, Docker multi-stage build с Go workspace, docker compose.

## Global Constraints

- **Git в распоряжении Claude**, коммиты — **без** трейлера `Co-Authored-By: Claude` (пользователь просил без соавторства).
- **Не менять логику** в `internal/domain/`, `internal/repository/`, `internal/service/`, `internal/transport/grpc/` — отревьюено и смерджено. Единственное разрешённое касание чужого кода — замена приватной константы `streamName` в `internal/consumer/user_registered.go` на общую `events.StreamName` (Task 1), ровно две строки.
- Версии зависимостей, уже зафиксированные в проекте и обязательные к соблюдению: `github.com/nats-io/nats.go v1.52.0`, `google.golang.org/grpc v1.83.0`, `google.golang.org/protobuf v1.36.11`, `go.uber.org/fx v1.24.0`, `github.com/joho/godotenv v1.5.1`, `github.com/jackc/pgx/v5 v5.10.0`. Одинаковые версии во всех модулях workspace — иначе сборка возьмёт одну, а `go.mod` будет утверждать другое.
- Внутри docker-сети адреса — **по именам сервисов** (`postgres-user:5432`, `nats:4222`), НЕ хост-порты (5436/4222 — это проброс наружу для разработки с хоста).
- Build context для Dockerfile — **`./backend`**, не корень и не папка сервиса: иначе `go.work` и модуль `ahishka/pkg` не попадут в образ и сборка развалится.
- PowerShell не понимает `\` как перенос строки — команды одной строкой.

## Файловая структура

| Файл | Ответственность | Действие |
|---|---|---|
| `backend/pkg/events/stream.go` | конфиг стрима `EVENTS` + идемпотентное создание | Create |
| `backend/pkg/go.mod` | зависимость на nats.go | Modify |
| `backend/user-service/internal/consumer/user_registered.go` | убрать дублирующую константу имени стрима | Modify (2 строки) |
| `backend/user-service/internal/config/config.go` | `GRPC_PORT`, `LOG_LEVEL` | Modify |
| `backend/user-service/cmd/user/main.go` | fx-склейка всех слоёв | Rewrite (сейчас пустой стаб) |
| `backend/user-service/go.mod` | fx, godotenv | Modify |
| `backend/user-service/Dockerfile` | multi-stage сборка в workspace-режиме | Create |
| `backend/auth-service/Dockerfile` | добавить `pkg/go.sum` (см. Task 3) | Modify |
| `docker-compose.yml` | сервис `user-service` | Modify |

---

### Task 1: `EnsureEventsStream` в общем модуле

**Files:**
- Create: `backend/pkg/events/stream.go`
- Modify: `backend/pkg/go.mod`
- Modify: `backend/user-service/internal/consumer/user_registered.go` (только константа)

**Interfaces:**
- Consumes: ничего (первая задача).
- Produces:
  - `events.StreamName` — константа `"EVENTS"`.
  - `events.EnsureEventsStream(ctx context.Context, js jetstream.JetStream) error` — идемпотентно создаёт/обновляет стрим. Task 2 зовёт её в `OnStart` до старта консьюмера.

- [ ] **Step 1: Добавить nats.go в модуль pkg**

Из `D:\social_network_golang\backend\pkg`:

```powershell
go get github.com/nats-io/nats.go@v1.52.0
go mod tidy
```

- [ ] **Step 2: Написать stream.go**

Создать `backend/pkg/events/stream.go`:

```go
package events

import (
	"context"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// StreamName — стрим со всеми доменными событиями проекта.
const StreamName = "EVENTS"

// EnsureEventsStream идемпотентно создаёт стрим EVENTS (или приводит его
// конфиг к этому виду, если стрим уже есть).
//
// Живёт здесь, а не в конкретном сервисе, намеренно: стрим — общая
// инфраструктура для всего домена user.> (а дальше post.>, group.> и т.д.),
// а не собственность паблишера или консьюмера. Каждый сервис, который его
// трогает, зовёт эту функцию на своём старте; повторные вызовы с одинаковым
// конфигом безопасны. Если бы конфиг лежал в двух местах, расхождение в
// Subjects/Duplicates/retention всплывало бы молча.
func EnsureEventsStream(ctx context.Context, js jetstream.JetStream) error {
	_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       StreamName,
		Subjects:   []string{"user.>"},
		Storage:    jetstream.FileStorage,
		Retention:  jetstream.LimitsPolicy,
		Duplicates: 2 * time.Minute,
	})
	return err
}
```

- [ ] **Step 3: Убрать дублирующую константу в консьюмере**

В `backend/user-service/internal/consumer/user_registered.go` в блоке `const` (строки ~18-20) удалить строку `streamName  = "EVENTS"`, оставив `durableName` и остальные константы на месте. Затем в `Start` заменить единственное использование:

```go
	cons, err := js.CreateOrUpdateConsumer(ctx, events.StreamName, jetstream.ConsumerConfig{
```

(было `js.CreateOrUpdateConsumer(ctx, streamName, ...)`). Пакет `ahishka/pkg/events` там уже импортирован — новых импортов не нужно.

Больше в этом файле **ничего не менять**.

- [ ] **Step 4: Собрать и проверить**

Из `backend/pkg`: `go build ./...` — Expected: без вывода.
Из `backend/user-service`: `go build ./internal/... ; go vet ./internal/... ; go test ./internal/...` — Expected: build/vet без вывода, тесты `ok` по `internal/consumer` и `internal/transport/grpc`.

- [ ] **Step 5: Commit**

```bash
git add backend/pkg/events/stream.go backend/pkg/go.mod backend/pkg/go.sum backend/user-service/internal/consumer/user_registered.go backend/go.work.sum
git commit -m "pkg: own EVENTS stream config, ensure it idempotently at startup"
```

---

### Task 2: Конфиг и fx-склейка `cmd/user/main.go`

**Files:**
- Modify: `backend/user-service/internal/config/config.go`
- Modify: `backend/user-service/cmd/user/main.go` (сейчас содержит одну строку `package main`)
- Modify: `backend/user-service/go.mod`

**Interfaces:**
- Consumes: `events.EnsureEventsStream(ctx, js) error` (Task 1). Из существующего кода: `repository.NewUser(pool *pgxpool.Pool) *repository.UserRepo`; `service.NewUserService(repo domain.UserRepository) *service.UserService`; `consumer.NewUserRegisteredConsumer(svc *service.UserService, logger *slog.Logger) *consumer.UserRegisteredConsumer` с методом `Start(ctx, js) (jetstream.ConsumeContext, error)`; `grpc.NewUserHandler(svc *service.UserService) *grpc.UserHandler`; `grpc.NewServer(handler *grpc.UserHandler) *google.golang.org/grpc.Server`.
- Produces: работающий бинарь `user-service/cmd/user`. Task 3 собирает его в образ.

- [ ] **Step 1: Добавить зависимости**

Из `D:\social_network_golang\backend\user-service`:

```powershell
go get go.uber.org/fx@v1.24.0
go get github.com/joho/godotenv@v1.5.1
go mod tidy
```

- [ ] **Step 2: Расширить конфиг**

Заменить содержимое `backend/user-service/internal/config/config.go`:

```go
package config

import (
	"errors"
	"os"
)

type Config struct {
	DBURL    string
	NATSURL  string
	GRPCPort string
	LogLevel string
}

func Load() (*Config, error) {
	cfg := &Config{
		DBURL:    os.Getenv("DB_URL"),
		NATSURL:  getEnv("NATS_URL", "nats://localhost:4222"),
		GRPCPort: getEnv("GRPC_PORT", "50051"),
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	if cfg.DBURL == "" {
		return nil, errors.New("DB_URL is required")
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
```

- [ ] **Step 3: Написать main.go**

Заменить содержимое `backend/user-service/cmd/user/main.go`:

```go
package main

import (
	"context"
	"log/slog"
	"net"
	"os"

	"ahishka/pkg/events"
	"user-service/internal/config"
	"user-service/internal/consumer"
	"user-service/internal/domain"
	"user-service/internal/repository"
	"user-service/internal/service"
	usergrpc "user-service/internal/transport/grpc"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"google.golang.org/grpc"
)

func main() {
	godotenv.Load()

	app := fx.New(
		fx.Provide(
			config.Load,
			NewLogger,
			NewDataBase,
			NewNATS,
			NewJetStream,
			fx.Annotate(repository.NewUser, fx.As(new(domain.UserRepository))),
			service.NewUserService,
			usergrpc.NewUserHandler,
			usergrpc.NewServer,
			consumer.NewUserRegisteredConsumer,
		),
		fx.WithLogger(func(logger *slog.Logger) fxevent.Logger {
			return &fxevent.SlogLogger{Logger: logger}
		}),
		fx.Invoke(RunGRPC, RunConsumer),
	)
	app.Run()
}

func NewLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return logger
}

func NewDataBase(lc fx.Lifecycle, cfg *config.Config, logger *slog.Logger) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), cfg.DBURL)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("connecting to database")
			return pool.Ping(ctx)
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("closing database connection")
			pool.Close()
			return nil
		},
	})

	return pool, nil
}

func NewNATS(lc fx.Lifecycle, cfg *config.Config, logger *slog.Logger) (*nats.Conn, error) {
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("draining nats connection")
			return nc.Drain()
		},
	})
	return nc, nil
}

func NewJetStream(nc *nats.Conn) (jetstream.JetStream, error) {
	return jetstream.New(nc)
}

func RunGRPC(lc fx.Lifecycle, srv *grpc.Server, cfg *config.Config, logger *slog.Logger, shutdowner fx.Shutdowner) {
	addr := ":" + cfg.GRPCPort

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Listen синхронно: занятый порт должен уронить старт, а не всплыть
			// как ошибка в фоновой горутине уже после "приложение поднялось".
			lis, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}

			logger.Info("starting grpc server", "addr", addr)
			go func() {
				if err := srv.Serve(lis); err != nil {
					logger.Error("grpc server failed", "error", err)
					_ = shutdowner.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("stopping grpc server")
			srv.GracefulStop()
			return nil
		},
	})
}

func RunConsumer(lc fx.Lifecycle, cons *consumer.UserRegisteredConsumer, js jetstream.JetStream, logger *slog.Logger) {
	var consumeCtx jetstream.ConsumeContext

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Стрим должен существовать до привязки консьюмера:
			// CreateOrUpdateConsumer к несуществующему стриму — ошибка.
			if err := events.EnsureEventsStream(ctx, js); err != nil {
				return err
			}

			logger.Info("starting user.registered consumer")
			started, err := cons.Start(ctx, js)
			if err != nil {
				return err
			}
			consumeCtx = started
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if consumeCtx != nil {
				logger.Info("stopping user.registered consumer")
				consumeCtx.Stop()
			}
			return nil
		},
	})
}
```

Осознанное решение, которое не надо «улучшать» по своему усмотрению: **сервис падает на старте, если NATS недоступен** (ошибка из `nats.Connect` или из `EnsureEventsStream` роняет `fx`). Альтернатива — деградировать (поднять gRPC-чтения, а консьюмер запускать в фоне с ретраями) — осмысленна, но это отдельное решение с ретрай-логикой и мониторингом; сейчас fail-fast проще и совпадает с поведением auth-service. Записано как известный компромисс.

- [ ] **Step 4: Собрать весь модуль**

Из `backend/user-service`:

Run: `go build ./... ; go vet ./... ; go test ./...`
Expected: build/vet без вывода — **это первый раз, когда `./...` (а не `./internal/...`) собирается целиком**, потому что до сих пор `cmd/user/main.go` был стабом без `func main`. Тесты — `ok` по `internal/consumer` и `internal/transport/grpc`.

- [ ] **Step 5: Поднять инфраструктуру и прогнать миграции**

Из `D:\social_network_golang`:

```powershell
docker compose up -d nats postgres-user
```

Убедиться, что миграции применены (таблица `users` существует):

```powershell
docker compose exec -T postgres-user psql -U user -d user_db -c "\dt"
```

Если в списке только `goose_db_version` — прогнать миграции из `backend\user-service\migrations`:

```powershell
goose -dir . postgres "postgres://user:user@localhost:5436/user_db?sslmode=disable" up
```

- [ ] **Step 6: Запустить сервис локально и проверить живьём**

Из `backend/user-service`, задав окружение на хост-порты:

```powershell
$env:DB_URL="postgres://user:user@localhost:5436/user_db?sslmode=disable"; $env:NATS_URL="nats://localhost:4222"; $env:GRPC_PORT="50051"; go run ./cmd/user
```

Запустить в фоне. Ожидаемо в логах: `connecting to database`, `starting user.registered consumer`, `starting grpc server addr=:50051`, и `[Fx] RUNNING`.

Затем в другом окне — три проверки:

**(а) gRPC отвечает и reflection работает:**

```powershell
grpcurl -plaintext 127.0.0.1:50051 list
```
Expected: печатает `ahishka.user.v1.UserService` и reflection-сервисы.

**(б) Стрим создался программно.** Это ключевая проверка Task 1 — раньше `EVENTS` заводили руками через CLI:

```powershell
curl.exe -s "http://localhost:8222/jsz?streams=true"
```
Expected: в JSON виден стрим с `"name":"EVENTS"`. (Порт 8222 — NATS monitoring, проброшен в compose.)

**(в) Полный асинхронный цикл: событие → профиль → чтение по gRPC.**
Опубликовать `user.registered` и убедиться, что профиль появился и читается. Сгенерировать любой uuid (например `aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee`), опубликовать событие через NATS и прочитать профиль:

```powershell
grpcurl -plaintext -d '{\"id\": \"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee\"}' 127.0.0.1:50051 ahishka.user.v1.UserService/GetUser
```

До публикации события Expected: `NotFound: user not found`.

**`nats` CLI на этой машине НЕ установлен** (проверено), и ставить его в рамках этой задачи не нужно. Публикуй событие самым дешёвым доступным способом: подними auth-service (`docker compose up -d auth-service`) и зарегистрируй пользователя — `curl.exe -s -X POST http://localhost:10001/auth/register -H "Content-Type: application/json" -d '{\"email\":\"e2e@example.com\",\"password\":\"password123\"}'`. Ответ 201 означает, что auth опубликовал `user.registered` с новым id; этот id виден в `users` (`docker compose exec -T postgres-user psql -U user -d user_db -c "SELECT id, handle, name FROM users"`), и по нему же дёрни `GetUser` через grpcurl.

Если auth-service поднять не выходит — зафиксируй в отчёте, что (в) полностью не проверено, и вместо этого убедись хотя бы, что консьюмер зарегистрирован на стриме:

```powershell
curl.exe -s "http://localhost:8222/jsz?consumers=true&streams=true"
```
Expected: виден консьюмер `user-service-user-registered`.

После публикации события повторный `GetUser` Expected: возвращает профиль с `handle` вида `u_aaaaaaaa` и пустым `name`.

- [ ] **Step 7: Остановить сервис и почистить тестовые данные**

Остановить фоновый `go run`. Удалить тестовый профиль, если он создавался:

```powershell
docker compose exec -T postgres-user psql -U user -d user_db -c "DELETE FROM users WHERE id = 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee'"
```

- [ ] **Step 8: Commit**

```bash
git add backend/user-service/cmd/user/main.go backend/user-service/internal/config/config.go backend/user-service/go.mod backend/user-service/go.sum backend/go.work.sum
git commit -m "user-service: wire everything together in cmd/user/main.go"
```

---

### Task 3: Dockerfile и docker compose

**Files:**
- Create: `backend/user-service/Dockerfile`
- Modify: `backend/auth-service/Dockerfile` (одна строка — см. Step 2)
- Modify: `docker-compose.yml`

**Interfaces:**
- Consumes: бинарь `user-service/cmd/user` (Task 2).
- Produces: сервис `user-service` в compose, слушающий gRPC на хост-порту 50051.

- [ ] **Step 1: Написать Dockerfile**

Создать `backend/user-service/Dockerfile`:

```dockerfile
FROM golang:1.26-alpine AS builder

WORKDIR /src

# go.work + go.mod всех модулей workspace: без них сборка в workspace-режиме
# не разрешит ahishka/pkg. Исходники auth-service при этом не нужны — только
# его go.mod, чтобы workspace был консистентен.
COPY go.work go.work.sum ./
COPY auth-service/go.mod auth-service/go.sum ./auth-service/
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY user-service/go.mod user-service/go.sum ./user-service/
RUN go mod download

COPY pkg/ ./pkg/
COPY user-service/ ./user-service/

RUN CGO_ENABLED=0 GOOS=linux go build -o /user ./user-service/cmd/user

FROM gcr.io/distroless/static-debian12

COPY --from=builder /user /user

EXPOSE 50051

ENTRYPOINT ["/user"]
```

- [ ] **Step 2: Починить Dockerfile auth-service**

`backend/auth-service/Dockerfile` строка 7 сейчас копирует только `pkg/go.mod`:

```dockerfile
COPY pkg/go.mod ./pkg/
```

Когда этот Dockerfile писался, у модуля `pkg` не было зависимостей и `go.sum` не существовал. Теперь `pkg` тянет grpc/protobuf/nats, `backend/pkg/go.sum` существует, и `go mod download` в workspace-режиме требует его. Заменить строку на:

```dockerfile
COPY pkg/go.mod pkg/go.sum ./pkg/
```

Это чинит скрытую поломку сборки auth-образа, а не «улучшение по пути» — без неё Step 5 покажет провал.

- [ ] **Step 3: Добавить сервис в compose**

В `docker-compose.yml` в блок `services:` добавить (рядом с `auth-service`, до `postgres-auth`):

```yaml
  user-service:
    build:
      context: ./backend
      dockerfile: user-service/Dockerfile
    container_name: ahishka-user
    environment:
      DB_URL: postgres://user:user@postgres-user:5432/user_db?sslmode=disable
      GRPC_PORT: 50051
      LOG_LEVEL: info
      NATS_URL: nats://nats:4222
    ports:
      - "50051:50051"
    depends_on:
      postgres-user:
        condition: service_healthy
      nats:
        condition: service_started
    networks:
      - ahishka
```

Обрати внимание: `postgres-user:5432` и `nats:4222` — внутрисетевые имена и **внутренние** порты, не хост-порты 5436/4222.

- [ ] **Step 4: Собрать образ**

Из `D:\social_network_golang`:

Run: `docker compose build user-service`
Expected: сборка проходит до конца, финальная строка вида `naming to docker.io/library/...user-service`.

Если падает на `go mod download` с жалобой на отсутствующий `go.sum` — проверь, что скопированы `go.sum` всех трёх модулей (Step 1).

- [ ] **Step 5: Проверить, что образ auth тоже собирается**

Run: `docker compose build auth-service`
Expected: успешная сборка. Это проверка фикса из Step 2.

- [ ] **Step 6: Поднять и проверить работающий контейнер**

```powershell
docker compose up -d user-service
docker compose logs user-service
```

Expected в логах: JSON-строки `connecting to database`, `starting user.registered consumer`, `starting grpc server` с `addr=:50051`, и fx-строка `RUNNING`. Никаких паник и повторных рестартов (`docker compose ps user-service` должен показывать `Up`, а не `Restarting`).

Проверить gRPC снаружи контейнера:

```powershell
grpcurl -plaintext 127.0.0.1:50051 list
```
Expected: `ahishka.user.v1.UserService` в списке.

```powershell
grpcurl -plaintext -d '{\"id\": \"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee\"}' 127.0.0.1:50051 ahishka.user.v1.UserService/GetUser
```
Expected: `NotFound: user not found` — сервис в контейнере реально ходит в свою БД и корректно мапит ошибку.

Если контейнер в `Restarting` с ошибкой про подключение к БД — почти наверняка в `DB_URL` указан хост-порт `5436` вместо внутреннего `5432`.

- [ ] **Step 7: Commit**

```bash
git add backend/user-service/Dockerfile backend/auth-service/Dockerfile docker-compose.yml
git commit -m "user-service: containerize and add to docker compose"
```

---

## Что НЕ входит в этот план

- **Автопрогон миграций** при старте сервиса: `users` в `user_db` по-прежнему накатывается вручную через goose, как и таблицы auth. Автоматизация (goose-embed или init-контейнер) — отдельное решение для обоих сервисов сразу.
- **Деградация при недоступном NATS:** сейчас fail-fast (см. Task 2 Step 3). Вариант «поднимать gRPC-чтения даже без брокера, консьюмер с ретраями» осмыслен и записан как компромисс, но требует ретрай-политики и мониторинга.
- **Health-check эндпоинт** у user-service: у auth есть `GET /health`, у gRPC аналога пока нет. Стандартный путь — `grpc_health_v1` + `healthcheck` в compose; делать вместе с gateway, когда появится, кому проверять.
- **gateway** — следующий крупный шаг по roadmap: REST/JSON наружу, валидация JWT, `X-User-Id` в gRPC-metadata, и он же первый реальный клиент этого gRPC-контракта.
- **Мелкий долг из прошлого ревью:** лог-строка generic-ошибки в консьюмере (`Nak`-путь) не несёт `subject`/`stream_seq`, в отличие от `Term`-путей.
