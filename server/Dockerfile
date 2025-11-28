# Multi-stage build для минимального размера образа
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Установка зависимостей
RUN apk add --no-cache git ca-certificates tzdata

# Копируем go.mod и go.sum для кэширования зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Сборка статического бинарника
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -extldflags '-static'" \
    -a -installsuffix cgo \
    -o yuki-server .

# Финальный образ
FROM scratch

# Копируем системные файлы из builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Копируем бинарник
COPY --from=builder /app/yuki-server /yuki-server

# Создаем пользователя без прав root
USER 1000:1000

# Экспонируем порты
EXPOSE 443 8443

# Точка входа
ENTRYPOINT ["/yuki-server"]
