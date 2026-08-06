FROM golang:1.26-alpine AS builder

WORKDIR /src

# go.work + go.mod всех модулей workspace: без них сборка в workspace-режиме
# не разрешит ahishka/pkg.
COPY go.work go.work.sum ./
COPY auth-service/go.mod auth-service/go.sum ./auth-service/
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY user-service/go.mod user-service/go.sum ./user-service/
COPY gateway/go.mod gateway/go.sum ./gateway/
RUN go mod download

COPY pkg/ ./pkg/
COPY gateway/ ./gateway/

RUN CGO_ENABLED=0 GOOS=linux go build -o /gateway ./gateway/cmd/gateway

FROM gcr.io/distroless/static-debian12

COPY --from=builder /gateway /gateway

EXPOSE 8080

ENTRYPOINT ["/gateway"]
