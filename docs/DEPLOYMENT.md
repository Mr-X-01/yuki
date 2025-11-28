# Развёртывание Yuki VPN

## Быстрая установка на Ubuntu 24

```bash
curl -sSL https://raw.githubusercontent.com/Mr-X-01/yuki/main/scripts/install.sh | bash
```

> Важно
> 
> Скрипт теперь поддерживает запуск как под обычным пользователем, так и под root. Он автоматически использует переменную `$SUDO` (пусто под root), поэтому можно безопасно запускать через `curl | bash` даже от root.

## Ручная установка

### Требования

- Ubuntu 24.04 LTS (рекомендуется)
- Go 1.23+
- Redis 7+
- Nginx
- SSL сертификат (Let's Encrypt или коммерческий)

### 1. Установка зависимостей

```bash
sudo apt update && sudo apt upgrade -y
sudo apt install -y git curl wget nginx redis-server certbot python3-certbot-nginx

# Установка Go 1.23
cd /tmp
wget https://go.dev/dl/go1.23.4.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.23.4.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### 2. Получение кода

```bash
cd /opt
sudo git clone https://github.com/Mr-X-01/yuki.git
sudo chown -R $USER:$USER yuki
cd yuki
```

### 3. Сборка сервера

```bash
cd server
go mod tidy
go build -ldflags "-s -w" -o yuki-server
```

### 4. Настройка конфигурации

```bash
# Генерация базовой конфигурации
./yuki-server --generate-config

# Редактирование конфигурации
nano config.json
```

Основные параметры для изменения:
- `server.domain` - ваш домен
- `server.cert_file` и `server.key_file` - пути к SSL сертификатам
- `auth.admin_api_key` - смените на собственный ключ
- `redis.address` - адрес Redis сервера

### 5. Получение SSL сертификата

```bash
# Для домена example.ru
sudo certbot certonly --nginx -d api.example.ru

# Или с webroot
sudo certbot certonly --webroot -w /var/www/html -d api.example.ru
```

Обновите пути к сертификатам в `config.json`:
```json
{
  "server": {
    "cert_file": "/etc/letsencrypt/live/api.example.ru/fullchain.pem",
    "key_file": "/etc/letsencrypt/live/api.example.ru/privkey.pem"
  }
}
```

### 6. Создание systemd сервиса

```bash
sudo tee /etc/systemd/system/yuki.service > /dev/null <<EOF
[Unit]
Description=Yuki VPN Server
After=network.target redis.service

[Service]
Type=simple
User=$USER  
# Примечание: install.sh автоматически выставляет пользователя запуска как $OWNER (учётка, от имени которой вызван скрипт)
WorkingDirectory=/opt/yuki/server
ExecStart=/opt/yuki/server/yuki-server
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable yuki
```

### 7. Настройка Nginx

```bash
sudo tee /etc/nginx/sites-available/yuki > /dev/null <<EOF
server {
    listen 80;
    server_name api.example.ru;
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name api.example.ru;
    
    ssl_certificate /etc/letsencrypt/live/api.example.ru/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/api.example.ru/privkey.pem;
    
    # gRPC проксирование для туннеля
    # Рекомендуется запускать gRPC-сервер на localhost:50051 и проксировать через Nginx.
    # Если сервер слушает 443 локально, измените порт здесь либо настройку сервера.
    location /tunnel.TunnelService/ {
        grpc_pass grpc://127.0.0.1:50051; # <-- при необходимости поменяйте на ваш внутренний порт
        grpc_set_header Host $host;
    }
    
    # REST API для админки
    location /admin/ {
        proxy_pass http://127.0.0.1:8443;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Real-IP \$remote_addr;
    }
    
    # Легитимные эндпоинты
    location / {
        proxy_pass http://127.0.0.1:8443;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
    }
}
EOF

$SUDO ln -sf /etc/nginx/sites-available/yuki /etc/nginx/sites-enabled/
$SUDO nginx -t && $SUDO systemctl reload nginx
```

### 8. Запуск сервисов

```bash
sudo systemctl start redis-server yuki
sudo systemctl status yuki
```

### 9. Проверка работы

```bash
# Проверка gRPC сервиса
grpcurl -insecure api.example.ru:443 tunnel.TunnelService/GetStatus

# Проверка REST API
curl -H "X-API-Key: your-api-key" https://api.example.ru/admin/stats
```

## Docker развёртывание

### 1. Подготовка

```bash
git clone https://github.com/Mr-X-01/yuki.git
cd yuki

# Создание директорий
mkdir -p data certs

# Размещение SSL сертификатов в ./certs/
```

### 2. Настройка конфигурации

```bash
cp server/config.json.example server/config.json
# Отредактируйте конфигурацию
```

### 3. Запуск

```bash
docker-compose up -d
docker-compose logs -f yuki-server
```

## Безопасность

### Файрвол

```bash
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

### Ограничение доступа к админ-панели

В Nginx добавьте ограничения по IP:
```nginx
location /admin/ {
    allow 192.168.1.0/24;
    deny all;
    proxy_pass http://127.0.0.1:8443;
}
```

### Автообновление сертификатов

```bash
# Добавьте в crontab
echo "0 12 * * * /usr/bin/certbot renew --quiet && systemctl reload nginx" | sudo crontab -
```

## Мониторинг

### Логи

```bash
# Системные логи
sudo journalctl -u yuki -f

# Логи Nginx
sudo tail -f /var/log/nginx/error.log
sudo tail -f /var/log/nginx/access.log

# Логи Redis
sudo tail -f /var/log/redis/redis-server.log
```

### Метрики

Сервер предоставляет базовые метрики через REST API:
```bash
curl -H "X-API-Key: your-key" https://api.example.ru/admin/stats
```

## Troubleshooting

### Сервер не запускается

1. Проверьте права доступа к SSL сертификатам:
```bash
sudo chmod 644 /etc/letsencrypt/live/*/fullchain.pem
sudo chmod 600 /etc/letsencrypt/live/*/privkey.pem
```

2. Проверьте, свободны ли порты:
```bash
sudo netstat -tulpn | grep :443
sudo netstat -tulpn | grep :8443
```

### Клиенты не подключаются

1. Проверьте DNS резолюцию домена
2. Проверьте правильность API ключей
3. Убедитесь, что gRPC сервис доступен:
```bash
grpcurl -insecure -d '{"service":"test"}' api.example.ru:443 tunnel.TunnelService/GetStatus
```

### Проблемы с производительностью

1. Увеличьте лимиты системы в `/etc/security/limits.conf`:
```
* soft nofile 65536
* hard nofile 65536
```

2. Настройте sysctl в `/etc/sysctl.conf`:
```
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216
```
