package domain

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCursorRoundTrip(t *testing.T) {
	// Микросекунды намеренно: Postgres timestamptz хранит именно их, и
	// курсор обязан вернуться из строки ровно тем же значением — иначе
	// сравнение (created_at, id) < ($1, $2) промахнётся мимо границы
	// страницы и пост либо продублируется, либо потеряется.
	want := FeedCursor{
		CreatedAt: time.Date(2026, 8, 6, 12, 34, 56, 123456000, time.UTC),
		ID:        uuid.MustParse("11111111-2222-3333-4444-555555555555"),
	}

	got, err := ParseCursor(want.Encode())
	if err != nil {
		t.Fatalf("ParseCursor(): %v", err)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want.CreatedAt)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %v, want %v", got.ID, want.ID)
	}
}

func TestEncodeIsOpaque(t *testing.T) {
	c := FeedCursor{CreatedAt: time.Now().UTC(), ID: uuid.New()}

	encoded := c.Encode()
	if encoded == "" {
		t.Fatal("Encode() вернул пустую строку")
	}
	// Курсор едет в query-строке: символы, требующие экранирования, там ни к чему.
	for _, r := range encoded {
		ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			t.Fatalf("Encode() дал не-URL-safe символ %q в %q", r, encoded)
		}
	}
}

func TestParseCursorRejectsGarbage(t *testing.T) {
	valid := uuid.New().String()
	tests := []struct {
		name  string
		input string
	}{
		{"не base64", "не-base64!!!"},
		{"пустая строка", ""},
		{"нет разделителя", base64.RawURLEncoding.EncodeToString([]byte("2026-08-06T12:00:00Z"))},
		{"битая дата", base64.RawURLEncoding.EncodeToString([]byte("вчера|" + valid))},
		{"битый uuid", base64.RawURLEncoding.EncodeToString([]byte("2026-08-06T12:00:00Z|не-uuid"))},
		{"лишние части", base64.RawURLEncoding.EncodeToString([]byte("2026-08-06T12:00:00Z|" + valid + "|хвост"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCursor(tt.input)
			if !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("ParseCursor(%q) err = %v, want ErrInvalidCursor", tt.input, err)
			}
			if got != nil {
				t.Errorf("ParseCursor(%q) вернул курсор %+v вместе с ошибкой", tt.input, got)
			}
		})
	}
}
