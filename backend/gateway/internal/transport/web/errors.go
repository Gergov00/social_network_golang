package web

import (
	"encoding/json"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type errorResponse struct {
	Error string `json:"error"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// WriteError отдаёт единый формат {"error": "..."} с текстом статуса.
// Тексты фиксированные: подробности ошибки наружу не уходят.
func WriteError(w http.ResponseWriter, r *http.Request, status int) {
	if status >= http.StatusInternalServerError {
		LoggerFrom(r.Context()).Error("request failed", "status", status)
	}
	WriteJSON(w, status, errorResponse{Error: http.StatusText(status)})
}

// WriteGRPCError переводит код ответа gRPC в HTTP-статус.
func WriteGRPCError(w http.ResponseWriter, r *http.Request, err error) {
	var httpStatus int

	switch status.Code(err) {
	case codes.NotFound:
		httpStatus = http.StatusNotFound
	case codes.InvalidArgument:
		httpStatus = http.StatusBadRequest
	case codes.AlreadyExists:
		httpStatus = http.StatusConflict
	case codes.Unauthenticated:
		httpStatus = http.StatusUnauthorized
	case codes.PermissionDenied:
		httpStatus = http.StatusForbidden
	case codes.DeadlineExceeded:
		httpStatus = http.StatusGatewayTimeout
	case codes.Unavailable:
		httpStatus = http.StatusServiceUnavailable
	default:
		httpStatus = http.StatusInternalServerError
	}

	if httpStatus >= http.StatusInternalServerError {
		LoggerFrom(r.Context()).Error("upstream grpc call failed", "error", err, "status", httpStatus)
	}
	WriteJSON(w, httpStatus, errorResponse{Error: http.StatusText(httpStatus)})
}
