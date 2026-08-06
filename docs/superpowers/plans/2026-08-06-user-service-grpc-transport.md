# user-service: transport/grpc Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Дать user-service gRPC-транспорт (`GetUser`/`GetUserByHandle`/`UpdateProfile`) — proto-контракт в `backend/proto`, сгенерированный код в модуле `ahishka/pkg/gen`, адаптер поверх существующего `service.UserService`, покрытый end-to-end тестами через `bufconn`.

**Architecture:** Ядро (`domain`/`repository`/`service`) транспорт-агностично и в этом плане **не меняется** — gRPC это тонкий адаптер `internal/transport/grpc` поверх того же `*service.UserService`, ровно как `transport/web` у auth-service. Контракт живёт в общем модуле `ahishka/pkg` (генерённый код в `pkg/gen`), чтобы будущий gateway импортировал клиента оттуда же, не завися от модуля user-service. Хендлер отвечает только за три вещи: разбор/валидацию входа, конвертацию `domain.User` ↔ proto, и перевод доменных ошибок в gRPC-коды.

**Tech Stack:** buf v1.72.0 (уже установлен), `protoc-gen-go` + `protoc-gen-go-grpc` (ставим локально), `google.golang.org/grpc`, `google.golang.org/protobuf`, `google.golang.org/grpc/test/bufconn` для тестов, `grpcurl` для ручной проверки reflection.

## Global Constraints

- **Git в распоряжении Claude**, коммиты — **без** трейлера `Co-Authored-By: Claude` (пользователь просил без соавторства).
- **Не трогать** `internal/domain/`, `internal/repository/`, `internal/service/`, `internal/consumer/` — они отревьюены и смерджены. Этот план только добавляет транспорт.
- Одна и та же версия `google.golang.org/grpc` и `google.golang.org/protobuf` в `pkg/go.mod` и `user-service/go.mod` — иначе workspace-сборка возьмёт одну, а `go.mod` будет врать. Проверять после `go get`.
- Генерённый код (`pkg/gen/**`) **коммитится** — как и `go.work`, чтобы сборка не требовала buf на машине.
- Слушающий сокет, порт и fx-lifecycle — **вне этого плана** (это `cmd/user/main.go`, следующий шаг roadmap). Здесь производится сконфигурированный `*grpc.Server`, который main.go останется только запустить.
- PowerShell не понимает `\` как перенос строки — команды одной строкой.

## Файловая структура

| Файл | Ответственность | Действие |
|---|---|---|
| `backend/buf.yaml` | buf-модуль: где лежат proto, правила lint/breaking | Create |
| `backend/buf.gen.yaml` | какие плагины и куда генерят | Create |
| `backend/proto/ahishka/user/v1/user.proto` | контракт: сервис + сообщения | Create |
| `backend/pkg/gen/ahishka/user/v1/*.pb.go` | генерённый код (не редактируется руками) | Create (сгенерённый) |
| `backend/pkg/go.mod` | зависимости генерённого кода | Modify |
| `backend/user-service/internal/transport/grpc/user.go` | хендлер: валидация, конвертация, коды ошибок | Create |
| `backend/user-service/internal/transport/grpc/server.go` | сборка `*grpc.Server` + регистрация + reflection | Create |
| `backend/user-service/internal/transport/grpc/user_test.go` | end-to-end тесты через `bufconn` | Create |
| `backend/user-service/go.mod` | зависимости транспорта | Modify |

**Про имя пакета:** директория `internal/transport/grpc`, имя пакета — `grpc` (симметрично `transport/web` → `web` в auth-service). Внутри этого пакета `import "google.golang.org/grpc"` работает без конфликта: Go не квалифицирует собственные символы пакета именем пакета. Конфликт возникнет только в `cmd/user/main.go`, который импортирует оба — там нужен алиас (это забота следующего плана, отмечено в Interfaces Task 3).

---

### Task 1: Тулинг, proto-контракт и генерация

**Files:**
- Create: `backend/buf.yaml`, `backend/buf.gen.yaml`, `backend/proto/ahishka/user/v1/user.proto`
- Create (сгенерённый): `backend/pkg/gen/ahishka/user/v1/user.pb.go`, `backend/pkg/gen/ahishka/user/v1/user_grpc.pb.go`
- Modify: `backend/pkg/go.mod`

**Interfaces:**
- Consumes: ничего (первая задача).
- Produces: Go-пакет `ahishka/pkg/gen/ahishka/user/v1`, импортируемый как `userv1`. Из него Task 2 и Task 3 берут: типы `userv1.User`, `userv1.GetUserRequest`/`GetUserResponse`, `userv1.GetUserByHandleRequest`/`GetUserByHandleResponse`, `userv1.UpdateProfileRequest`/`UpdateProfileResponse`; интерфейс `userv1.UserServiceServer`; встраиваемую заглушку `userv1.UnimplementedUserServiceServer`; функцию регистрации `userv1.RegisterUserServiceServer(s grpc.ServiceRegistrar, srv UserServiceServer)`; клиент `userv1.NewUserServiceClient(cc grpc.ClientConnInterface) UserServiceClient`.

- [ ] **Step 1: Поставить кодогенераторы**

```powershell
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Проверить: `protoc-gen-go --version` — печатает версию. Если «команда не найдена» — `%USERPROFILE%\go\bin` не в PATH (там же лежит уже работающий `buf.exe`, так что PATH почти наверняка в порядке — перезапустить терминал).

`protoc` ставить **не нужно**: buf содержит собственный компилятор и вызывает только плагины.

- [ ] **Step 2: Создать `backend/buf.yaml`**

```yaml
version: v2
modules:
  - path: proto
lint:
  use:
    - STANDARD
breaking:
  use:
    - FILE
```

- [ ] **Step 3: Создать `backend/buf.gen.yaml`**

```yaml
version: v2
plugins:
  - local: protoc-gen-go
    out: pkg/gen
    opt: paths=source_relative
  - local: protoc-gen-go-grpc
    out: pkg/gen
    opt: paths=source_relative
```

`paths=source_relative` + `out: pkg/gen` означает: файл `proto/ahishka/user/v1/user.proto` (путь относительно модуля `proto/`) → `pkg/gen/ahishka/user/v1/user.pb.go`. Этот путь обязан совпасть с `go_package` в самом proto, иначе импорт не разрешится.

- [ ] **Step 4: Создать `backend/proto/ahishka/user/v1/user.proto`**

```proto
syntax = "proto3";

package ahishka.user.v1;

import "google/protobuf/timestamp.proto";

option go_package = "ahishka/pkg/gen/ahishka/user/v1;userv1";

// UserService — профили пользователей. id сквозной через все сервисы:
// его генерит auth при регистрации, здесь он же первичный ключ профиля.
service UserService {
  rpc GetUser(GetUserRequest) returns (GetUserResponse);
  rpc GetUserByHandle(GetUserByHandleRequest) returns (GetUserByHandleResponse);
  rpc UpdateProfile(UpdateProfileRequest) returns (UpdateProfileResponse);
}

message User {
  string id = 1;
  string name = 2;
  string handle = 3;
  string bio = 4;
  string city = 5;
  string work = 6;
  bool verified = 7;
  // optional, т.к. в БД avatar_url nullable (media-сервис отложен)
  optional string avatar_url = 8;
  google.protobuf.Timestamp created_at = 9;
  google.protobuf.Timestamp updated_at = 10;
}

message GetUserRequest {
  string id = 1;
}

message GetUserResponse {
  User user = 1;
}

message GetUserByHandleRequest {
  string handle = 1;
}

message GetUserByHandleResponse {
  User user = 1;
}

// Обновляются только эти поля — verified/avatar_url/created_at меняются
// другими сценариями (модерация, media), профиль их здесь не трогает.
message UpdateProfileRequest {
  string id = 1;
  string name = 2;
  string handle = 3;
  string bio = 4;
  string city = 5;
  string work = 6;
}

message UpdateProfileResponse {
  User user = 1;
}
```

- [ ] **Step 5: Прогнать lint и генерацию**

Из `D:\social_network_golang\backend`:

```powershell
buf lint
buf generate
```

Expected: `buf lint` — без вывода. `buf generate` — без вывода, появились `pkg/gen/ahishka/user/v1/user.pb.go` и `user_grpc.pb.go`.

Если lint ругается на `PACKAGE_DIRECTORY_MATCH` — путь файла не совпал с proto-пакетом: файл обязан лежать именно в `proto/ahishka/user/v1/`.

- [ ] **Step 6: Добавить зависимости в модуль `pkg`**

Генерённый код импортирует protobuf runtime и grpc. Из `D:\social_network_golang\backend\pkg`:

```powershell
go get google.golang.org/protobuf@latest
go get google.golang.org/grpc@latest
go mod tidy
go build ./...
```

Expected: `go build ./...` без вывода.

- [ ] **Step 7: Зафиксировать версии для Task 2**

Run: `Get-Content go.mod` (в `backend\pkg`)
Записать в отчёт точные строки версий `google.golang.org/grpc` и `google.golang.org/protobuf` — Task 2 обязан получить те же версии в `user-service/go.mod`.

- [ ] **Step 8: Commit**

```bash
git add backend/buf.yaml backend/buf.gen.yaml backend/proto backend/pkg/gen backend/pkg/go.mod backend/pkg/go.sum backend/go.work.sum
git commit -m "pkg: add user v1 proto contract and generated grpc code"
```

---

### Task 2: gRPC-хендлер поверх service

**Files:**
- Create: `backend/user-service/internal/transport/grpc/user.go`
- Modify: `backend/user-service/go.mod`

**Interfaces:**
- Consumes: из Task 1 — `userv1.UserServiceServer`, `userv1.UnimplementedUserServiceServer`, все Request/Response типы и `userv1.User`. Из существующего кода (не меняется): `service.NewUserService(repo domain.UserRepository) *service.UserService` с методами `GetByID(ctx, uuid.UUID) (*domain.User, error)`, `GetByHandle(ctx, string) (*domain.User, error)`, `UpdateProfile(ctx, *domain.User) (*domain.User, error)`; доменные ошибки `domain.ErrNotFound`, `domain.ErrHandleTaken`; сущность `domain.User{ID uuid.UUID; Name, Handle, Bio, City, Work string; Verified bool; AvatarURL *string; CreatedAt, UpdatedAt time.Time}`.
- Produces: `grpc.NewUserHandler(svc *service.UserService) *grpc.UserHandler` — на него опирается Task 3 (регистрация в сервере).

- [ ] **Step 1: Добавить зависимости**

Из `D:\social_network_golang\backend\user-service`:

```powershell
go get google.golang.org/grpc@latest
go get google.golang.org/protobuf@latest
go mod tidy
```

Затем сверить: версии `google.golang.org/grpc` и `google.golang.org/protobuf` в `user-service/go.mod` **обязаны совпасть** с версиями в `backend/pkg/go.mod` (см. отчёт Task 1). Если разошлись — привести к одной явным `go get <module>@<version>`.

- [ ] **Step 2: Написать хендлер**

Создать `backend/user-service/internal/transport/grpc/user.go`:

```go
package grpc

import (
	"context"
	"errors"
	"strings"

	userv1 "ahishka/pkg/gen/ahishka/user/v1"
	"user-service/internal/domain"
	"user-service/internal/service"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ userv1.UserServiceServer = (*UserHandler)(nil)

type UserHandler struct {
	// Встраивание обязательно: оно даёт forward-compatibility, когда в
	// контракт добавят новый rpc, а этот хендлер ещё его не реализует.
	userv1.UnimplementedUserServiceServer

	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{
		svc: svc,
	}
}

func (h *UserHandler) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "id must be a uuid")
	}

	user, err := h.svc.GetByID(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.GetUserResponse{User: toProto(user)}, nil
}

func (h *UserHandler) GetUserByHandle(ctx context.Context, req *userv1.GetUserByHandleRequest) (*userv1.GetUserByHandleResponse, error) {
	handle := strings.TrimSpace(req.GetHandle())
	if handle == "" {
		return nil, status.Error(codes.InvalidArgument, "handle must not be empty")
	}

	user, err := h.svc.GetByHandle(ctx, handle)
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.GetUserByHandleResponse{User: toProto(user)}, nil
}

func (h *UserHandler) UpdateProfile(ctx context.Context, req *userv1.UpdateProfileRequest) (*userv1.UpdateProfileResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "id must be a uuid")
	}

	handle := strings.TrimSpace(req.GetHandle())
	if handle == "" {
		return nil, status.Error(codes.InvalidArgument, "handle must not be empty")
	}

	if _, err := h.svc.UpdateProfile(ctx, &domain.User{
		ID:     id,
		Name:   req.GetName(),
		Handle: handle,
		Bio:    req.GetBio(),
		City:   req.GetCity(),
		Work:   req.GetWork(),
	}); err != nil {
		return nil, mapError(err)
	}

	// Перечитываем: service.UpdateProfile возвращает ту же сущность, что ему
	// передали, а в неё не входят verified/avatar_url/created_at (репозиторий
	// их не обновляет и не читает при UPDATE). Без перечитывания ответ врал бы
	// нулями по этим полям.
	updated, err := h.svc.GetByID(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.UpdateProfileResponse{User: toProto(updated)}, nil
}

// mapError переводит доменные ошибки в gRPC-коды. Наружу не отдаём текст
// ошибки от БД — только фиксированные сообщения (как writeError в auth).
func mapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, "user not found")
	case errors.Is(err, domain.ErrHandleTaken):
		return status.Error(codes.AlreadyExists, "handle already taken")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func toProto(user *domain.User) *userv1.User {
	return &userv1.User{
		Id:        user.ID.String(),
		Name:      user.Name,
		Handle:    user.Handle,
		Bio:       user.Bio,
		City:      user.City,
		Work:      user.Work,
		Verified:  user.Verified,
		AvatarUrl: user.AvatarURL,
		CreatedAt: timestamppb.New(user.CreatedAt),
		UpdatedAt: timestamppb.New(user.UpdatedAt),
	}
}
```

- [ ] **Step 3: Собрать и проверить**

Из `backend/user-service`:

Run: `go build ./internal/... ; go vet ./internal/...`
Expected: обе команды без вывода. Ассерт `var _ userv1.UserServiceServer = (*UserHandler)(nil)` не скомпилируется, если сигнатура разошлась с контрактом — это и есть проверка соответствия proto.

- [ ] **Step 4: Commit**

```bash
git add backend/user-service/go.mod backend/user-service/go.sum backend/user-service/internal/transport/grpc/user.go backend/go.work.sum
git commit -m "user-service: add grpc handler over user service"
```

---

### Task 3: Сборка сервера, end-to-end тесты, ручная проверка reflection

**Files:**
- Create: `backend/user-service/internal/transport/grpc/server.go`
- Create: `backend/user-service/internal/transport/grpc/user_test.go`

**Interfaces:**
- Consumes: `grpc.NewUserHandler(svc *service.UserService) *grpc.UserHandler` (Task 2), `userv1.RegisterUserServiceServer` и `userv1.NewUserServiceClient` (Task 1).
- Produces: `grpc.NewServer(handler *UserHandler) *google_grpc.Server` — **на это опирается следующий план** (`cmd/user/main.go`): он делает `net.Listen` на порт из конфига, зовёт `srv.Serve(lis)` в `OnStart` и `srv.GracefulStop()` в `OnStop`. Импорт в main.go потребует алиаса, т.к. имена пакетов совпадают: `usergrpc "user-service/internal/transport/grpc"`.

- [ ] **Step 1: Написать сборку сервера**

Создать `backend/user-service/internal/transport/grpc/server.go`:

```go
package grpc

import (
	userv1 "ahishka/pkg/gen/ahishka/user/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewServer собирает сконфигурированный gRPC-сервер. Слушающий сокет и
// lifecycle — не здесь: их заводит cmd/user/main.go через fx.
//
// reflection нужен, чтобы grpcurl и подобные клиенты работали без
// раздачи .proto файлов вручную.
func NewServer(handler *UserHandler) *grpc.Server {
	srv := grpc.NewServer()
	userv1.RegisterUserServiceServer(srv, handler)
	reflection.Register(srv)
	return srv
}
```

- [ ] **Step 2: Написать end-to-end тесты через bufconn**

`bufconn` —ин-мемори listener из grpc-go: поднимает **настоящий** gRPC-сервер и настоящего клиента поверх буфера в памяти, без портов и без сети. Это ловит то, что юнит-тест хендлера пропустил бы: реальную сериализацию protobuf, реальный проброс кодов ошибок через транспорт, реальную регистрацию сервиса.

Создать `backend/user-service/internal/transport/grpc/user_test.go`:

```go
package grpc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	userv1 "ahishka/pkg/gen/ahishka/user/v1"
	"user-service/internal/domain"
	"user-service/internal/service"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeRepo реализует domain.UserRepository без БД: отдаёт заранее
// заданного пользователя и заранее заданные ошибки.
type fakeRepo struct {
	user      *domain.User
	getErr    error
	updateErr error
}

func (f *fakeRepo) Create(ctx context.Context, user *domain.User) error { return nil }

func (f *fakeRepo) Update(ctx context.Context, user *domain.User) error { return f.updateErr }

func (f *fakeRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.user, nil
}

func (f *fakeRepo) GetByHandle(ctx context.Context, handle string) (*domain.User, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.user, nil
}

// newTestClient поднимает настоящий gRPC-сервер поверх bufconn и возвращает
// подключённого к нему клиента. Всё закрывается через t.Cleanup.
func newTestClient(t *testing.T, repo domain.UserRepository) userv1.UserServiceClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	srv := NewServer(NewUserHandler(service.NewUserService(repo)))

	go func() {
		if err := srv.Serve(lis); err != nil {
			// после srv.Stop() Serve возвращает ошибку — это нормально
			return
		}
	}()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}

	t.Cleanup(func() {
		conn.Close()
		srv.Stop()
		lis.Close()
	})

	return userv1.NewUserServiceClient(conn)
}

func testUser() *domain.User {
	avatar := "https://example.com/a.png"
	return &domain.User{
		ID:        uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		Name:      "Тест Тестов",
		Handle:    "u_11111111",
		Bio:       "bio",
		City:      "Москва",
		Work:      "работа",
		Verified:  true,
		AvatarURL: &avatar,
		CreatedAt: time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 6, 13, 0, 0, 0, time.UTC),
	}
}

// requireCode достаёт gRPC-код из ошибки и сравнивает с ожидаемым.
func requireCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %v, got nil", want)
	}
	if got := status.Code(err); got != want {
		t.Fatalf("expected code %v, got %v (err: %v)", want, got, err)
	}
}

func TestGetUserReturnsProfile(t *testing.T) {
	want := testUser()
	client := newTestClient(t, &fakeRepo{user: want})

	resp, err := client.GetUser(context.Background(), &userv1.GetUserRequest{Id: want.ID.String()})
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}

	got := resp.GetUser()
	if got.GetId() != want.ID.String() {
		t.Errorf("id: got %q, want %q", got.GetId(), want.ID.String())
	}
	if got.GetName() != want.Name {
		t.Errorf("name: got %q, want %q", got.GetName(), want.Name)
	}
	if got.GetHandle() != want.Handle {
		t.Errorf("handle: got %q, want %q", got.GetHandle(), want.Handle)
	}
	if !got.GetVerified() {
		t.Error("verified: got false, want true")
	}
	if got.GetAvatarUrl() != *want.AvatarURL {
		t.Errorf("avatar_url: got %q, want %q", got.GetAvatarUrl(), *want.AvatarURL)
	}
	if !got.GetCreatedAt().AsTime().Equal(want.CreatedAt) {
		t.Errorf("created_at: got %v, want %v", got.GetCreatedAt().AsTime(), want.CreatedAt)
	}
}

func TestGetUserRejectsMalformedID(t *testing.T) {
	client := newTestClient(t, &fakeRepo{user: testUser()})

	_, err := client.GetUser(context.Background(), &userv1.GetUserRequest{Id: "not-a-uuid"})
	requireCode(t, err, codes.InvalidArgument)
}

func TestGetUserMapsNotFound(t *testing.T) {
	client := newTestClient(t, &fakeRepo{getErr: domain.ErrNotFound})

	_, err := client.GetUser(context.Background(), &userv1.GetUserRequest{
		Id: uuid.New().String(),
	})
	requireCode(t, err, codes.NotFound)
}

func TestGetUserHidesInternalErrors(t *testing.T) {
	client := newTestClient(t, &fakeRepo{getErr: errors.New("pq: connection refused on 10.0.0.5")})

	_, err := client.GetUser(context.Background(), &userv1.GetUserRequest{
		Id: uuid.New().String(),
	})
	requireCode(t, err, codes.Internal)
	if msg := status.Convert(err).Message(); msg != "internal error" {
		t.Errorf("internal error leaked to client: %q", msg)
	}
}

func TestGetUserByHandleReturnsProfile(t *testing.T) {
	want := testUser()
	client := newTestClient(t, &fakeRepo{user: want})

	resp, err := client.GetUserByHandle(context.Background(), &userv1.GetUserByHandleRequest{
		Handle: want.Handle,
	})
	if err != nil {
		t.Fatalf("GetUserByHandle: %v", err)
	}
	if resp.GetUser().GetHandle() != want.Handle {
		t.Errorf("handle: got %q, want %q", resp.GetUser().GetHandle(), want.Handle)
	}
}

func TestGetUserByHandleRejectsEmptyHandle(t *testing.T) {
	client := newTestClient(t, &fakeRepo{user: testUser()})

	_, err := client.GetUserByHandle(context.Background(), &userv1.GetUserByHandleRequest{
		Handle: "   ",
	})
	requireCode(t, err, codes.InvalidArgument)
}

func TestUpdateProfileReturnsFreshUser(t *testing.T) {
	stored := testUser()
	client := newTestClient(t, &fakeRepo{user: stored})

	resp, err := client.UpdateProfile(context.Background(), &userv1.UpdateProfileRequest{
		Id:     stored.ID.String(),
		Name:   "Новое Имя",
		Handle: stored.Handle,
		Bio:    "новое bio",
		City:   "Питер",
		Work:   "новая работа",
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	// Ответ собирается из перечитанного пользователя, поэтому поля, которые
	// UPDATE не трогает, приходят настоящими, а не нулевыми.
	if !resp.GetUser().GetVerified() {
		t.Error("verified: got false, want true (must survive update)")
	}
	if !resp.GetUser().GetCreatedAt().AsTime().Equal(stored.CreatedAt) {
		t.Errorf("created_at: got %v, want %v (must survive update)",
			resp.GetUser().GetCreatedAt().AsTime(), stored.CreatedAt)
	}
}

func TestUpdateProfileMapsHandleTaken(t *testing.T) {
	client := newTestClient(t, &fakeRepo{user: testUser(), updateErr: domain.ErrHandleTaken})

	_, err := client.UpdateProfile(context.Background(), &userv1.UpdateProfileRequest{
		Id:     testUser().ID.String(),
		Handle: "taken_handle",
	})
	requireCode(t, err, codes.AlreadyExists)
}

func TestUpdateProfileRejectsMalformedID(t *testing.T) {
	client := newTestClient(t, &fakeRepo{user: testUser()})

	_, err := client.UpdateProfile(context.Background(), &userv1.UpdateProfileRequest{
		Id:     "not-a-uuid",
		Handle: "some_handle",
	})
	requireCode(t, err, codes.InvalidArgument)
}
```

- [ ] **Step 3: Прогнать тесты**

Из `backend/user-service`:

Run: `go test ./internal/transport/grpc/... -v`
Expected: все 9 тестов PASS, вывод чистый (без предупреждений), общее время — доли секунды.

Если `TestUpdateProfileReturnsFreshUser` падает на `verified: got false` — значит хендлер отдаёт результат `service.UpdateProfile` напрямую вместо перечитывания через `GetByID` (см. комментарий в Task 2 Step 2).

- [ ] **Step 4: Прогнать весь модуль**

Run: `go build ./internal/... ; go vet ./internal/... ; go test ./internal/...`
Expected: build/vet без вывода; тесты — `ok` по `internal/consumer` и `internal/transport/grpc`, `no test files` по остальным пакетам.

- [ ] **Step 5: Проверить reflection руками через grpcurl**

`bufconn`-тесты не покрывают reflection (клиент знает контракт из сгенерённого кода) — а именно на него завязан весь дев-тулинг. Проверяем отдельно.

Поставить grpcurl:

```powershell
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

Создать **временный** файл `backend/user-service/cmd/devgrpc/main.go`:

```go
// ВРЕМЕННЫЙ ФАЙЛ: ручная проверка reflection через grpcurl.
// Удаляется сразу после проверки, в коммит не попадает.
package main

import (
	"context"
	"log"
	"net"

	"user-service/internal/domain"
	"user-service/internal/service"
	usergrpc "user-service/internal/transport/grpc"

	"github.com/google/uuid"
)

type stubRepo struct{}

func (stubRepo) Create(context.Context, *domain.User) error { return nil }
func (stubRepo) Update(context.Context, *domain.User) error { return nil }
func (stubRepo) GetByID(context.Context, uuid.UUID) (*domain.User, error) {
	return nil, domain.ErrNotFound
}
func (stubRepo) GetByHandle(context.Context, string) (*domain.User, error) {
	return nil, domain.ErrNotFound
}

func main() {
	lis, err := net.Listen("tcp", "127.0.0.1:50051")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	srv := usergrpc.NewServer(usergrpc.NewUserHandler(service.NewUserService(stubRepo{})))
	log.Println("serving on 127.0.0.1:50051")
	log.Fatal(srv.Serve(lis))
}
```

Запустить его в фоне (`go run ./cmd/devgrpc`), затем в другом окне:

```powershell
grpcurl -plaintext 127.0.0.1:50051 list
grpcurl -plaintext 127.0.0.1:50051 describe ahishka.user.v1.UserService
grpcurl -plaintext -d '{\"id\": \"11111111-2222-3333-4444-555555555555\"}' 127.0.0.1:50051 ahishka.user.v1.UserService/GetUser
```

Expected:
- `list` печатает `ahishka.user.v1.UserService` и `grpc.reflection.v1.ServerReflection`.
- `describe` печатает три метода: `GetUser`, `GetUserByHandle`, `UpdateProfile`.
- Третья команда возвращает ошибку `NotFound: user not found` — это **успех**: stub-репозиторий всегда отдаёт `ErrNotFound`, значит запрос дошёл через reflection до реального хендлера и код ошибки замаплен верно.

Если `list` выдаёт `server does not support the reflection API` — в `NewServer` не вызван `reflection.Register(srv)`.

- [ ] **Step 6: Убрать временный файл**

Остановить фоновый сервер, затем:

Run: `Remove-Item -Recurse -Force .\cmd\devgrpc`
Затем: `go build ./internal/...` — Expected: без вывода.
Затем: `git status --short` — Expected: `cmd/devgrpc` отсутствует.

- [ ] **Step 7: Commit**

```bash
git add backend/user-service/internal/transport/grpc/server.go backend/user-service/internal/transport/grpc/user_test.go
git commit -m "user-service: add grpc server assembly and bufconn tests"
```

---

## Что НЕ входит в этот план

- **`cmd/user/main.go`** (следующий шаг roadmap): fx-склейка — pool, NATS + `events.EnsureEventsStream`, `repo→service→consumer`, `net.Listen` + `NewServer(...).Serve()` в `OnStart` / `GracefulStop()` в `OnStop`, `GRPC_PORT` в `internal/config`. + Dockerfile user-service (context `backend/`, обязательно — иначе `ahishka/pkg` не попадёт в образ).
- **Аутентификация/авторизация на gRPC** (кто имеет право звать `UpdateProfile`): по архитектуре JWT валидирует gateway и прокидывает `X-User-Id`, user-service ему доверяет. Interceptor'ы — когда появится gateway.
- **Перевод событий на proto** — `pkg/events` остаётся на JSON+Go-структурах, как решено; миграция контрактов событий на proto это отдельное решение.
- **Валидация длин полей** (`name`/`bio`/`city`/`work`): сейчас проверяются только uuid и непустой handle. Полноценные лимиты — когда появится DTO-слой gateway, чтобы не разъезжались правила в двух местах.
