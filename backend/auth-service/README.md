# Проверки auth-service

Unit-проверки запускаются из этой папки:

```powershell
go build ./...
go test ./... -count=1
go vet ./...
```

Интеграционные тесты требуют отдельного PostgreSQL-сервера. На каждый тест создаётся случайная БД, применяются существующие миграции и после теста БД удаляется. Без явного `AUTH_TEST_DATABASE_URL` интеграционный запуск завершится ошибкой; пользовательские application-БД не используются.

Пример с отдельным временным контейнером (без пользовательских volumes):

```powershell
docker run --detach --name ahishka-auth-tests --publish 127.0.0.1::5432 --env POSTGRES_PASSWORD=test-only postgres:16-alpine
docker exec ahishka-auth-tests pg_isready -U postgres
$authTestPort = (docker port ahishka-auth-tests 5432).Split(':')[-1]
$env:AUTH_TEST_DATABASE_URL = "postgres://postgres:test-only@127.0.0.1:$authTestPort/postgres?sslmode=disable"
go test -tags=integration ./internal/service -count=1 -race
docker rm --force --volumes ahishka-auth-tests
Remove-Item Env:AUTH_TEST_DATABASE_URL
```

Перед тестами дождитесь успешного `pg_isready`. Флаг `-race` требует CGO и C-компилятор. Тесты проверяют SQL rollback регистрации/ротации, гонки logout/reuse с refresh, отзыв всей семьи и изоляцию двух пользователей. NATS-публикация в этих тестах подставная; доставка событий и HTTP-cookie ими не проверяются.

В `.github/workflows/ci.yml` integration job запускает PostgreSQL 16 как отдельный service и использует `AUTH_TEST_DATABASE_URL`. Обычный Go matrix запускает тесты без integration tag. HTTP unit-тесты отдельно проверяют cookie и ошибки через настоящий router с подставным AuthService; это не сетевой smoke. Token-тесты проверяют JWT и refresh-формат, без ожиданий истечения по sleep.

Из корня синтаксис workflow можно проверить `actionlint .github/workflows/ci.yml`. Proto CI закрепляет Buf 1.72.0, protoc-gen-go 1.36.11 и protoc-gen-go-grpc 1.6.2; локальный drift сравнивайте с теми же версиями.
