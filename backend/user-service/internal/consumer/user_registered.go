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
