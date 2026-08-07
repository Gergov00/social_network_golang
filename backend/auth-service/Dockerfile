FROM golang:1.26-alpine AS builder

WORKDIR /src

# go.work + go.mod всех модулей workspace: без них сборка в workspace-режиме
# не разрешит ahishka/pkg. Список обязан покрывать ВСЕ модули из go.work —
# добавили сервис в workspace, значит его go.mod нужен и здесь, иначе
# `go mod download` падает на «cannot load module listed in go.work file».
COPY go.work go.work.sum ./
COPY auth-service/go.mod auth-service/go.sum ./auth-service/
COPY gateway/go.mod gateway/go.sum ./gateway/
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY post-service/go.mod post-service/go.sum ./post-service/
COPY user-service/go.mod user-service/go.sum ./user-service/
RUN go mod download

COPY pkg/ ./pkg/
COPY auth-service/ ./auth-service/

RUN CGO_ENABLED=0 GOOS=linux go build -o /auth ./auth-service/cmd/auth

FROM gcr.io/distroless/static-debian12

COPY --from=builder /auth /auth

EXPOSE 10001

ENTRYPOINT ["/auth"]
