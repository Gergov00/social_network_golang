package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestWriteGRPCErrorMapsAllCodes проверяет каждую ветку WriteGRPCError:
// правильный HTTP-статус и то, что тело — строго {"error": "<StatusText>"},
// без утечки деталей апстрима (grpc-сообщение содержит "upstream detail" —
// специально непохоже ни на один StatusText, чтобы утечку легко было заметить).
func TestWriteGRPCErrorMapsAllCodes(t *testing.T) {
	tests := []struct {
		name       string
		code       codes.Code
		wantStatus int
	}{
		{"NotFound", codes.NotFound, http.StatusNotFound},
		{"InvalidArgument", codes.InvalidArgument, http.StatusBadRequest},
		{"AlreadyExists", codes.AlreadyExists, http.StatusConflict},
		{"Unauthenticated", codes.Unauthenticated, http.StatusUnauthorized},
		{"PermissionDenied", codes.PermissionDenied, http.StatusForbidden},
		{"DeadlineExceeded", codes.DeadlineExceeded, http.StatusGatewayTimeout},
		{"Unavailable", codes.Unavailable, http.StatusServiceUnavailable},
		{"UnmappedCode", codes.Unknown, http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := status.Error(tc.code, "upstream detail")
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/whatever", nil)

			WriteGRPCError(rec, req, err)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}

			var got errorResponse
			if decodeErr := json.Unmarshal(rec.Body.Bytes(), &got); decodeErr != nil {
				t.Fatalf("decode body: %v", decodeErr)
			}
			want := errorResponse{Error: http.StatusText(tc.wantStatus)}
			if got != want {
				t.Errorf("body: got %+v, want %+v", got, want)
			}
			if got.Error != http.StatusText(tc.wantStatus) {
				t.Errorf("error text: got %q, want %q", got.Error, http.StatusText(tc.wantStatus))
			}
		})
	}
}
