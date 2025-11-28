@echo off
chcp 65001 >nul
title Yuki VPN Client

echo.
echo 🌸 Yuki VPN Client Launcher
echo ================================
echo.

cd /d "%~dp0"

:: Проверка наличия yuki-client.exe
if not exist "yuki-client.exe" (
    echo ❌ yuki-client.exe не найден!
    echo.
    echo Запустите сборку:
    echo   go build -o yuki-client.exe .
    echo.
    pause
    exit /b 1
)

:: Проверка наличия yuki.json
if not exist "yuki.json" (
    echo ❌ yuki.json не найден!
    echo.
    echo Создайте конфигурационный файл yuki.json
    echo Пример:
    echo {
    echo   "server_address": "yourdomain.com:443",
    echo   "client_id": "your-client-id",
    echo   "client_secret": "your-secret",
    echo   "protocol": "grpc",
    echo   "encryption": "xchacha20-poly1305"
    echo }
    echo.
    pause
    exit /b 1
)

:: Запуск с правами администратора
echo 📋 Запуск VPN клиента...
echo ⚠️  Будет запрошено разрешение UAC (права администратора)
echo.

powershell -Command "Start-Process cmd -ArgumentList '/k cd /d \"%~dp0\" && yuki-client.exe' -Verb RunAs"

:: Ждем немного и проверяем логи
timeout /t 3 /nobreak >nul

if exist "yuki-client.log" (
    echo.
    echo 📄 Последние 5 строк из yuki-client.log:
    echo ----------------------------------------
    powershell -Command "Get-Content 'yuki-client.log' -Tail 5"
    echo ----------------------------------------
)

echo.
echo ✅ VPN клиент запущен в отдельном окне
echo 📝 Все логи сохраняются в yuki-client.log
echo.
pause
