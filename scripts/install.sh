#!/bin/bash
set -e

# Yuki VPN Server - Ubuntu 24 Quick Install
echo "🌸 Installing Yuki VPN Server..."

# Determine sudo usage and owner
if [[ $EUID -eq 0 ]]; then
   SUDO=""
   OWNER="${SUDO_USER:-root}"
else
   SUDO="sudo"
   OWNER="${USER}"
fi

# Interactive prompt helper that reads from /dev/tty if stdin is not TTY
ask() {
    local var_name="$1"
    local prompt_text="$2"
    local current_value
    # Read current env value if set
    current_value="${!var_name}"
    if [ -z "$current_value" ]; then
        if [ -t 0 ]; then
            read -r -p "$prompt_text" current_value
        else
            # stdin is not a TTY (e.g., curl | bash). Read from the controlling terminal.
            read -r -p "$prompt_text" current_value < /dev/tty
        fi
    fi
    if [ -z "$current_value" ]; then
        echo "❌ ${var_name} is required"
        exit 1
    fi
    printf -v "$var_name" '%s' "$current_value"
    export "$var_name"
}

# Ask required inputs
ask DOMAIN "🌐 Enter your domain (e.g., api.example.ru): "
ask EMAIL "📧 Enter your email for SSL certificate: "
ask ADMIN_LOGIN "👤 Enter admin login for web panel: "
ask ADMIN_PASSWORD "🔐 Enter admin password for web panel: "

# Normalize domain to ASCII (Punycode) if needed
DOMAIN_ASCII="$DOMAIN"
if ! [[ "$DOMAIN" =~ ^[A-Za-z0-9.-]+$ ]]; then
    echo "🔤 Converting Internationalized domain to ASCII (Punycode)..."
    if command -v idn2 >/dev/null 2>&1; then
        DOMAIN_ASCII=$(echo "$DOMAIN" | idn2) || DOMAIN_ASCII="$DOMAIN"
    fi
fi

# Update system
echo "📦 Updating system packages..."
$SUDO apt update && $SUDO apt upgrade -y

# Install Go 1.23+
echo "🔧 Installing Go..."
if ! command -v go &> /dev/null; then
    cd /tmp
    wget https://go.dev/dl/go1.23.4.linux-amd64.tar.gz
    $SUDO rm -rf /usr/local/go
    $SUDO tar -C /usr/local -xzf go1.23.4.linux-amd64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    export PATH=$PATH:/usr/local/go/bin
fi

# Install dependencies
echo "📋 Installing dependencies..."
# ensure cron is available (provides crontab) and include openssl for key generation
$SUDO apt install -y git curl wget nginx certbot python3-certbot-nginx redis-server ufw cron openssl

# Clone or update repository
echo "📥 Downloading Yuki..."
cd /opt
if [ -d "yuki/.git" ]; then
    echo "🔄 Updating existing Yuki repository..."
    $SUDO git -C yuki fetch --all --prune
    $SUDO git -C yuki reset --hard origin/main
    $SUDO git -C yuki clean -fdx
    $SUDO chown -R "$OWNER":"$OWNER" yuki
    cd yuki
elif [ -d "yuki" ]; then
    echo "⚠️ Found /opt/yuki directory without git metadata. Backing up and cloning fresh..."
    TS=$(date +%s)
    $SUDO mv yuki "yuki.backup.$TS"
    $SUDO git clone https://github.com/Mr-X-01/yuki.git
    $SUDO chown -R "$OWNER":"$OWNER" yuki
    cd yuki
else
    $SUDO git clone https://github.com/Mr-X-01/yuki.git
    $SUDO chown -R "$OWNER":"$OWNER" yuki
    cd yuki
fi

# Setup firewall
echo "🔥 Configuring firewall..."
$SUDO ufw --force reset
$SUDO ufw default deny incoming
$SUDO ufw default allow outgoing
$SUDO ufw allow ssh
$SUDO ufw allow 80/tcp
$SUDO ufw allow 443/tcp
$SUDO ufw --force enable

# Enable IP forwarding для VPN
echo "🌐 Configuring IP forwarding and NAT..."
echo "net.ipv4.ip_forward=1" | $SUDO tee -a /etc/sysctl.conf
$SUDO sysctl -p

# Определяем основной сетевой интерфейс
MAIN_INTERFACE=$($SUDO ip route | grep default | awk '{print $5}' | head -n1)
if [ -z "$MAIN_INTERFACE" ]; then
    MAIN_INTERFACE="eth0"
fi
echo "📡 Основной сетевой интерфейс: $MAIN_INTERFACE"

# Настройка NAT для VPN клиентов
$SUDO iptables -t nat -A POSTROUTING -s 10.0.0.0/24 -o $MAIN_INTERFACE -j MASQUERADE
$SUDO iptables -A FORWARD -s 10.0.0.0/24 -j ACCEPT
$SUDO iptables -A FORWARD -d 10.0.0.0/24 -m state --state RELATED,ESTABLISHED -j ACCEPT

# Сохраняем правила iptables
$SUDO apt install -y iptables-persistent
echo iptables-persistent iptables-persistent/autosave_v4 boolean true | $SUDO debconf-set-selections
echo iptables-persistent iptables-persistent/autosave_v6 boolean true | $SUDO debconf-set-selections
$SUDO netfilter-persistent save

# Открываем порты в firewall
echo "🔓 Открываем порты в firewall..."
$SUDO ufw allow 22/tcp
$SUDO ufw allow 80/tcp
$SUDO ufw allow 443/tcp
$SUDO ufw allow 8443/tcp  # gRPC tunnel service
$SUDO ufw allow 8444/tcp  # Admin API (optional, только для внутреннего использования)
$SUDO ufw reload

echo "✅ IP forwarding, NAT и firewall настроены"

# Build server
echo "🏗️ Building server..."
cd server

# Clean old files to prevent conflicts
echo "🧹 Cleaning old files..."
rm -f tunnel/tun_linux.go 2>/dev/null || true

# Stop existing server if running
sudo systemctl stop yuki 2>/dev/null || true
sudo pkill -f yuki-server 2>/dev/null || true

# Backup config.json if it exists (to preserve user data during rebuild)
if [ -f "config.json" ]; then
    echo "📦 Backing up existing config.json..."
    cp config.json config.json.backup
fi

# Fix protobuf version compatibility
echo "🔄 Fixing protobuf versions..."
go get google.golang.org/protobuf@v1.28.1
go get google.golang.org/grpc@v1.50.1

# Install protoc generators
echo "📦 Installing protoc generators..."
export PATH=$PATH:$(go env GOPATH)/bin
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.1
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2.0

# Regenerate proto files
echo "🔄 Regenerating proto files..."
cd proto
rm -f *.pb.go
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       tunnel.proto
if [ ! -f "tunnel.pb.go" ]; then
    echo "❌ Failed to generate proto files"
    exit 1
fi
echo "✅ Proto files generated"
cd ..

go mod tidy
go build -ldflags "-s -w" -o yuki-server .

if [ ! -f "yuki-server" ] || [ ! -x "yuki-server" ]; then
    echo "❌ Build failed - check logs above"
    exit 1
fi

echo "✅ Server binary built successfully"

# Configure nginx for domain verification
echo "🌐 Configuring nginx for SSL verification..."
# Remove possible conflicting enabled sites
$SUDO rm -f /etc/nginx/sites-enabled/default /etc/nginx/sites-enabled/yuki || true
$SUDO tee /etc/nginx/sites-available/yuki > /dev/null <<EOF
server {
    listen 80;
    server_name $DOMAIN $DOMAIN_ASCII;
    
    location /.well-known/acme-challenge/ {
        root /var/www/html;
    }
    
    location / {
        return 301 https://\$server_name\$request_uri;
    }
}
EOF

# Удаляем старые симлинки и создаем новый
$SUDO rm -f /etc/nginx/sites-enabled/default /etc/nginx/sites-enabled/yuki
$SUDO ln -sf /etc/nginx/sites-available/yuki /etc/nginx/sites-enabled/yuki

# Проверяем что симлинк создан
if [ ! -L "/etc/nginx/sites-enabled/yuki" ]; then
    echo "❌ Не удалось создать симлинк для конфигурации nginx"
    exit 1
fi

echo "✅ Конфигурация nginx активирована"
$SUDO nginx -t && $SUDO systemctl reload nginx

# Get SSL certificate
echo "🔒 Obtaining SSL certificate with certbot..."
$SUDO certbot certonly --webroot -w /var/www/html -d $DOMAIN_ASCII --email $EMAIL --agree-tos --non-interactive

# Check if certificate was obtained successfully
if [ ! -f "/etc/letsencrypt/live/$DOMAIN_ASCII/fullchain.pem" ]; then
    echo "❌ Failed to obtain SSL certificate. Please check domain DNS and try again."
    exit 1
fi

# Generate initial config with correct domain and SSL paths (use helper to avoid protobuf init panic)
echo "📝 Generating configuration..."
cd /opt/yuki/server

# If we have a backup, use it and preserve admin_login/admin_password
if [ -f "config.json.backup" ]; then
    echo "💾 Restoring backed up configuration..."
    # Update login/password in backup with current values
    cp config.json.backup config.json
    # Update the admin credentials in config
    ADMIN_LOGIN_ESCAPED=$(echo "$ADMIN_LOGIN" | sed 's/[\/&]/\\&/g')
    ADMIN_PASSWORD_ESCAPED=$(echo "$ADMIN_PASSWORD" | sed 's/[\/&]/\\&/g')
    sed -i "s/\"admin_login\": \"[^\"]*\"/\"admin_login\": \"$ADMIN_LOGIN_ESCAPED\"/g" config.json
    sed -i "s/\"admin_password\": \"[^\"]*\"/\"admin_password\": \"$ADMIN_PASSWORD_ESCAPED\"/g" config.json
else
    # Generate new config with correct settings from the start
    cat > config.json <<JSON
{
  "server": {
    "address": "0.0.0.0",
    "port": 8443,
    "admin_port": 8444,
    "cert_file": "/etc/letsencrypt/live/$DOMAIN_ASCII/fullchain.pem",
    "key_file": "/etc/letsencrypt/live/$DOMAIN_ASCII/privkey.pem",
    "domain": "$DOMAIN_ASCII"
  },
  "redis": {
    "address": "localhost:6379",
    "password": "",
    "db": 0
  },
  "auth": {
    "admin_api_key": "$(openssl rand -hex 32)",
    "jwt_secret": "$(openssl rand -hex 32)",
    "admin_login": "$ADMIN_LOGIN",
    "admin_password": "$ADMIN_PASSWORD"
  },
  "tunnel": {
    "keep_alive": 15,
    "compression": false,
    "buffer_size": 32768
  },
  "limits": {
    "max_clients": 1000,
    "rate_limit": 100,
    "max_bandwidth": 1073741824
  }
}
JSON

    echo "✅ Config generated with correct settings"
fi

# Configure nginx with SSL
echo "🌐 Configuring nginx with SSL..."
$SUDO rm -f /etc/nginx/sites-enabled/default /etc/nginx/sites-enabled/yuki || true
$SUDO tee /etc/nginx/sites-available/yuki > /dev/null <<EOF
server {
    listen 80;
    server_name $DOMAIN $DOMAIN_ASCII;
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name $DOMAIN $DOMAIN_ASCII;
    
    ssl_certificate /etc/letsencrypt/live/$DOMAIN_ASCII/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$DOMAIN_ASCII/privkey.pem;
    
    # SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-RSA-AES128-GCM-SHA256:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-RSA-AES128-SHA256:ECDHE-RSA-AES256-SHA384;
    ssl_prefer_server_ciphers on;
    ssl_session_cache shared:SSL:10m;
    
    # gRPC tunnel service (direct proxy to Yuki server)
    location /tunnel.TunnelService/ {
        grpc_pass grpc://127.0.0.1:8443;
        grpc_set_header Host \$host;
        grpc_set_header X-Real-IP \$remote_addr;
    }
    
    # Admin API
    location /admin/ {
        proxy_pass http://127.0.0.1:8444;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
    
    # Legitimate API endpoints for cover
    location / {
        proxy_pass http://127.0.0.1:8444;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF

# Удаляем старые симлинки и создаем новый для SSL конфигурации
$SUDO rm -f /etc/nginx/sites-enabled/default /etc/nginx/sites-enabled/yuki
$SUDO ln -sf /etc/nginx/sites-available/yuki /etc/nginx/sites-enabled/yuki

# Проверяем что симлинк создан
if [ ! -L "/etc/nginx/sites-enabled/yuki" ]; then
    echo "❌ Не удалось создать симлинк для конфигурации nginx"
    exit 1
fi

echo "✅ SSL конфигурация nginx активирована"
ls -la /etc/nginx/sites-enabled/

$SUDO nginx -t && $SUDO systemctl reload nginx

# Проверка nginx после конфигурации
echo "🔍 Проверка nginx..."
if ! $SUDO systemctl is-active --quiet nginx; then
    echo "⚠️ nginx не запущен, запускаем..."
    $SUDO systemctl restart nginx
fi

# Проверяем что nginx слушает порты 80 и 443
if ! $SUDO netstat -tlnp | grep -q ':80 '; then
    echo "❌ nginx не слушает порт 80"
    echo "Логи nginx:"
    $SUDO tail -20 /var/log/nginx/error.log
fi

if ! $SUDO netstat -tlnp | grep -q ':443 '; then
    echo "❌ nginx не слушает порт 443"
    echo "Проверка SSL сертификата:"
    ls -la /etc/letsencrypt/live/$DOMAIN_ASCII/ || echo "Сертификат не найден"
    echo "Логи nginx:"
    $SUDO tail -20 /var/log/nginx/error.log
    echo "Пробуем перезапустить nginx..."
    $SUDO systemctl restart nginx
    sleep 2
    if ! $SUDO netstat -tlnp | grep -q ':443 '; then
        echo "❌ nginx все еще не слушает порт 443, проверьте конфигурацию вручную"
    else
        echo "✅ nginx теперь слушает порт 443"
    fi
else
    echo "✅ nginx слушает порты 80 и 443"
fi

# Create systemd service
echo "⚙️ Creating systemd service..."
$SUDO tee /etc/systemd/system/yuki.service > /dev/null <<EOF
[Unit]
Description=Yuki VPN Server
After=network.target redis.service

[Service]
Type=simple
User=$OWNER
WorkingDirectory=/opt/yuki/server
ExecStart=/opt/yuki/server/yuki-server
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=yuki

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/yuki/server

[Install]
WantedBy=multi-user.target
EOF

# Enable and start services
$SUDO systemctl daemon-reload
$SUDO systemctl enable redis-server yuki
$SUDO systemctl start redis-server

# Setup automatic certificate renewal
echo "🔄 Setting up automatic certificate renewal..."
# Prefer using crontab; if unavailable install cron or fallback to /etc/cron.d
CRON_CMD="/usr/bin/certbot renew --quiet && systemctl reload nginx"
CRON_ENTRY="0 12 * * * $CRON_CMD"

if ! command -v crontab >/dev/null 2>&1; then
    echo "⚠️ 'crontab' not found — attempting to install 'cron' package..."
    $SUDO apt update -y || true
    $SUDO apt install -y cron || true
    $SUDO systemctl enable --now cron || true
fi

if command -v crontab >/dev/null 2>&1; then
    # Add the job to root's crontab if not already present
    ( $SUDO crontab -l 2>/dev/null | grep -Fv "$CRON_CMD" || true; echo "$CRON_ENTRY" ) | $SUDO crontab -
else
    # Fallback: create a cron.d file (requires a user field). Run as root.
    $SUDO tee /etc/cron.d/yuki-cert-renew > /dev/null <<EOF
# Cron job to renew Let's Encrypt certs for Yuki
0 12 * * * root $CRON_CMD
EOF
    $SUDO chmod 644 /etc/cron.d/yuki-cert-renew
fi

# Start Yuki server
echo "🚀 Starting Yuki VPN server..."
$SUDO systemctl restart yuki

# Wait a moment for startup
echo "⏳ Ожидание запуска сервера..."
sleep 5

# Проверка что сервер запустился
if ! $SUDO systemctl is-active --quiet yuki; then
    echo "❌ Yuki сервер не запустился, проверяем логи:"
    $SUDO journalctl -u yuki -n 30 --no-pager
fi

# Extract API key from config for display
API_KEY=$(grep -oP '"admin_api_key":\s*"\K[^"]+' config.json || echo "SEE CONFIG FILE")

# Check service status
if $SUDO systemctl is-active --quiet yuki; then
    echo "✅ Yuki VPN Server installed and started successfully!"
else
    echo "⚠️ Yuki VPN Server installed but failed to start. Check logs:"
    echo "$SUDO journalctl -u yuki -n 20"
fi

echo ""
echo "📋 Installation complete!"
echo "🌐 Server: https://$DOMAIN"
echo ""
echo "🔐 Admin Panel Access:"
echo "   URL: https://$DOMAIN/admin"
echo "   Login: $ADMIN_LOGIN"
echo "   Password: $ADMIN_PASSWORD"
echo ""
echo "💡 Save these credentials securely - you'll need them to manage the VPN"
echo ""
echo "📝 Next steps:"
echo "1. Open https://$DOMAIN/admin in your browser"
echo "2. Login with credentials above"
echo "3. Create VPN clients and download their configurations"
echo "4. Test health endpoint: curl https://$DOMAIN/health"
echo "5. Check logs: sudo journalctl -u yuki -f"
echo ""
echo "🔒 SSL certificate will auto-renew via cron job"
echo ""
echo "🔍 Финальная диагностика:"
echo "Статус nginx:"
$SUDO systemctl status nginx --no-pager -l | head -10
echo ""
echo "Открытые порты:"
$SUDO netstat -tlnp | grep -E ':(80|443|8443|50051) '
echo ""
echo "Статус Yuki:"
$SUDO systemctl status yuki --no-pager -l | head -10
echo ""
echo "✅ Установка завершена! Проверьте https://$DOMAIN/health"
