# 🏗️ Инструкция по сборке и установке Yuki VPN

## 📋 Содержание
- [Серверная часть (Linux)](#серверная-часть-linux)
- [Windows клиент](#windows-клиент)
- [Быстрый старт](#быстрый-старт)
- [Устранение неполадок](#устранение-неполадок)

---

## 🖥️ Серверная часть (Linux)

### Требования
- Ubuntu/Debian Linux (20.04+)
- Домен с настроенной DNS записью
- Root доступ к серверу
- Открытые порты: 80, 443

### Автоматическая установка (рекомендуется)

```bash
# Клонируйте репозиторий
git clone https://github.com/Mr-X-01/yuki.git
cd yuki/scripts

# Запустите установочный скрипт
sudo bash install.sh

# Следуйте инструкциям на экране
# Скрипт запросит домен и admin пароль
```

**Скрипт автоматически:**
- ✅ Установит все зависимости (Go, nginx, certbot, redis)
- ✅ Настроит SSL сертификат через Let's Encrypt
- ✅ Создаст systemd сервис
- ✅ Настроит firewall (ufw)
- ✅ Запустит сервер

### Ручная сборка сервера

```bash
# Установите зависимости
sudo apt update
sudo apt install -y golang-go git nginx certbot python3-certbot-nginx redis-server

# Клонируйте репозиторий
git clone https://github.com/Mr-X-01/yuki.git
cd yuki/server

# Соберите сервер
go mod download
go build -o yuki main.go

# Создайте конфигурационный файл
cat > config.json << EOF
{
  "grpc_port": "8443",
  "api_port": "8444",
  "api_key": "your-secure-api-key-here",
  "admin_login": "admin",
  "admin_password": "secure-password-here",
  "redis_addr": "localhost:6379",
  "tls_cert": "/etc/letsencrypt/live/yourdomain.com/fullchain.pem",
  "tls_key": "/etc/letsencrypt/live/yourdomain.com/privkey.pem"
}
EOF

# Запустите сервер
./yuki
```

### Настройка nginx

Создайте `/etc/nginx/sites-available/yuki`:

```nginx
server {
    listen 80;
    server_name yourdomain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name yourdomain.com;

    ssl_certificate /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;
    
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # gRPC tunnel service
    location /tunnel.TunnelService/ {
        grpc_pass grpc://127.0.0.1:8443;
        grpc_set_header Host $host;
        grpc_set_header X-Real-IP $remote_addr;
    }

    # Admin API
    location /admin {
        proxy_pass http://127.0.0.1:8444;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Health and other endpoints
    location / {
        proxy_pass http://127.0.0.1:8444;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Активируйте конфигурацию:
```bash
sudo ln -s /etc/nginx/sites-available/yuki /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl restart nginx
```

### Проверка работы сервера

```bash
# Проверьте статус сервиса
sudo systemctl status yuki

# Проверьте логи
sudo journalctl -u yuki -f

# Проверьте health endpoint
curl https://yourdomain.com/health

# Проверьте админ панель
curl https://yourdomain.com/admin
```

---

## 💻 Windows клиент

### Требования
- Windows 10/11
- Go 1.21+ (для сборки)
- Права администратора (для работы)

### Сборка клиента

```powershell
# Перейдите в директорию клиента
cd client-windows

# Установите зависимости
go mod download

# Соберите клиент
go build -o yuki-client.exe .
```

### Подготовка конфигурации

Создайте файл `yuki.json` рядом с `yuki-client.exe`:

```json
{
  "server_address": "yourdomain.com:8443",
  "client_id": "your-client-id-from-admin-panel",
  "client_secret": "your-client-secret",
  "protocol": "grpc",
  "encryption": "xchacha20-poly1305"
}
```

**Получение client_id и client_secret:**
1. Откройте админ панель: `https://yourdomain.com/admin`
2. Войдите с учетными данными администратора
3. Создайте нового клиента
4. Скачайте конфигурацию или скопируйте credentials

### Запуск клиента

**⚠️ ВАЖНО: Клиент должен запускаться с правами администратора!**

**Способ 1: PowerShell с правами администратора**
```powershell
# Запустите PowerShell от имени администратора
cd C:\path\to\client-windows
.\yuki-client.exe
```

**Способ 2: Автоматический запуск с UAC**
```powershell
powershell -Command "Start-Process cmd -ArgumentList '/k cd /d C:\path\to\client-windows && yuki-client.exe' -Verb RunAs"
```

**Способ 3: Создайте .bat файл**

Создайте `run-vpn.bat`:
```batch
@echo off
cd /d "%~dp0"
powershell -Command "Start-Process cmd -ArgumentList '/k yuki-client.exe' -Verb RunAs"
```

### Проверка подключения

После запуска клиента вы должны увидеть:
```
🌸 Yuki VPN Client Starting...
✅ Tunnel connected!
🌐 Interface: Yuki Tunnel (10.0.0.2/24)
🚀 Весь трафик теперь направляется через VPN туннель
🔍 Проверьте свой внешний IP на https://2ip.ru
💚 VPN туннель работает (IP: 10.0.0.2)
```

Проверьте IP:
1. Откройте https://2ip.ru
2. IP должен измениться на IP вашего VPN сервера

### Логи клиента

Все логи сохраняются в файл `yuki-client.log` в той же директории.

---

## 🚀 Быстрый старт

### 1. Установите сервер (5 минут)

```bash
# На сервере
git clone https://github.com/Mr-X-01/yuki.git
cd yuki/scripts
sudo bash install.sh
```

Введите:
- Ваш домен (например: vpn.example.com)
- Email для SSL сертификата
- Пароль администратора

### 2. Создайте клиента в админ панели

1. Откройте `https://yourdomain.com/admin`
2. Войдите (admin / ваш_пароль)
3. Нажмите "Создать клиента"
4. Введите имя клиента
5. Скачайте конфигурацию (`yuki-client-name.json`)

### 3. Запустите Windows клиент

```powershell
# На Windows машине
cd C:\path\to\yuki\client-windows
go build -o yuki-client.exe .

# Скопируйте скачанный файл в yuki.json
# Запустите от имени администратора
.\yuki-client.exe
```

### 4. Проверьте подключение

Откройте https://2ip.ru - IP должен показывать IP вашего VPN сервера.

---

## 🔧 Устранение неполадок

### Сервер

**Проблема: nginx не слушает порт 443**
```bash
# Проверьте конфигурацию nginx
sudo nginx -t

# Проверьте, слушает ли nginx порт 443
sudo netstat -tlnp | grep :443

# Перезапустите nginx
sudo systemctl restart nginx

# Проверьте логи
sudo tail -f /var/log/nginx/error.log
```

**Проблема: SSL сертификат не работает**
```bash
# Проверьте сертификат
sudo certbot certificates

# Обновите сертификат вручную
sudo certbot renew --force-renewal

# Перезапустите nginx
sudo systemctl restart nginx
```

**Проблема: Сервер не запускается**
```bash
# Проверьте логи
sudo journalctl -u yuki -n 50

# Проверьте конфигурацию
cat /opt/yuki/config.json

# Проверьте порты
sudo netstat -tlnp | grep -E '(8443|8444)'
```

### Windows клиент

**Проблема: Клиент сразу закрывается**
1. Проверьте логи в `yuki-client.log`
2. Убедитесь что запускаете от имени администратора
3. Проверьте наличие файла `yuki.json`

**Проблема: IP не меняется**
```powershell
# Проверьте маршруты
route print

# Проверьте сетевые интерфейсы
ipconfig /all

# Должен быть интерфейс "Yuki Tunnel" с IP 10.0.0.2
```

**Проблема: "Требуются права администратора"**
- Запускайте через правую кнопку мыши → "Запустить от имени администратора"
- Или используйте PowerShell от имени администратора

**Проблема: "Failed to create TUN interface"**
- Убедитесь что `wintun.dll` находится в той же папке
- Скачайте wintun.dll: https://www.wintun.net/
- Положите рядом с `yuki-client.exe`

### Сеть

**Проблема: Не могу подключиться к серверу**
```bash
# На сервере проверьте firewall
sudo ufw status

# Должны быть открыты порты
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
```

**Проблема: DNS не работает**
```powershell
# На клиенте проверьте DNS
ipconfig /all

# Должны быть DNS: 1.1.1.1, 8.8.8.8 на Yuki Tunnel интерфейсе
```

---

## 📚 Дополнительная информация

### Архитектура проекта

```
yuki/
├── server/              # Go сервер
│   ├── main.go         # Точка входа
│   ├── api/            # REST API endpoints
│   ├── client/         # Управление клиентами
│   └── config/         # Конфигурация
├── client-windows/     # Windows клиент
│   ├── main.go         # Точка входа
│   ├── tun/            # TUN интерфейс
│   ├── crypto/         # Шифрование
│   └── config/         # Конфигурация
├── admin-panel/        # Web админ панель
├── scripts/            # Установочные скрипты
└── docs/               # Документация
```

### Порты

- **80** - HTTP (редирект на HTTPS)
- **443** - HTTPS (nginx proxy)
- **8443** - gRPC tunnel service (внутренний, закрыт)
- **8444** - Admin API backend (внутренний, закрыт)

### Безопасность

- Весь трафик шифруется XChaCha20-Poly1305
- TLS 1.2/1.3 для HTTPS
- Аутентификация по client_id/client_secret
- Firewall настраивается автоматически

### Мониторинг

```bash
# Логи сервера
sudo journalctl -u yuki -f

# Статус сервиса
sudo systemctl status yuki

# Использование ресурсов
htop
```

---

## 🆘 Поддержка

Если возникли проблемы:
1. Проверьте логи (сервер и клиент)
2. Убедитесь что все порты открыты
3. Проверьте DNS настройки
4. Проверьте права администратора

Логи:
- Сервер: `sudo journalctl -u yuki -f`
- Клиент: `yuki-client.log`
- Nginx: `/var/log/nginx/error.log`

---

**Создано для Yuki VPN 🌸**
