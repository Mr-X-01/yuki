# Клиент Yuki VPN

## Windows Client (обновлено)

### Системные требования

- Windows 10/11 (x64/arm64)
- Права администратора для установки TUN-драйвера
- Никаких дополнительных рантаймов не требуется

### Установка WinTun

1. Скопируйте `yuki-client.exe` и скрипт `install-wintun.ps1` из каталога `client-windows/` на целевую машину.
2. Запустите PowerShell от имени администратора и выполните:

```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope Process -Force
./install-wintun.ps1
```

Скрипт автоматически скачает и установит WinTun (x64/x86) и скопирует `wintun.dll` рядом с клиентом.

### Конфигурация

Используйте пример `client-windows/config-example.json` и сохраните как `config.json` в папке с клиентом:

```json
{
  "server": {
    "address": "api.example.ru:443",
    "tls": {"enabled": true, "verify_certificate": true, "server_name": "api.example.ru"}
  },
  "auth": {"uuid": "ваш-uuid", "secret": "ваш-secret"},
  "tunnel": {
    "interface_name": "YukiVPN",
    "mtu": 1420,
    "ip": "10.8.0.2",
    "netmask": "255.255.255.0",
    "gateway": "10.8.0.1",
    "dns": ["1.1.1.1", "8.8.8.8"]
  },
  "client": {"reconnect_interval": 5, "log_level": "info"}
}
```

### Использование

#### Командная строка (PowerShell / CMD)

```batch
# Генерация примера конфигурации
yuki-client.exe -generate-config

# Подключение с конфигом
yuki-client.exe -config config.json

# Запуск в фоне (если поддерживается)
yuki-client.exe -config config.json -daemon

# Статус
yuki-client.exe -status
```

#### Интерактивный режим

Запустите `yuki-client.exe` и следуйте инструкциям:

```
🌸 Yuki VPN Client Starting...
🔌 Connecting to server...
✅ Connected to Yuki VPN server
📊 Up: 1.2 MB | Down: 15.3 MB | Connected: 0:05:23
🚀 Client is running. Press Ctrl+C to stop.
```

### Автозапуск

Добавьте в автозагрузку Windows:

1. `Win + R` → `shell:startup`
2. Создайте bat-файл:

```batch
@echo off
cd /d "C:\Path\To\Yuki"
yuki-client.exe -daemon
```

### Troubleshooting

#### TUN интерфейс не создается

1. Повторно запустите `install-wintun.ps1` от имени администратора.
2. Проверьте, что `wintun.dll` присутствует в `C:\Windows\System32` и в папке с `yuki-client.exe`.
3. Как fallback можно использовать TAP-Windows (не рекомендуется): установите драйвер из пакета OpenVPN.

2. Проверьте права администратора

#### Ошибки подключения

1. Проверьте конфигурацию:
```batch
yuki-client.exe -status
```

2. Проверьте доступность сервера:
```batch
ping api.example.ru
telnet api.example.ru 443
```

3. Проверьте файрвол Windows

#### Медленная скорость

1. Отключите антивирус временно
2. Проверьте настройки DNS в TUN интерфейсе
3. Попробуйте другой DNS сервер

## Конфигурация через QR-код (опционально)

Сгенерируйте QR-код с конфигурацией:

```json
{
  "server_address": "api.example.ru:443",
  "client_id": "uuid",
  "client_secret": "secret"
}
```

Отсканируйте в мобильном приложении или импортируйте в Windows клиент.

## API клиента (если включено)

Клиент предоставляет простой HTTP API на порту 8080:

```bash
# Статус подключения  
curl http://localhost:8080/status

# Статистика
curl http://localhost:8080/stats

# Переподключение
curl -X POST http://localhost:8080/reconnect

# Отключение  
curl -X POST http://localhost:8080/disconnect
```

Ответ `/status`:
```json
{
  "connected": true,
  "server": "api.example.ru:443",
  "uptime": 3600,
  "bytes_up": 1048576,
  "bytes_down": 16777216
}
```

## Сборка из исходников

```batch
cd client-windows
go mod tidy
go build -ldflags "-s -w" -o yuki-client.exe

# Для консольной версии
go build -ldflags "-s -w" -o yuki-client-console.exe
```

### Кросс-компиляция

```bash
# Из Linux/macOS
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o yuki-client.exe

# ARM64 версия  
GOOS=windows GOARCH=arm64 go build -ldflags "-s -w" -o yuki-client-arm64.exe
```
