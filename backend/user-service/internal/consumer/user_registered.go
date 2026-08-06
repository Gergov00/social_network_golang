package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"ahishka/pkg/events"
	"user-service/internal/domain"
	"user-service/internal/service"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	streamName  = "EVENTS"
	durableName = "user-service-user-registered"

	// handleTimeout ограничивает время обработки одного сообщения, чтобы
	// зависший CreateProfile/поход в БД не остановил весь consume-цикл
	// навечно (Consume гоняет handler синхронно, одно сообщение за раз, в
	// одной диспатч-горутине). Держим ниже AckWait (30s), чтобы не растягивать
	// время до истечения ack-окна сверх меры.
	handleTimeout = 25 * time.Second

	// nakFallbackDelay — задержка редоставки, когда не удалось прочитать
	// msg.Metadata() (NumDelivered), поэтому backoff по номеру попытки
	// посчитать нельзя.
	nakFallbackDelay = 5 * time.Second

	// nakMaxDelay ограничивает экспоненциальный backoff, чтобы задержка
	// редоставки не уходила далеко за пределы окрестности AckWait.
	nakMaxDelay = 30 * time.Second
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

	return cons.Consume(c.handle, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		// Это ошибка самого consume-цикла (пропущенные heartbeat'ы, консьюмер
		// удалён на сервере и т.п.), а не сбой обработки одного сообщения.
		// Consume может после такого перестать доставлять сообщения молча —
		// логируем громко, чтобы это было видно в мониторинге.
		c.logger.Error("user.registered: jetstream consume loop error", "error", err)
	}))
}

func (c *UserRegisteredConsumer) handle(msg jetstream.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), handleTimeout)
	defer cancel()

	var payload events.UserRegistered
	if err := json.Unmarshal(msg.Data(), &payload); err != nil {
		c.logger.Error("user.registered: bad payload, dropping",
			"error", err, "subject", msg.Subject(), "stream_seq", streamSeq(msg))
		_ = msg.Term()
		return
	}

	id, err := uuid.Parse(payload.ID)
	if err != nil {
		c.logger.Error("user.registered: bad id, dropping",
			"raw_id", payload.ID, "error", err, "subject", msg.Subject(), "stream_seq", streamSeq(msg))
		_ = msg.Term()
		return
	}

	// Запись в БД -> потом Ack. Если CreateProfile не отработал, сообщение
	// НЕ подтверждается: JetStream передоставит его (MaxDeliver=5), а
	// Create в репозитории идемпотентен (ON CONFLICT DO NOTHING).
	if _, err := c.svc.CreateProfile(ctx, id); err != nil {
		if errors.Is(err, domain.ErrHandleTaken) {
			// Детерминированная ошибка: повтор с тем же input даст тот же
			// результат каждый раз, ретраи бессмысленны (только съедят
			// MaxDeliver). Это реальная аномалия (коллизия стаб-хендла) —
			// стоит внимания человека.
			c.logger.Error("user.registered: handle collision, terminating",
				"id", id.String(), "error", err, "subject", msg.Subject(), "stream_seq", streamSeq(msg))
			_ = msg.Term()
			return
		}

		delay := nakDelay(msg)
		c.logger.Error("user.registered: CreateProfile failed, will retry",
			"id", id.String(), "error", err, "delay", delay)
		_ = msg.NakWithDelay(delay)
		return
	}

	if err := msg.Ack(); err != nil {
		c.logger.Error("user.registered: ack failed", "id", id.String(), "error", err)
	}
}

// streamSeq возвращает stream-sequence сообщения для логов (0, если
// метаданные недоступны), чтобы дропнутое сообщение можно было потом найти
// в стриме вручную.
func streamSeq(msg jetstream.Msg) uint64 {
	md, err := msg.Metadata()
	if err != nil {
		return 0
	}
	return md.Sequence.Stream
}

// nakDelay считает задержку редоставки по числу уже сделанных попыток
// (экспоненциальный backoff, capped на nakMaxDelay). Если метаданные
// недоступны, отступаем на фиксированную задержку вместо провала хендлера.
func nakDelay(msg jetstream.Msg) time.Duration {
	md, err := msg.Metadata()
	if err != nil {
		return nakFallbackDelay
	}

	delay := time.Duration(1<<md.NumDelivered) * time.Second
	if delay > nakMaxDelay || delay <= 0 {
		delay = nakMaxDelay
	}
	return delay
}
