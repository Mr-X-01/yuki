// +build linux

package tunnel

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

const (
	TUNSETIFF = 0x400454ca
	IFF_TUN   = 0x0001
	IFF_NO_PI = 0x1000
)

type ifReq struct {
	Name  [16]byte
	Flags uint16
	pad   [22]byte
}

// CreateTunInterface creates a TUN interface on Linux
func CreateTunInterface(name string, ip string, mtu int) (*os.File, error) {
	// Open TUN device
	file, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open /dev/net/tun: %v", err)
	}

	// Prepare interface request
	var ifr ifReq
	copy(ifr.Name[:], name)
	ifr.Flags = IFF_TUN | IFF_NO_PI

	// Create TUN interface
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(TUNSETIFF), uintptr(unsafe.Pointer(&ifr)))
	if errno != 0 {
		file.Close()
		return nil, fmt.Errorf("failed to create TUN interface: %v", errno)
	}

	// Get actual interface name
	actualName := string(ifr.Name[:])
	for i, b := range ifr.Name {
		if b == 0 {
			actualName = string(ifr.Name[:i])
			break
		}
	}

	// Configure interface
	if err := configureInterface(actualName, ip, mtu); err != nil {
		file.Close()
		return nil, err
	}

	return file, nil
}

// Configure TUN interface with IP and MTU
func configureInterface(name, ip string, mtu int) error {
	// Set IP address
	cmd := exec.Command("ip", "addr", "add", ip, "dev", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set IP address: %v, output: %s", err, output)
	}

	// Set MTU
	cmd = exec.Command("ip", "link", "set", "dev", name, "mtu", fmt.Sprintf("%d", mtu))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set MTU: %v, output: %s", err, output)
	}

	// Bring interface up
	cmd = exec.Command("ip", "link", "set", "dev", name, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring interface up: %v, output: %s", err, output)
	}

	return nil
}

// TunConn wraps os.File to implement net.Conn interface
type TunConn struct {
	*os.File
	localAddr  net.Addr
	remoteAddr net.Addr
}

func NewTunConn(file *os.File, localIP, remoteIP string) *TunConn {
	return &TunConn{
		File:       file,
		localAddr:  &net.IPAddr{IP: net.ParseIP(localIP)},
		remoteAddr: &net.IPAddr{IP: net.ParseIP(remoteIP)},
	}
}

func (t *TunConn) LocalAddr() net.Addr {
	return t.localAddr
}

func (t *TunConn) RemoteAddr() net.Addr {
	return t.remoteAddr
}

func (t *TunConn) SetDeadline(deadline time.Time) error {
	// TUN interfaces don't support deadlines in the same way
	return nil
}

func (t *TunConn) SetReadDeadline(deadline time.Time) error {
	return nil
}

func (t *TunConn) SetWriteDeadline(deadline time.Time) error {
	return nil
}
