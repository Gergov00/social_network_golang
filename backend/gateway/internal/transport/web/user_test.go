package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	userv1 "ahishka/pkg/gen/ahishka/user/v1"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeUserClient подменяет gRPC-клиента user-service: отдаёт заготовленного
// пользователя или ошибку и запоминает последний запрос каждого метода, чтобы
// тесты могли проверить, что хендлер на самом деле отправил, а не только что
// он вернул клиенту.
type fakeUserClient struct {
	user            *userv1.User
	err             error
	lastGet         *userv1.GetUserRequest
	lastGetByHandle *userv1.GetUserByHandleRequest
	lastUpdate      *userv1.UpdateProfileRequest
}

func (f *fakeUserClient) GetUser(ctx context.Context, in *userv1.GetUserRequest, opts ...grpc.CallOption) (*userv1.GetUserResponse, error) {
	f.lastGet = in
	if f.err != nil {
		return nil, f.err
	}
	return &userv1.GetUserResponse{User: f.user}, nil
}

func (f *fakeUserClient) GetUserByHandle(ctx context.Context, in *userv1.GetUserByHandleRequest, opts ...grpc.CallOption) (*userv1.GetUserByHandleResponse, error) {
	f.lastGetByHandle = in
	if f.err != nil {
		return nil, f.err
	}
	return &userv1.GetUserByHandleResponse{User: f.user}, nil
}

func (f *fakeUserClient) UpdateProfile(ctx context.Context, in *userv1.UpdateProfileRequest, opts ...grpc.CallOption) (*userv1.UpdateProfileResponse, error) {
	f.lastUpdate = in
	if f.err != nil {
		return nil, f.err
	}
	return &userv1.UpdateProfileResponse{User: f.user}, nil
}

func testUser(id uuid.UUID) *userv1.User {
	avatar := "https://example.com/a.png"
	return &userv1.User{
		Id:        id.String(),
		Name:      "Тест Тестов",
		Handle:    "u_" + id.String()[:8],
		Bio:       "bio",
		City:      "Москва",
		Work:      "работа",
		Verified:  true,
		AvatarUrl: &avatar,
		CreatedAt: timestamppb.New(time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)),
		UpdatedAt: timestamppb.New(time.Date(2026, 8, 6, 13, 0, 0, 0, time.UTC)),
	}
}

// requestWithUser собирает запрос так, будто RequireAuth уже отработал.
func requestWithUser(method, target string, body string, id uuid.UUID) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	return req.WithContext(context.WithValue(req.Context(), userIDKey, id))
}

func TestMeReturnsProfileOfTokenOwner(t *testing.T) {
	id := uuid.New()
	client := &fakeUserClient{user: testUser(id)}
	rec := httptest.NewRecorder()

	NewUserHandler(client).Me(rec, requestWithUser(http.MethodGet, "/me", "", id))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var got userDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.ID != id.String() {
		t.Errorf("id: got %q, want %q", got.ID, id.String())
	}
	if !got.Verified {
		t.Error("verified: got false, want true")
	}
	if got.AvatarURL == nil || *got.AvatarURL != "https://example.com/a.png" {
		t.Errorf("avatarUrl: got %v", got.AvatarURL)
	}

	// SECURITY: the fake returns a preset user regardless of what Me actually
	// sent, so the assertions above would pass even if Me sent the wrong id
	// (or none at all). Assert on the outgoing gRPC request itself to prove
	// the id going upstream is genuinely the token owner's.
	if client.lastGet == nil || client.lastGet.GetId() != id.String() {
		t.Fatalf("SECURITY: GetUser request went out with id %q, must be token owner %q",
			client.lastGet.GetId(), id.String())
	}
}

func TestMeWithoutUserInContextIsUnauthorized(t *testing.T) {
	client := &fakeUserClient{user: testUser(uuid.New())}
	rec := httptest.NewRecorder()

	// Без RequireAuth в контексте нет id — хендлер обязан отказать, а не
	// сходить в user-service с пустым идентификатором.
	NewUserHandler(client).Me(rec, httptest.NewRequest(http.MethodGet, "/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rec.Code)
	}
}

func TestMeMapsNotFoundTo404(t *testing.T) {
	id := uuid.New()
	client := &fakeUserClient{err: status.Error(codes.NotFound, "user not found")}
	rec := httptest.NewRecorder()

	NewUserHandler(client).Me(rec, requestWithUser(http.MethodGet, "/me", "", id))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
}

func TestMeHidesUpstreamErrorDetails(t *testing.T) {
	id := uuid.New()
	client := &fakeUserClient{err: status.Error(codes.Internal, "pq: connection refused on 10.0.0.5")}
	rec := httptest.NewRecorder()

	NewUserHandler(client).Me(rec, requestWithUser(http.MethodGet, "/me", "", id))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("upstream error leaked to client: %s", rec.Body.String())
	}
}

// Ключевой тест безопасности: id для обновления берётся из токена, а не из тела.
func TestUpdateMeIgnoresIDFromBody(t *testing.T) {
	tokenOwner := uuid.New()
	victim := uuid.New()
	client := &fakeUserClient{user: testUser(tokenOwner)}
	rec := httptest.NewRecorder()

	body := `{"id": "` + victim.String() + `", "city": "Берлин"}`
	NewUserHandler(client).UpdateMe(rec, requestWithUser(http.MethodPatch, "/me", body, tokenOwner))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if client.lastUpdate.GetId() != tokenOwner.String() {
		t.Fatalf("SECURITY: update went to %q, must be token owner %q",
			client.lastUpdate.GetId(), tokenOwner.String())
	}
}

func TestUpdateMeSendsOnlyPresentFields(t *testing.T) {
	id := uuid.New()
	client := &fakeUserClient{user: testUser(id)}
	rec := httptest.NewRecorder()

	NewUserHandler(client).UpdateMe(rec, requestWithUser(http.MethodPatch, "/me", `{"city": "Берлин"}`, id))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if client.lastUpdate.City == nil || client.lastUpdate.GetCity() != "Берлин" {
		t.Errorf("city: got %v, want Берлин", client.lastUpdate.City)
	}
	// Отсутствующие в теле поля обязаны уехать как nil, иначе partial update
	// в user-service затрёт их пустыми строками.
	if client.lastUpdate.Name != nil {
		t.Errorf("name must be nil when absent from body, got %q", client.lastUpdate.GetName())
	}
	if client.lastUpdate.Bio != nil {
		t.Errorf("bio must be nil when absent from body, got %q", client.lastUpdate.GetBio())
	}
}

func TestUpdateMeRejectsMalformedJSON(t *testing.T) {
	id := uuid.New()
	client := &fakeUserClient{user: testUser(id)}
	rec := httptest.NewRecorder()

	NewUserHandler(client).UpdateMe(rec, requestWithUser(http.MethodPatch, "/me", `{"city":`, id))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
}

// TestUpdateMeRejectsOversizedBody guards the maxBodyBytes limit: a body
// past the cap must be rejected with 400, not read into memory without
// bound. The oversized body is built as a huge value for a real field
// (city) rather than padding/garbage, so this fails for the right reason
// (MaxBytesReader tripping mid-decode) rather than merely producing
// unrelated invalid JSON.
func TestUpdateMeRejectsOversizedBody(t *testing.T) {
	id := uuid.New()
	client := &fakeUserClient{user: testUser(id)}
	rec := httptest.NewRecorder()

	oversized := `{"city": "` + strings.Repeat("a", maxBodyBytes+1) + `"}`
	if len(oversized) <= maxBodyBytes {
		t.Fatalf("test body (%d bytes) does not actually exceed maxBodyBytes (%d)", len(oversized), maxBodyBytes)
	}

	NewUserHandler(client).UpdateMe(rec, requestWithUser(http.MethodPatch, "/me", oversized, id))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
	if client.lastUpdate != nil {
		t.Error("SECURITY/RESOURCE: oversized body must be rejected before reaching the upstream call")
	}
}

func TestUserByHandleReturnsProfile(t *testing.T) {
	id := uuid.New()
	user := testUser(id)
	client := &fakeUserClient{user: user}
	rec := httptest.NewRecorder()

	req := requestWithUser(http.MethodGet, "/users/"+user.GetHandle(), "", id)
	req.SetPathValue("handle", user.GetHandle())
	NewUserHandler(client).UserByHandle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var got userDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.Handle != user.GetHandle() {
		t.Errorf("handle: got %q, want %q", got.Handle, user.GetHandle())
	}
	if client.lastGetByHandle == nil || client.lastGetByHandle.GetHandle() != user.GetHandle() {
		t.Errorf("GetUserByHandle request: got handle %q, want %q",
			client.lastGetByHandle.GetHandle(), user.GetHandle())
	}
}

func TestUserByHandleRejectsEmptyHandle(t *testing.T) {
	id := uuid.New()
	client := &fakeUserClient{user: testUser(id)}
	rec := httptest.NewRecorder()

	req := requestWithUser(http.MethodGet, "/users/", "", id)
	req.SetPathValue("handle", "")
	NewUserHandler(client).UserByHandle(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
}
