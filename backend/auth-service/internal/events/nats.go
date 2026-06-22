package events

import (
	"ahishka/pkg/events"
	"auth-service/internal/domain"
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

var _ domain.EventPublisher = (*Publisher)(nil)

type Publisher struct {
	js jetstream.JetStream
}

func NewPublisher(js jetstream.JetStream) *Publisher {
	return &Publisher{
		js: js,
	}
}

func (p *Publisher) PublishUserRegistered(ctx context.Context, id uuid.UUID, email string) error {
	payload := events.UserRegistered{
		ID:    id.String(),
		Email: email,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = p.js.Publish(ctx, events.SubjectUserRegistered, data, jetstream.WithMsgID(id.String()))
	return err
}
