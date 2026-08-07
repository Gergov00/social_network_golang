FROM golang:1.26-alpine AS builder

WORKDIR /src

# go.work + go.mod всех модулей workspace: без них сборка в workspace-режиме
# не разрешит ahishka/pkg. Исходники auth-service при этом не нужны — только
# его go.mod, чтобы workspace был консистентен.
COPY go.work go.work.sum ./
COPY auth-service/go.mod auth-service/go.sum ./auth-service/
COPY gateway/go.mod gateway/go.sum ./gateway/
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY post-service/go.mod post-service/go.sum ./post-service/
COPY user-service/go.mod user-service/go.sum ./user-service/
RUN go mod download

COPY pkg/ ./pkg/
COPY user-service/ ./user-service/

RUN CGO_ENABLED=0 GOOS=linux go build -o /user ./user-service/cmd/user

FROM gcr.io/distroless/static-debian12

COPY --from=builder /user /user

EXPOSE 50051

ENTRYPOINT ["/user"]
