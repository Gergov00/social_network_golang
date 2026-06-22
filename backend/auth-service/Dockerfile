FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.work go.work.sum ./
COPY auth-service/go.mod auth-service/go.sum ./auth-service/
COPY pkg/go.mod ./pkg/
COPY user-service/go.mod ./user-service/
RUN go mod download

COPY pkg/ ./pkg/
COPY auth-service/ ./auth-service/

RUN CGO_ENABLED=0 GOOS=linux go build -o /auth ./auth-service/cmd/auth

FROM gcr.io/distroless/static-debian12

COPY --from=builder /auth /auth

EXPOSE 10001

ENTRYPOINT ["/auth"]
