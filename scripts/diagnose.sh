#!/bin/bash
# Yuki VPN Server Diagnostics Script

echo "🔍 Yuki VPN Server Diagnostics"
echo "================================"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   SUDO="sudo"
else
   SUDO=""
fi

echo "1️⃣ Проверка системных сервисов"
echo "--------------------------------"

# Check nginx
if $SUDO systemctl is-active --quiet nginx; then
    echo -e "${GREEN}✅ nginx: Запущен${NC}"
else
    echo -e "${RED}❌ nginx: Остановлен${NC}"
    echo "  Попытка запуска: sudo systemctl start nginx"
fi

# Check yuki
if $SUDO systemctl is-active --quiet yuki; then
    echo -e "${GREEN}✅ yuki: Запущен${NC}"
else
    echo -e "${RED}❌ yuki: Остановлен${NC}"
    echo "  Попытка запуска: sudo systemctl start yuki"
fi

# Check redis
if $SUDO systemctl is-active --quiet redis-server; then
    echo -e "${GREEN}✅ redis-server: Запущен${NC}"
else
    echo -e "${YELLOW}⚠️ redis-server: Остановлен${NC}"
    echo "  Попытка запуска: sudo systemctl start redis-server"
fi

echo ""
echo "2️⃣ Проверка открытых портов"
echo "--------------------------------"

# Check port 80
if $SUDO netstat -tlnp 2>/dev/null | grep -q ':80 '; then
    echo -e "${GREEN}✅ Порт 80: Открыт${NC}"
    $SUDO netstat -tlnp | grep ':80 '
else
    echo -e "${RED}❌ Порт 80: Закрыт${NC}"
fi

# Check port 443
if $SUDO netstat -tlnp 2>/dev/null | grep -q ':443 '; then
    echo -e "${GREEN}✅ Порт 443: Открыт${NC}"
    $SUDO netstat -tlnp | grep ':443 '
else
    echo -e "${RED}❌ Порт 443: Закрыт${NC}"
fi

# Check port 8443
if $SUDO netstat -tlnp 2>/dev/null | grep -q ':8443 '; then
    echo -e "${GREEN}✅ Порт 8443: Открыт (API backend)${NC}"
else
    echo -e "${YELLOW}⚠️ Порт 8443: Закрыт${NC}"
fi

# Check port 50051
if $SUDO netstat -tlnp 2>/dev/null | grep -q ':50051 '; then
    echo -e "${GREEN}✅ Порт 50051: Открыт (gRPC)${NC}"
else
    echo -e "${YELLOW}⚠️ Порт 50051: Закрыт${NC}"
fi

echo ""
echo "3️⃣ Проверка SSL сертификатов"
echo "--------------------------------"

# Find domain from nginx config
DOMAIN=$(grep -oP 'server_name \K[^;]+' /etc/nginx/sites-available/yuki 2>/dev/null | head -1 | awk '{print $1}')

if [ -n "$DOMAIN" ]; then
    echo "Домен: $DOMAIN"
    
    if [ -d "/etc/letsencrypt/live/$DOMAIN" ]; then
        echo -e "${GREEN}✅ SSL сертификат найден${NC}"
        echo "Путь: /etc/letsencrypt/live/$DOMAIN/"
        
        # Check certificate expiration
        CERT_FILE="/etc/letsencrypt/live/$DOMAIN/fullchain.pem"
        if [ -f "$CERT_FILE" ]; then
            EXPIRY=$(openssl x509 -enddate -noout -in "$CERT_FILE" 2>/dev/null | cut -d= -f2)
            echo "Истекает: $EXPIRY"
            
            # Check if expiring soon
            EXPIRY_EPOCH=$(date -d "$EXPIRY" +%s 2>/dev/null)
            NOW_EPOCH=$(date +%s)
            DAYS_LEFT=$(( ($EXPIRY_EPOCH - $NOW_EPOCH) / 86400 ))
            
            if [ $DAYS_LEFT -lt 30 ]; then
                echo -e "${YELLOW}⚠️ Сертификат истекает через $DAYS_LEFT дней${NC}"
            else
                echo -e "${GREEN}✅ Сертификат действителен ($DAYS_LEFT дней)${NC}"
            fi
        fi
    else
        echo -e "${RED}❌ SSL сертификат не найден для $DOMAIN${NC}"
        echo "  Получите сертификат: sudo certbot certonly --webroot -w /var/www/html -d $DOMAIN"
    fi
else
    echo -e "${YELLOW}⚠️ Не удалось определить домен${NC}"
fi

echo ""
echo "4️⃣ Проверка конфигурации nginx"
echo "--------------------------------"

if $SUDO nginx -t 2>&1 | grep -q "successful"; then
    echo -e "${GREEN}✅ Конфигурация nginx: OK${NC}"
else
    echo -e "${RED}❌ Конфигурация nginx: Ошибка${NC}"
    $SUDO nginx -t
fi

echo ""
echo "5️⃣ Проверка конфигурации Yuki"
echo "--------------------------------"

CONFIG_FILE="/opt/yuki/server/config.json"
if [ -f "$CONFIG_FILE" ]; then
    echo -e "${GREEN}✅ Файл конфигурации найден${NC}"
    echo "Путь: $CONFIG_FILE"
    
    # Check if valid JSON
    if python3 -m json.tool "$CONFIG_FILE" >/dev/null 2>&1; then
        echo -e "${GREEN}✅ JSON синтаксис: OK${NC}"
    else
        echo -e "${RED}❌ JSON синтаксис: Ошибка${NC}"
    fi
    
    # Extract key settings
    echo ""
    echo "Основные настройки:"
    grep -E '"(address|port|admin_port|domain)"' "$CONFIG_FILE" | head -10
else
    echo -e "${RED}❌ Файл конфигурации не найден${NC}"
fi

echo ""
echo "6️⃣ Последние логи Yuki (20 строк)"
echo "--------------------------------"
$SUDO journalctl -u yuki -n 20 --no-pager

echo ""
echo "7️⃣ Последние логи nginx (10 строк)"
echo "--------------------------------"
if [ -f "/var/log/nginx/error.log" ]; then
    $SUDO tail -10 /var/log/nginx/error.log
else
    echo "Лог-файл не найден"
fi

echo ""
echo "8️⃣ Проверка firewall (ufw)"
echo "--------------------------------"
if command -v ufw >/dev/null 2>&1; then
    $SUDO ufw status | head -20
else
    echo "ufw не установлен"
fi

echo ""
echo "9️⃣ Тест подключения к серверу"
echo "--------------------------------"

if [ -n "$DOMAIN" ]; then
    echo "Проверка HTTP -> HTTPS редиректа:"
    curl -sI "http://$DOMAIN" 2>/dev/null | grep -E "(HTTP|Location)" || echo "Не удалось подключиться"
    
    echo ""
    echo "Проверка HTTPS /health endpoint:"
    curl -sk "https://$DOMAIN/health" 2>/dev/null | head -5 || echo "Не удалось подключиться"
    
    echo ""
    echo "Проверка HTTPS /admin endpoint:"
    HTTP_CODE=$(curl -sk -o /dev/null -w "%{http_code}" "https://$DOMAIN/admin" 2>/dev/null)
    if [ "$HTTP_CODE" = "200" ]; then
        echo -e "${GREEN}✅ Админ панель доступна (HTTP $HTTP_CODE)${NC}"
    else
        echo -e "${YELLOW}⚠️ Админ панель вернула код: $HTTP_CODE${NC}"
    fi
fi

echo ""
echo "🔟 Использование ресурсов"
echo "--------------------------------"

# Memory usage
echo "Память:"
free -h | grep -E "(Mem|Swap)"

echo ""
echo "Диск:"
df -h / | tail -1

echo ""
echo "CPU загрузка (1, 5, 15 мин):"
uptime | awk -F'load average:' '{print $2}'

echo ""
echo "================================"
echo "Диагностика завершена!"
echo ""

# Recommendations
echo "💡 Рекомендации:"
if ! $SUDO systemctl is-active --quiet nginx; then
    echo "  - Запустите nginx: sudo systemctl restart nginx"
fi
if ! $SUDO systemctl is-active --quiet yuki; then
    echo "  - Запустите yuki: sudo systemctl restart yuki"
fi
if ! $SUDO netstat -tlnp 2>/dev/null | grep -q ':443 '; then
    echo "  - Проверьте SSL сертификаты и перезапустите nginx"
    echo "    sudo systemctl restart nginx"
fi
if [ -n "$DOMAIN" ]; then
    echo "  - Проверьте доступность: curl https://$DOMAIN/health"
fi

echo ""
