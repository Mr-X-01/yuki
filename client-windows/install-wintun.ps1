# Yuki VPN - WinTun Installation Script
# Автоматически скачивает и устанавливает WinTun драйвер

param(
    [string]$WinTunVersion = "0.14.1"
)

$ErrorActionPreference = "Stop"

Write-Host "🔧 Yuki VPN - WinTun Installation" -ForegroundColor Green
Write-Host "=================================" -ForegroundColor Green

# Проверка прав администратора
if (-NOT ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole] "Administrator")) {
    Write-Host "❌ Этот скрипт требует прав администратора!" -ForegroundColor Red
    Write-Host "Пожалуйста, запустите PowerShell от имени администратора и повторите попытку." -ForegroundColor Yellow
    exit 1
}

$WinTunUrl = "https://www.wintun.net/builds/wintun-${WinTunVersion}.zip"
$TempDir = "$env:TEMP\yuki-wintun"
$WinTunZip = "$TempDir\wintun.zip"
$System32Dir = "$env:SystemRoot\System32"
$SysWow64Dir = "$env:SystemRoot\SysWOW64"

Write-Host "📥 Скачивание WinTun v$WinTunVersion..." -ForegroundColor Cyan

# Создание временной директории
if (Test-Path $TempDir) {
    Remove-Item $TempDir -Recurse -Force
}
New-Item -ItemType Directory -Path $TempDir -Force | Out-Null

try {
    # Скачивание WinTun
    $webClient = New-Object System.Net.WebClient
    $webClient.DownloadFile($WinTunUrl, $WinTunZip)
    Write-Host "✅ WinTun скачан успешно" -ForegroundColor Green

    # Извлечение архива
    Write-Host "📦 Извлечение архива..." -ForegroundColor Cyan
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [System.IO.Compression.ZipFile]::ExtractToDirectory($WinTunZip, $TempDir)

    # Поиск файлов DLL
    $WinTunDir = Get-ChildItem -Path $TempDir -Directory | Where-Object { $_.Name -like "wintun*" } | Select-Object -First 1
    
    if (-not $WinTunDir) {
        throw "Не удалось найти директорию WinTun в архиве"
    }

    $x64DllPath = Join-Path $WinTunDir.FullName "bin\amd64\wintun.dll"
    $x86DllPath = Join-Path $WinTunDir.FullName "bin\x86\wintun.dll"

    # Проверка архитектуры системы и установка соответствующих файлов
    if ([Environment]::Is64BitOperatingSystem) {
        Write-Host "💾 Установка WinTun для x64..." -ForegroundColor Cyan
        
        # Копирование x64 версии в System32
        if (Test-Path $x64DllPath) {
            Copy-Item $x64DllPath "$System32Dir\wintun.dll" -Force
            Write-Host "✅ wintun.dll (x64) установлен в System32" -ForegroundColor Green
        } else {
            throw "Не найден wintun.dll для x64"
        }
        
        # Копирование x86 версии в SysWOW64 для совместимости
        if (Test-Path $x86DllPath) {
            Copy-Item $x86DllPath "$SysWow64Dir\wintun.dll" -Force
            Write-Host "✅ wintun.dll (x86) установлен в SysWOW64" -ForegroundColor Green
        }
    } else {
        Write-Host "💾 Установка WinTun для x86..." -ForegroundColor Cyan
        
        # Копирование x86 версии в System32
        if (Test-Path $x86DllPath) {
            Copy-Item $x86DllPath "$System32Dir\wintun.dll" -Force
            Write-Host "✅ wintun.dll (x86) установлен в System32" -ForegroundColor Green
        } else {
            throw "Не найден wintun.dll для x86"
        }
    }

    # Копирование в директорию клиента для локального использования
    $ClientDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    Copy-Item "$System32Dir\wintun.dll" "$ClientDir\wintun.dll" -Force
    Write-Host "✅ wintun.dll скопирован в директорию клиента" -ForegroundColor Green

    Write-Host ""
    Write-Host "🎉 WinTun успешно установлен!" -ForegroundColor Green
    Write-Host "Теперь можно запускать Yuki VPN клиент." -ForegroundColor Yellow

} catch {
    Write-Host "❌ Ошибка при установке WinTun: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
} finally {
    # Очистка временных файлов
    if (Test-Path $TempDir) {
        Remove-Item $TempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Write-Host ""
Write-Host "📋 Для запуска Yuki VPN клиента:" -ForegroundColor Cyan
Write-Host "   .\yuki-client.exe -config config.json" -ForegroundColor White
Write-Host ""
Write-Host "📋 Для создания конфигурационного файла:" -ForegroundColor Cyan
Write-Host "   .\yuki-client.exe -generate-config" -ForegroundColor White
