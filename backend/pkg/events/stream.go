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
