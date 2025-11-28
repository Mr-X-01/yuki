package tun

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// WinTun constants
	WINTUN_RING_CAPACITY = 0x800000 // 8MB
	WINTUN_MAX_PACKET_SIZE = 0xFFFF
)

type WinTun struct {
	adapter   windows.Handle
	session   windows.Handle
	readWait  windows.Handle
	writeWait windows.Handle
	running   bool
}

// Windows API и WinTun DLL функции
var (
	wintunDLL                *windows.LazyDLL
	wintunCreateAdapter      *windows.LazyProc
	wintunOpenAdapter        *windows.LazyProc
	wintunCloseAdapter       *windows.LazyProc
	wintunStartSession       *windows.LazyProc
	wintunEndSession         *windows.LazyProc
	wintunGetReadWaitEvent   *windows.LazyProc
	wintunReceivePacket      *windows.LazyProc
	wintunReleaseReceivePacket *windows.LazyProc
	wintunAllocateSendPacket *windows.LazyProc
	wintunSendPacket         *windows.LazyProc
)

// GUID для WinTun
var WINTUN_GUID = windows.GUID{
	Data1: 0xdeadbabe,
	Data2: 0xcafe,
	Data3: 0xbeef,
	Data4: [8]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef},
}

func init() {
	// Попытка загрузки wintun.dll из текущей директории
	wintunDLL = windows.NewLazyDLL("wintun.dll")
	if wintunDLL.Load() == nil {
		wintunCreateAdapter = wintunDLL.NewProc("WintunCreateAdapter")
		wintunOpenAdapter = wintunDLL.NewProc("WintunOpenAdapter")
		wintunCloseAdapter = wintunDLL.NewProc("WintunCloseAdapter")
		wintunStartSession = wintunDLL.NewProc("WintunStartSession")
		wintunEndSession = wintunDLL.NewProc("WintunEndSession")
		wintunGetReadWaitEvent = wintunDLL.NewProc("WintunGetReadWaitEvent")
		wintunReceivePacket = wintunDLL.NewProc("WintunReceivePacket")
		wintunReleaseReceivePacket = wintunDLL.NewProc("WintunReleaseReceivePacket")
		wintunAllocateSendPacket = wintunDLL.NewProc("WintunAllocateSendPacket")
		wintunSendPacket = wintunDLL.NewProc("WintunSendPacket")
	}
}

func createWinTunAdapter(name string) (windows.Handle, error) {
	if wintunDLL == nil || wintunCreateAdapter == nil {
		return windows.InvalidHandle, fmt.Errorf("WinTun DLL not loaded")
	}
	
	namePtr, _ := windows.UTF16PtrFromString(name)
	tunnelType, _ := windows.UTF16PtrFromString("Yuki")
	
	ret, _, err := wintunCreateAdapter.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(tunnelType)),
		uintptr(unsafe.Pointer(&WINTUN_GUID)),
	)
	
	if ret == 0 {
		return windows.InvalidHandle, fmt.Errorf("WintunCreateAdapter failed: %v", err)
	}
	
	return windows.Handle(ret), nil
}

func (w *WinTun) Create(name string) error {
	adapter, err := createWinTunAdapter(name)
	if err != nil {
		return fmt.Errorf("failed to create WinTun adapter: %w", err)
	}
	
	w.adapter = adapter
	
	// Запуск сессии WinTun
	if wintunStartSession != nil {
		ret, _, _ := wintunStartSession.Call(
			uintptr(w.adapter),
			uintptr(WINTUN_RING_CAPACITY),
		)
		
		if ret == 0 {
			windows.CloseHandle(w.adapter)
			return fmt.Errorf("failed to start WinTun session")
		}
		
		w.session = windows.Handle(ret)
		
		// Получаем события для чтения
		if wintunGetReadWaitEvent != nil {
			eventRet, _, _ := wintunGetReadWaitEvent.Call(uintptr(w.session))
			w.readWait = windows.Handle(eventRet)
		}
	}
	
	w.running = true
	return nil
}

func (w *WinTun) Read(buf []byte) (int, error) {
	if !w.running {
		return 0, fmt.Errorf("tun interface not running")
	}
	
	if w.session == windows.InvalidHandle {
		return 0, fmt.Errorf("session not started")
	}
	
	// Ждем данных с таймаутом
	if w.readWait != windows.InvalidHandle {
		waitResult, err := windows.WaitForSingleObject(w.readWait, 1000) // 1 сек таймаут
		if err != nil {
			return 0, err
		}
		if waitResult == 0x00000102 { // WAIT_TIMEOUT
			return 0, fmt.Errorf("read timeout")
		}
	}
	
	// Читаем пакет из WinTun
	if wintunReceivePacket != nil {
		var packetSize uint32
		ret, _, _ := wintunReceivePacket.Call(
			uintptr(w.session),
			uintptr(unsafe.Pointer(&packetSize)),
		)
		
		if ret == 0 {
			return 0, fmt.Errorf("no packet available")
		}
		
		// Копируем данные пакета
		packetPtr := unsafe.Pointer(ret)
		if packetSize > uint32(len(buf)) {
			packetSize = uint32(len(buf))
		}
		// Используем unsafe.Slice для безопасного среза и copy
		//nolint:gosec // Pointer is provided by WinTun API and valid for the duration before release
		src := unsafe.Slice((*byte)(packetPtr), packetSize)
		copy(buf[:packetSize], src)
		
		// Освобождаем пакет
		if wintunReleaseReceivePacket != nil {
			wintunReleaseReceivePacket.Call(uintptr(w.session), ret)
		}
		
		return int(packetSize), nil
	}
	
	return 0, fmt.Errorf("WinTun receive function not available")
}

func (w *WinTun) Write(buf []byte) (int, error) {
	if !w.running {
		return 0, fmt.Errorf("tun interface not running")
	}
	
	if w.session == windows.InvalidHandle {
		return 0, fmt.Errorf("session not started")
	}
	
	if len(buf) == 0 {
		return 0, nil
	}
	
	// Аллокация пакета в WinTun
	if wintunAllocateSendPacket != nil && wintunSendPacket != nil {
		packetSize := uint32(len(buf))
		ret, _, _ := wintunAllocateSendPacket.Call(
			uintptr(w.session),
			uintptr(packetSize),
		)
		
		if ret == 0 {
			return 0, fmt.Errorf("failed to allocate send packet")
		}
		
		// Копируем данные в пакет WinTun с помощью unsafe.Slice
		packetPtr := unsafe.Pointer(ret)
		//nolint:gosec // Pointer is provided by WinTun API and valid until send completes
		dst := unsafe.Slice((*byte)(packetPtr), packetSize)
		copy(dst, buf[:packetSize])
		
		// Отправляем пакет
		wintunSendPacket.Call(uintptr(w.session), ret)
		
		return len(buf), nil
	}
	
	return 0, fmt.Errorf("WinTun send functions not available")
}

func (w *WinTun) Close() error {
	// Удаляем маршруты перед закрытием интерфейса
	w.cleanupRoutes()
	
	if w.adapter != windows.InvalidHandle {
		windows.CloseHandle(w.adapter)
	}
	if w.session != windows.InvalidHandle {
		windows.CloseHandle(w.session)
	}
	w.running = false
	return nil
}

func (w *WinTun) cleanupRoutes() {
	// Удаляем маршруты, которые мы добавили
	cmds := []string{
		"route delete 0.0.0.0 mask 0.0.0.0 10.0.0.1",
		"route delete 0.0.0.0 mask 128.0.0.0 10.0.0.1", 
		"route delete 128.0.0.0 mask 128.0.0.0 10.0.0.1",
	}
	
	for _, cmd := range cmds {
		w.runNetshCommand(cmd) // Игнорируем ошибки при очистке
	}
	
	fmt.Printf("🧹 Очищены VPN маршруты\n")
}

func (w *WinTun) SetIP(ip net.IP, mask net.IPMask, gateway net.IP) error {
	// Используем netsh для настройки IP адреса и маршрутов
	ipStr := ip.String()
	maskStr := net.IP(mask).String()
	gatewayStr := gateway.String()
	
	// 1. Установка статического IP на интерфейс
	cmd1 := fmt.Sprintf(`netsh interface ip set address name="Yuki Tunnel" static %s %s %s`, ipStr, maskStr, gatewayStr)
	if err := w.runNetshCommand(cmd1); err != nil {
		return fmt.Errorf("failed to set IP address: %w", err)
	}
	
	// 2. Установка DNS серверов
	cmd2 := `netsh interface ip set dns name="Yuki Tunnel" static 1.1.1.1 primary`
	if err := w.runNetshCommand(cmd2); err != nil {
		// DNS не критичен, продолжаем
		fmt.Printf("Warning: failed to set primary DNS: %v\n", err)
	}
	
	cmd3 := `netsh interface ip add dns name="Yuki Tunnel" 8.8.8.8 index=2`
	if err := w.runNetshCommand(cmd3); err != nil {
		fmt.Printf("Warning: failed to set secondary DNS: %v\n", err)
	}
	
	// 3. Добавляем маршрут по умолчанию через VPN (самое важное!)
	cmd4 := fmt.Sprintf(`route add 0.0.0.0 mask 0.0.0.0 %s metric 1`, gatewayStr)
	if err := w.runNetshCommand(cmd4); err != nil {
		return fmt.Errorf("failed to add default route: %w", err)
	}
	
	// 4. Добавляем более специфичные маршруты для перехвата всего трафика
	cmd5 := fmt.Sprintf(`route add 0.0.0.0 mask 128.0.0.0 %s metric 1`, gatewayStr)
	w.runNetshCommand(cmd5)
	
	cmd6 := fmt.Sprintf(`route add 128.0.0.0 mask 128.0.0.0 %s metric 1`, gatewayStr)
	w.runNetshCommand(cmd6)
	
	fmt.Printf("✅ Настроена маршрутизация через VPN туннель\n")
	return nil
}

func (w *WinTun) runNetshCommand(cmd string) error {
	var si syscall.StartupInfo
	var pi syscall.ProcessInformation
	
	cmdPtr, _ := syscall.UTF16PtrFromString("cmd /c " + cmd)
	err := syscall.CreateProcess(
		nil,
		cmdPtr,
		nil,
		nil,
		false,
		0x08000000, // CREATE_NO_WINDOW
		nil,
		nil,
		&si,
		&pi,
	)
	
	if err != nil {
		return fmt.Errorf("failed to execute command '%s': %v", cmd, err)
	}
	
	// Ждем завершения команды
	defer syscall.CloseHandle(pi.Process)
	defer syscall.CloseHandle(pi.Thread)
	
	syscall.WaitForSingleObject(pi.Process, syscall.INFINITE)
	
	return nil
}

// Alternative TAP implementation for compatibility
type TAPInterface struct {
	handle windows.Handle
	name   string
}

func (t *TAPInterface) Create(name string) error {
	// Open TAP-Windows adapter
	devicePath := `\\.\Global\` + name + `.tap`
	
	handle, err := windows.CreateFile(
		windows.StringToUTF16Ptr(devicePath),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_SYSTEM|windows.FILE_FLAG_OVERLAPPED,
		0,
	)
	
	if err != nil {
		return fmt.Errorf("failed to open TAP device: %w", err)
	}
	
	t.handle = handle
	t.name = name
	return nil
}

func (t *TAPInterface) Read(buf []byte) (int, error) {
	var bytesRead uint32
	err := windows.ReadFile(t.handle, buf, &bytesRead, nil)
	if err != nil {
		return 0, err
	}
	return int(bytesRead), nil
}

func (t *TAPInterface) Write(buf []byte) (int, error) {
	var bytesWritten uint32
	err := windows.WriteFile(t.handle, buf, &bytesWritten, nil)
	if err != nil {
		return 0, err
	}
	return int(bytesWritten), nil
}

func (t *TAPInterface) Close() error {
	if t.handle != windows.InvalidHandle {
		return windows.CloseHandle(t.handle)
	}
	return nil
}

func (t *TAPInterface) SetIP(ip net.IP, mask net.IPMask, gateway net.IP) error {
	// Используем netsh для настройки TAP интерфейса
	ipStr := ip.String()
	maskStr := net.IP(mask).String()
	gatewayStr := gateway.String()
	
	cmd := fmt.Sprintf(`netsh interface ip set address name="%s" static %s %s %s`, t.name, ipStr, maskStr, gatewayStr)
	
	var si syscall.StartupInfo
	var pi syscall.ProcessInformation
	
	cmdPtr, _ := syscall.UTF16PtrFromString("cmd /c " + cmd)
	err := syscall.CreateProcess(
		nil,
		cmdPtr,
		nil,
		nil,
		false,
		0x08000000,
		nil,
		nil,
		&si,
		&pi,
	)
	
	if err != nil {
		return fmt.Errorf("failed to set TAP IP: %v", err)
	}
	
	defer syscall.CloseHandle(pi.Process)
	defer syscall.CloseHandle(pi.Thread)
	syscall.WaitForSingleObject(pi.Process, syscall.INFINITE)
	
	return nil
}

// TUN interface abstraction
type Interface interface {
	Create(name string) error
	Read(buf []byte) (int, error)
	Write(buf []byte) (int, error) 
	Close() error
	SetIP(ip net.IP, mask net.IPMask, gateway net.IP) error
}

func NewTunInterface() Interface {
	// Try WinTun first, fallback to TAP
	return &WinTun{}
}

func NewTapInterface() Interface {
	return &TAPInterface{}
}
