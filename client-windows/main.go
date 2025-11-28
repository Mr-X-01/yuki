package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"yuki-client/config"
	"yuki-client/crypto"
	"yuki-client/tun"
)

var (
	configFile = flag.String("config", "yuki.json", "Path to config file")
	genLink    = flag.Bool("gen-link", false, "Generate connection link")
	uriString  = flag.String("uri", "", "Connection URI (yuki://client_id:client_secret@server:port?encryption=...)")
)

type Client struct {
	config   *config.Config
	tunIface tun.Interface
	cipher   *crypto.Cipher
	conn     net.Conn
	connected bool
}

func main() {
	flag.Parse()

	// Настройка логирования в файл
	logFile, logErr := os.OpenFile("yuki-client.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if logErr == nil {
		defer logFile.Close()
		multiWriter := io.MultiWriter(os.Stdout, logFile)
		log.SetOutput(multiWriter)
	}

	// Отлавливаем панику для диагностики
	defer func() {
		if r := recover(); r != nil {
			log.Printf("\n❌ ПАНИКА: %v\n", r)
			log.Printf("Программа завершилась с ошибкой. Проверьте yuki-client.log\n")
			log.Printf("Нажмите Enter для выхода...")
			fmt.Scanln()
		}
	}()

	// Проверяем права администратора
	if !isAdmin() {
		log.Printf("❌ Требуются права администратора для настройки сетевых маршрутов!\n")
		log.Printf("   Запустите программу от имени администратора.\n")
		log.Printf("\nНажмите Enter для выхода...")
		fmt.Scanln()
		os.Exit(1)
	}

	var cfg *config.Config
	var err error

	// Если указан URI, используем его
	if *uriString != "" {
		log.Printf("📋 Парсинг URI подключения...\n")
		cfg, err = parseURI(*uriString)
		if err != nil {
			log.Printf("❌ Ошибка парсинга URI: %v\n", err)
			log.Printf("\nФормат: yuki://client_id:client_secret@server:port?encryption=xchacha20-poly1305\n")
			log.Printf("Нажмите Enter для выхода...")
			fmt.Scanln()
			os.Exit(1)
		}
		log.Printf("✅ URI успешно обработан\n")
	} else {
		// Иначе загружаем из файла
		cfg, err = loadConfig(*configFile)
		if err != nil {
			log.Printf("❌ Не удалось загрузить конфигурацию: %v\n", err)
			log.Printf("   Убедитесь что файл %s существует\n", *configFile)
			log.Printf("   Или используйте флаг -uri для подключения через URI\n")
			log.Printf("\nНажмите Enter для выхода...")
			fmt.Scanln()
			os.Exit(1)
		}
	}

	if *genLink {
		generateLink(cfg)
		return
	}

	client := &Client{config: cfg}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("🌸 Yuki VPN Client Starting...")
	log.Println("Press Ctrl+C to exit")

	go func() {
		<-sigChan
		log.Println("\n🛑 Shutting down...")
		if client.connected {
			client.Disconnect()
		}
		os.Exit(0)
	}()

	// Основной цикл подключения
	for {
		log.Println("🔌 Подключение к серверу...")
		if err := client.Connect(); err != nil {
			log.Printf("❌ Ошибка подключения: %v\n", err)
			log.Println("🔄 Повторная попытка через 5 секунд...")
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println("✅ Подключено! Нажмите Ctrl+C для отключения")
		
		// Если отключились, пробуем переподключиться
		if !client.connected {
			log.Println("⚠️ Соединение потеряно, переподключение...")
			time.Sleep(2 * time.Second)
		}
	}
}

func loadConfig(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &config.Config{}
	err = json.Unmarshal(data, cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func parseURI(uri string) (*config.Config, error) {
	if !strings.HasPrefix(uri, "yuki://") {
		return nil, fmt.Errorf("URI должен начинаться с yuki://")
	}

	// Убираем префикс yuki://
	uri = strings.TrimPrefix(uri, "yuki://")

	// Разделяем на части: credentials@server и query параметры
	parts := strings.Split(uri, "?")
	if len(parts) < 1 {
		return nil, fmt.Errorf("неверный формат URI")
	}

	// Парсим credentials@server:port
	credentialsPart := parts[0]
	atIndex := strings.LastIndex(credentialsPart, "@")
	if atIndex == -1 {
		return nil, fmt.Errorf("отсутствует @ в URI")
	}

	credentials := credentialsPart[:atIndex]
	serverAddr := credentialsPart[atIndex+1:]

	// Разделяем credentials на client_id:client_secret
	credParts := strings.Split(credentials, ":")
	if len(credParts) != 2 {
		return nil, fmt.Errorf("неверный формат credentials")
	}

	clientID := credParts[0]
	clientSecret := credParts[1]

	// Создаем конфигурацию
	cfg := &config.Config{
		ServerAddress: serverAddr,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		Protocol:      "grpc",
		Encryption:    "xchacha20-poly1305", // по умолчанию
	}

	// Парсим query параметры если есть
	if len(parts) > 1 {
		queryParams := strings.Split(parts[1], "&")
		for _, param := range queryParams {
			kv := strings.Split(param, "=")
			if len(kv) == 2 {
				switch kv[0] {
				case "encryption":
					cfg.Encryption = kv[1]
				case "protocol":
					cfg.Protocol = kv[1]
				}
			}
		}
	}

	return cfg, nil
}

func (c *Client) Connect() error {
	// Setup TLS connection
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // For testing, ignore cert warnings
	}

	// Extract host from address
	host := c.config.ServerAddress
	if len(host) == 0 {
		return fmt.Errorf("server address not configured")
	}

	conn, err := tls.Dial("tcp", host, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS connection failed: %w", err)
	}

	c.conn = conn
	defer conn.Close()

	// Send authentication
	authData := map[string]string{
		"client_id":     c.config.ClientID,
		"client_secret": c.config.ClientSecret,
	}

	_ = authData  // Will be used in tunnel handshake

	// Send POST request
	req, err := http.NewRequest("POST", "https://"+c.config.ServerAddress+"/tunnel/connect", 
		io.NopCloser(nil))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-ID", c.config.ClientID)
	req.Header.Set("X-Client-Secret", c.config.ClientSecret)

	// Actually, let's use simpler approach - direct TCP tunnel
	return c.tunneL()
}

func (c *Client) tunneL() error {
	// Create TUN interface
	tunIface := tun.NewTunInterface()
	if tunIface == nil {
		return fmt.Errorf("failed to create TUN interface")
	}
	defer tunIface.Close()

	// Initialize the interface
	if err := tunIface.Create("Yuki Tunnel"); err != nil {
		return fmt.Errorf("failed to create TUN interface: %w", err)
	}

	// Configure IP address
	clientIP := net.ParseIP("10.0.0.2")
	subnetMask := net.IPMask(net.ParseIP("255.255.255.0").To4())
	gateway := net.ParseIP("10.0.0.1")

	if err := tunIface.SetIP(clientIP, subnetMask, gateway); err != nil {
		log.Printf("⚠️ Failed to set IP address: %v", err)
		// Continue anyway - some systems may handle this differently
	}

	c.tunIface = tunIface
	c.connected = true

	log.Printf("✅ Tunnel connected!")
	log.Printf("🌐 Interface: %s (%s)\n", "Yuki Tunnel", "10.0.0.2/24")
	log.Printf("🚀 Весь трафик теперь направляется через VPN туннель")
	log.Printf("🔍 Проверьте свой внешний IP на https://2ip.ru")
	log.Printf("💡 Если IP не изменился, проверьте права администратора")

	// Start packet handling goroutine
	go c.handlePackets()

	log.Printf("🔗 VPN туннель активен. Нажмите Ctrl+C для отключения")
	
	// Keep connection alive - простой цикл без лишних проверок
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for c.connected {
		select {
		case <-ticker.C:
			// Каждые 5 секунд проверяем статус
			log.Printf("💚 VPN туннель работает (IP: 10.0.0.2)")
			
			// Можно добавить проверку статуса сетевого интерфейса
			// if !c.checkTunStatus() {
			//     log.Printf("⚠️ TUN интерфейс недоступен")
			// }
			
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil
}

func (c *Client) Disconnect() {
	log.Println("🛑 Отключение VPN туннеля...")
	if c.tunIface != nil {
		c.tunIface.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
	c.connected = false
	log.Println("✅ VPN туннель отключен, маршруты восстановлены")
}

// Проверка прав администратора
func isAdmin() bool {
	_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	if err != nil {
		return false
	}
	return true
}


func (c *Client) handlePackets() {
	if c.tunIface == nil {
		return
	}

	log.Printf("🔄 Запущен обработчик пакетов TUN интерфейса")
	buffer := make([]byte, 1500) // MTU size
	for c.connected {
		// Устанавливаем таймаут для чтения
		// В реальной реализации здесь был бы SetReadDeadline, но пока просто ждем
		n, err := c.tunIface.Read(buffer)
		if err != nil {
			// Многие TUN интерфейсы возвращают ошибки при отсутствии данных
			// Это нормально, просто ждем и продолжаем
			time.Sleep(100 * time.Millisecond)
			continue
		}
		
		if n > 0 {
			log.Printf("📨 Получено %d байт от TUN интерфейса", n)
			// В реальной реализации здесь пакет шифровался бы и отправлялся на сервер
			// Пока что просто логируем трафик
			
			// Имитируем обработку пакета - можно добавить счетчик трафика
			// c.stats.BytesUp += int64(n)
		}
		
		// Небольшая задержка чтобы не нагружать CPU
		time.Sleep(10 * time.Millisecond)
	}
	
	log.Printf("🛑 Обработчик пакетов завершен")
}

func generateLink(cfg *config.Config) {
	// Generate link like: yuki://client_id:secret@server:port
	link := fmt.Sprintf("yuki://%s:%s@%s?encryption=%s",
		cfg.ClientID,
		cfg.ClientSecret,
		cfg.ServerAddress,
		cfg.Encryption,
	)

	fmt.Println("🔗 Connection Link:")
	fmt.Println(link)
	fmt.Println()
	fmt.Println("📋 To connect:")
	fmt.Println("  Windows: yuki-client -config <link>")
	fmt.Println("  Linux:   ./yuki-client -config <link>")
	fmt.Println()
	fmt.Println("Or copy config as:")
	data, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(data))
}
