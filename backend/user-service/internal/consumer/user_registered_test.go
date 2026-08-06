package consumer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"user-service/internal/domain"
	"user-service/internal/service"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// fakeMsg — минимальная реализация jetstream.Msg для юнит-тестов handle:
// без реального NATS-соединения, только запись того, какой терминальный
// метод (Ack/Nak/NakWithDelay/Term) был вызван и с каким аргументом.
type fakeMsg struct {
	data    []byte
	subject string

	meta    *jetstream.MsgMetadata
	metaErr error

	acked    bool
	termed   bool
	nakked   bool
	nakDelay time.Duration
}

func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return m.meta, m.metaErr
}

func (m *fakeMsg) Data() []byte { return m.data }

func (m *fakeMsg) Headers() nats.Header { return nil }

func (m *fakeMsg) Subject() string { return m.subject }

func (m *fakeMsg) Reply() string { return "" }

func (m *fakeMsg) Ack() error {
	m.acked = true
	return nil
}

func (m *fakeMsg) DoubleAck(_ context.Context) error {
	m.acked = true
	return nil
}

func (m *fakeMsg) Nak() error {
	m.nakked = true
	m.nakDelay = 0
	return nil
}

func (m *fakeMsg) NakWithDelay(delay time.Duration) error {
	m.nakked = true
	m.nakDelay = delay
	return nil
}

func (m *fakeMsg) InProgress() error { return nil }

func (m *fakeMsg) Term() error {
	m.termed = true
	return nil
}

func (m *fakeMsg) TermWithReason(_ string) error {
	m.termed = true
	return nil
}

// fakeRepo — фейковая domain.UserRepository, возвращающая настраиваемую
// ошибку из Create, чтобы гонять CreateProfile через все ветки без БД.
type fakeRepo struct {
	createErr error
}

func (r *fakeRepo) Create(_ context.Context, _ *domain.User) error {
	return r.createErr
}

func (r *fakeRepo) Update(_ context.Context, _ *domain.User) error {
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.User, error) {
	return nil, domain.ErrNotFound
}

func (r *fakeRepo) GetByHandle(_ context.Context, _ string) (*domain.User, error) {
	return nil, domain.ErrNotFound
}

func (r *fakeRepo) GetByIDs(_ context.Context, _ []uuid.UUID) ([]domain.User, error) {
	return nil, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestConsumer(createErr error) *UserRegisteredConsumer {
	repo := &fakeRepo{createErr: createErr}
	svc := service.NewUserService(repo)
	return NewUserRegisteredConsumer(svc, testLogger())
}

func TestHandle_ValidPayload_Success_Acks(t *testing.T) {
	id := uuid.New()
	msg := &fakeMsg{
		data:    []byte(`{"id":"` + id.String() + `","email":"a@b.com"}`),
		subject: "user.registered",
		meta:    &jetstream.MsgMetadata{Sequence: jetstream.SequencePair{Stream: 1}, NumDelivered: 1},
	}

	c := newTestConsumer(nil)
	c.handle(msg)

	if !msg.acked {
		t.Errorf("expected Ack() to be called, got acked=%v", msg.acked)
	}
	if msg.nakked {
		t.Errorf("expected Nak/NakWithDelay NOT to be called")
	}
	if msg.termed {
		t.Errorf("expected Term() NOT to be called")
	}
}

func TestHandle_MalformedJSON_Terms_RepoNotCalled(t *testing.T) {
	msg := &fakeMsg{
		data:    []byte(`{not valid json`),
		subject: "user.registered",
		meta:    &jetstream.MsgMetadata{Sequence: jetstream.SequencePair{Stream: 2}, NumDelivered: 1},
	}

	repo := &fakeRepo{createErr: errors.New("must not be called")}
	svc := service.NewUserService(repo)
	c := NewUserRegisteredConsumer(svc, testLogger())

	c.handle(msg)

	if !msg.termed {
		t.Errorf("expected Term() to be called for malformed JSON")
	}
	if msg.acked || msg.nakked {
		t.Errorf("expected no Ack/Nak for malformed JSON, got acked=%v nakked=%v", msg.acked, msg.nakked)
	}
}

func TestHandle_BadUUID_Terms_RepoNotCalled(t *testing.T) {
	msg := &fakeMsg{
		data:    []byte(`{"id":"not-a-uuid","email":"a@b.com"}`),
		subject: "user.registered",
		meta:    &jetstream.MsgMetadata{Sequence: jetstream.SequencePair{Stream: 3}, NumDelivered: 1},
	}

	repo := &fakeRepo{createErr: errors.New("must not be called")}
	svc := service.NewUserService(repo)
	c := NewUserRegisteredConsumer(svc, testLogger())

	c.handle(msg)

	if !msg.termed {
		t.Errorf("expected Term() to be called for unparseable UUID")
	}
	if msg.acked || msg.nakked {
		t.Errorf("expected no Ack/Nak for bad UUID, got acked=%v nakked=%v", msg.acked, msg.nakked)
	}
}

func TestHandle_HandleTaken_Terms(t *testing.T) {
	id := uuid.New()
	msg := &fakeMsg{
		data:    []byte(`{"id":"` + id.String() + `","email":"a@b.com"}`),
		subject: "user.registered",
		meta:    &jetstream.MsgMetadata{Sequence: jetstream.SequencePair{Stream: 4}, NumDelivered: 1},
	}

	c := newTestConsumer(domain.ErrHandleTaken)
	c.handle(msg)

	if !msg.termed {
		t.Errorf("expected Term() to be called for domain.ErrHandleTaken")
	}
	if msg.acked || msg.nakked {
		t.Errorf("expected no Ack/Nak for ErrHandleTaken, got acked=%v nakked=%v", msg.acked, msg.nakked)
	}
}

func TestHandle_OtherRepoError_NaksWithDelay(t *testing.T) {
	id := uuid.New()
	msg := &fakeMsg{
		data:    []byte(`{"id":"` + id.String() + `","email":"a@b.com"}`),
		subject: "user.registered",
		meta:    &jetstream.MsgMetadata{Sequence: jetstream.SequencePair{Stream: 5}, NumDelivered: 2},
	}

	c := newTestConsumer(errors.New("db unavailable"))
	c.handle(msg)

	if !msg.nakked {
		t.Errorf("expected Nak/NakWithDelay to be called for a generic CreateProfile error")
	}
	if msg.acked || msg.termed {
		t.Errorf("expected no Ack/Term for generic error, got acked=%v termed=%v", msg.acked, msg.termed)
	}
	if msg.nakDelay <= 0 {
		t.Errorf("expected NakWithDelay to be used with a positive delay, got %v", msg.nakDelay)
	}
}

func TestHandle_OtherRepoError_MetadataUnavailable_FallsBackToFixedDelay(t *testing.T) {
	id := uuid.New()
	msg := &fakeMsg{
		data:    []byte(`{"id":"` + id.String() + `","email":"a@b.com"}`),
		subject: "user.registered",
		metaErr: errors.New("no reply subject"),
	}

	c := newTestConsumer(errors.New("db unavailable"))
	c.handle(msg)

	if !msg.nakked {
		t.Errorf("expected NakWithDelay to be called even when Metadata() errors")
	}
	if msg.nakDelay != nakFallbackDelay {
		t.Errorf("expected fallback delay %v, got %v", nakFallbackDelay, msg.nakDelay)
	}
}
