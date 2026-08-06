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
