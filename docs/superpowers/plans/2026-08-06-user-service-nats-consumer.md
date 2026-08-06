# user-service: NATS consumer (user.registered → CreateProfile) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Замкнуть асинхронный цикл auth→user: durable JetStream-консьюмер на `user.registered` вызывает `service.UserService.CreateProfile`, чтобы профиль в `user_db` появлялся сам после регистрации в auth, без ручных действий.

**Architecture:** Новый пакет `internal/consumer` в user-service. Конструктор принимает уже существующий `*service.UserService` (Task repo+service, `1c7e792`) и логгер; метод `Start` привязывается к durable pull-консьюмеру на стриме `EVENTS` (nats.go `jetstream.Consumer.Consume`, callback-обёртка над pull-моделью) и возвращает `jetstream.ConsumeContext` для остановки. Порядок в хендлере — строго «запись в БД → Ack»: если `CreateProfile` не отработал, сообщение не подтверждается и JetStream передоставит его повторно (идемпотентность уже обеспечена `ON CONFLICT DO NOTHING` в репозитории). Подключение к NATS/JetStream и запуск консьюмера в `cmd/user/main.go` — вне этого плана (главный fx-wiring ещё не начат).

**Tech Stack:** `github.com/nats-io/nats.go` + `github.com/nats-io/nats.go/jetstream` v1.52.0 (та же версия, что в `auth-service/go.mod`), `ahishka/pkg/events` (контракт `UserRegistered`/`SubjectUserRegistered`), NATS JetStream в docker-compose (стрим `EVENTS`, `--subjects "user.>"`).

## Global Constraints

- **Git — в распоряжении Claude** (см. `CLAUDE.md`, правило от 2026-08-06): коммиты можно делать самостоятельно, но **без** трейлера `Co-Authored-By: Claude`.
- Версии зависимостей — как в `auth-service/go.mod`: `github.com/nats-io/nats.go v1.52.0`.
- Конструкторы принимают конкретные типы соседних слоёв, не интерфейсы (`*service.UserService`), как уже решено для repo/service.
- Никакой валидации/бизнес-логики в консьюмере сверх разбора payload — вся логика уже в `CreateProfile`.
- Создание стрима `EVENTS` в проде остаётся ручным (CLI) — это известный долг из `CLAUDE.md`, этот план его не закрывает. Скрипт проверки в Task 2 создаёт стрим идемпотентно только для локального прогона.
- PowerShell не понимает `\` как перенос строки — команды одной строкой.

## Файловая структура

| Файл | Ответственность | Действие |
|---|---|---|
| `backend/user-service/go.mod` | зависимость на nats.go/jetstream | Modify |
| `backend/user-service/internal/consumer/user_registered.go` | durable-консьюмер `user.registered` → `CreateProfile` | Create |
| `backend/user-service/cmd/devcheck/main.go` | **временный** прогон консьюмера на живых NATS+Postgres | Create, затем Delete |

---

### Task 1: Durable-консьюмер `user.registered`

**Files:**
- Modify: `backend/user-service/go.mod`
- Create: `backend/user-service/internal/consumer/user_registered.go`

**Interfaces:**
- Consumes: `service.NewUserService` / `(*service.UserService).CreateProfile(ctx, id uuid.UUID) (*domain.User, error)` (существует, `internal/service/user.go`), `events.UserRegistered{ID, Email string}` и `events.SubjectUserRegistered` (существуют, `ahishka/pkg/events`).
- Produces:
  - `consumer.NewUserRegisteredConsumer(svc *service.UserService, logger *slog.Logger) *consumer.UserRegisteredConsumer`
  - `(*UserRegisteredConsumer).Start(ctx context.Context, js jetstream.JetStream) (jetstream.ConsumeContext, error)` — на этот метод будет опираться будущий `cmd/user/main.go` (fx `OnStart` вызовет `Start`, `OnStop` — `cc.Stop()` на возвращённом `jetstream.ConsumeContext`).

- [ ] **Step 1: Добавить зависимость**

Из `D:\social_network_golang\backend\user-service` в PowerShell:

```powershell
go get github.com/nats-io/nats.go@v1.52.0
go mod tidy
```

- [ ] **Step 2: Написать консьюмер**

Создать `backend/user-service/internal/consumer/user_registered.go`:

```go
package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"ahishka/pkg/events"
	"user-service/internal/service"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	streamName  = "EVENTS"
	durableName = "user-service-user-registered"
)

type UserRegisteredConsumer struct {
	svc    *service.UserService
	logger *slog.Logger
}

func NewUserRegisteredConsumer(svc *service.UserService, logger *slog.Logger) *UserRegisteredConsumer {
	return &UserRegisteredConsumer{
		svc:    svc,
		logger: logger,
	}
}

// Start привязывается к durable pull-консьюмеру на стриме EVENTS и начинает
// разбирать сообщения в фоне (jetstream.Consumer.Consume сам гоняет pull-цикл
// под удобным callback API). Вызывающий останавливает обработку через
// cc.Stop() на возвращённом ConsumeContext.
func (c *UserRegisteredConsumer) Start(ctx context.Context, js jetstream.JetStream) (jetstream.ConsumeContext, error) {
	cons, err := js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		Durable:       durableName,
		FilterSubject: events.SubjectUserRegistered,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    5,
	})
	if err != nil {
		return nil, err
	}

	return cons.Consume(c.handle)
}

func (c *UserRegisteredConsumer) handle(msg jetstream.Msg) {
	ctx := context.Background()

	var payload events.UserRegistered
	if err := json.Unmarshal(msg.Data(), &payload); err != nil {
		c.logger.Error("user.registered: bad payload, dropping", "error", err)
		_ = msg.Term()
		return
	}

	id, err := uuid.Parse(payload.ID)
	if err != nil {
		c.logger.Error("user.registered: bad id, dropping", "raw_id", payload.ID, "error", err)
		_ = msg.Term()
		return
	}

	// Запись в БД -> потом Ack. Если CreateProfile не отработал, сообщение
	// НЕ подтверждается: JetStream передоставит его (MaxDeliver=5), а
	// Create в репозитории идемпотентен (ON CONFLICT DO NOTHING).
	if _, err := c.svc.CreateProfile(ctx, id); err != nil {
		c.logger.Error("user.registered: CreateProfile failed, will retry", "id", id.String(), "error", err)
		_ = msg.Nak()
		return
	}

	if err := msg.Ack(); err != nil {
		c.logger.Error("user.registered: ack failed", "id", id.String(), "error", err)
	}
}
```

Известное ограничение (осознанно не решаем в этом плане): если `CreateProfile` падает на `domain.ErrHandleTaken` (коллизия стаб-хендла — вероятность ~1/4·10⁹, см. предыдущий план), `Nak` не поможет — повтор даёт тот же хендл и ту же ошибку. После `MaxDeliver=5` JetStream просто перестанет передоставлять; сообщение останется неподтверждённым в стриме. Это тот же риск, что уже принят на уровне `service.CreateProfile` (без ретраев), консьюмер его не усугубляет и не должен здесь чинить.

- [ ] **Step 3: Собрать и проверить**

Run: `go build ./internal/... ; go vet ./internal/...`
Expected: обе команды без вывода.

- [ ] **Step 4: Commit**

```bash
git add backend/user-service/go.mod backend/user-service/go.sum backend/user-service/internal/consumer/user_registered.go
git commit -m "user-service: add durable JetStream consumer for user.registered"
```

---

### Task 2: Проверка на живых NATS + Postgres, уборка

**Files:**
- Create: `backend/user-service/cmd/devcheck/main.go`
- Delete: `backend/user-service/cmd/devcheck/` (в конце задачи)

**Interfaces:**
- Consumes: всё из Task 1, `repository.NewUser`/`service.NewUserService` (существуют).
- Produces: ничего для последующих задач — проверочный прогон.

- [ ] **Step 1: Поднять NATS и postgres-user**

Run: `docker compose up -d nats postgres-user`
Expected: оба контейнера `Up`/`healthy`. `nats` мог быть остановлен (не поднимался 6+ недель) — `docker compose up -d nats` его запустит с уже существующим томом `nats_data`.

- [ ] **Step 2: Написать проверочный прогон**

Создать `backend/user-service/cmd/devcheck/main.go`:

```go
// ВРЕМЕННЫЙ ФАЙЛ: ручная проверка consumer'а по живым NATS+user_db.
// Удаляется после прогона, в коммит не попадает.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"ahishka/pkg/events"
	"user-service/internal/consumer"
	"user-service/internal/repository"
	"user-service/internal/service"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	dbURL   = "postgres://user:user@localhost:5436/user_db?sslmode=disable"
	natsURL = "nats://localhost:4222"
)

var failed bool

func check(name string, ok bool, detail string) {
	if ok {
		fmt.Printf("OK   %s\n", name)
		return
	}
	failed = true
	fmt.Printf("FAIL %s: %s\n", name, detail)
}

// waitFor опрашивает cond раз в 100мс до timeout.
func waitFor(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("pg pool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("pg ping: %v", err)
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	// Идемпотентно гарантируем стрим EVENTS для локального прогона
	// (в проде стрим создаётся вручную через nats CLI, см. CLAUDE.md).
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       "EVENTS",
		Subjects:   []string{"user.>"},
		Storage:    jetstream.FileStorage,
		Retention:  jetstream.LimitsPolicy,
		Duplicates: 2 * time.Minute,
	})
	if err != nil {
		log.Fatalf("ensure stream: %v", err)
	}

	repo := repository.NewUser(pool)
	svc := service.NewUserService(repo)
	cons := consumer.NewUserRegisteredConsumer(svc, logger)

	cc, err := cons.Start(ctx, js)
	check("consumer starts", err == nil, fmt.Sprint(err))
	if err != nil {
		os.Exit(1)
	}
	defer cc.Stop()

	// 1. Валидное событие -> профиль появляется в БД
	id := uuid.New()
	defer func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id); err != nil {
			fmt.Printf("cleanup failed: %v\n", err)
		}
	}()

	payload, _ := json.Marshal(events.UserRegistered{ID: id.String(), Email: "check@example.com"})
	_, err = js.Publish(ctx, events.SubjectUserRegistered, payload, jetstream.WithMsgID(id.String()))
	check("publish valid event", err == nil, fmt.Sprint(err))

	created := waitFor(5*time.Second, func() bool {
		u, err := svc.GetByID(ctx, id)
		return err == nil && u != nil
	})
	check("valid event -> profile created", created, "profile did not appear within 5s")

	// 2. Некорректный payload -> Term, консьюмер не падает и продолжает работать
	_, err = js.Publish(ctx, events.SubjectUserRegistered, []byte(`{"id": true}`))
	check("publish malformed event", err == nil, fmt.Sprint(err))

	id2 := uuid.New()
	defer func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id2); err != nil {
			fmt.Printf("cleanup failed: %v\n", err)
		}
	}()
	payload2, _ := json.Marshal(events.UserRegistered{ID: id2.String(), Email: "check2@example.com"})
	_, err = js.Publish(ctx, events.SubjectUserRegistered, payload2, jetstream.WithMsgID(id2.String()))
	check("publish valid event after malformed", err == nil, fmt.Sprint(err))

	survived := waitFor(5*time.Second, func() bool {
		u, err := svc.GetByID(ctx, id2)
		return err == nil && u != nil
	})
	check("consumer survives malformed message", survived, "second profile did not appear -> consumer likely stuck/crashed")

	// 3. Консьюмер зарегистрирован как durable (не эфемерный push-сабскрайбер)
	_, err = js.Consumer(ctx, "EVENTS", "user-service-user-registered")
	check("durable consumer registered", err == nil, fmt.Sprint(err))

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
Expected: 6 строк `OK ...` и `ВСЁ ЗЕЛЁНОЕ`.

Разбор типичных падений:
- `nats connect: dial tcp ... connection refused` → контейнер `nats` не поднят (Step 1) или порт не 4222.
- `ensure stream: ...` с кодом о несовпадении конфига → на сервере уже есть стрим `EVENTS` с другими параметрами (например, другой `Duplicates`) — `CreateOrUpdateStream` тогда обновит несовпадающие поля; если ошибка persists, стрим создан вручную с конфликтующими subjects — разобраться руками (`docker compose exec nats nats stream info EVENTS`, если есть CLI внутри образа, либо через monitor `http://localhost:8222/jsz?streams=true`).
- `valid event -> profile created` FAIL при `consumer starts` == OK → проверить логи консьюмера (`logger` пишет в stdout) на `bad id`/`CreateProfile failed`; частая причина — `FilterSubject` не совпал с реально опубликованным subject (проверить константу `events.SubjectUserRegistered`).
- `durable consumer registered` FAIL → консьюмер не был создан как `Durable` (опечатка в `durableName`/`streamName` между Task 1 и Task 2 — они должны быть одной строкой `"user-service-user-registered"` / `"EVENTS"`).

- [ ] **Step 4: Удалить временный файл**

Run: `Remove-Item -Recurse -Force .\cmd\devcheck`
Затем: `go build ./internal/...` — Expected: без вывода.

- [ ] **Step 5: Проверить чистоту рабочего дерева**

Run: `git status --short`
Expected: нет `cmd/devcheck`. Есть только файлы из Task 1 (если ещё не закоммичены).

---

## Что НЕ входит в этот план

- **`cmd/user/main.go`:** fx-wiring (`pgxpool.Pool`, `nats.Connect`+`jetstream.New`, `repo→service→consumer`, запуск `consumer.Start` в `OnStart` / `cc.Stop()` в `OnStop`) + Dockerfile user-service.
- **transport/grpc:** proto-контракт через buf (уже установлен, `buf --version` → `1.72.0`) в `pkg/gen`, gRPC-сервер для `GetUser`/`GetByHandle`/`UpdateProfile`.
- **Автоматизация создания стрима `EVENTS` в проде** — остаётся ручной операцией (CLI); идемпотентное создание в `cmd/devcheck` (Task 2) решает только локальный прогон, не production debt.
- **Dead-letter/алертинг** для сообщений, исчерпавших `MaxDeliver` — известное ограничение, описано в Task 1, не устраняется.
