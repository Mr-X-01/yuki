package crypto

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	KeySize   = 32
	NonceSize = 24
	TagSize   = 16
)

type Cipher struct {
	aead       cipher.AEAD
	key        [KeySize]byte
	sendNonce  [NonceSize]byte
	recvNonce  [NonceSize]byte
	isClient   bool
}

func NewCipher(key []byte) (*Cipher, error) {
	return NewCipherWithRole(key, false)
}

func NewClientCipher(key []byte) (*Cipher, error) {
	return NewCipherWithRole(key, true)
}

func NewServerCipher(key []byte) (*Cipher, error) {
	return NewCipherWithRole(key, false)
}

func NewCipherWithRole(key []byte, isClient bool) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, errors.New("invalid key size")
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}

	c := &Cipher{aead: aead, isClient: isClient}
	copy(c.key[:], key)
	
	// Initialize nonces based on role to avoid conflicts
	if isClient {
		// Client uses even nonces for sending, odd for receiving
		copy(c.sendNonce[:], []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		copy(c.recvNonce[:], []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01})
	} else {
		// Server uses odd nonces for sending, even for receiving
		copy(c.sendNonce[:], []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01})
		copy(c.recvNonce[:], []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	}

	return c, nil
}

func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	// Increment nonce by 2 to maintain even/odd separation
	incrementNonceBy(c.sendNonce[:], 2)
	
	// Encrypt with XChaCha20-Poly1305
	ciphertext := c.aead.Seal(nil, c.sendNonce[:], plaintext, nil)
	
	// Prepend nonce to ciphertext
	result := make([]byte, NonceSize+len(ciphertext))
	copy(result[:NonceSize], c.sendNonce[:])
	copy(result[NonceSize:], ciphertext)
	
	return result, nil
}

func (c *Cipher) Decrypt(data []byte) ([]byte, error) {
	if len(data) < NonceSize+TagSize {
		return nil, errors.New("invalid ciphertext length")
	}

	// Extract nonce
	nonce := data[:NonceSize]
	ciphertext := data[NonceSize:]
	
	// Verify nonce sequence to prevent replay attacks
	if err := c.verifyAndUpdateReceiveNonce(nonce); err != nil {
		return nil, fmt.Errorf("invalid nonce sequence: %w", err)
	}
	
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}
	
	return plaintext, nil
}

func incrementNonce(nonce []byte) {
	incrementNonceBy(nonce, 1)
}

func incrementNonceBy(nonce []byte, increment int) {
	carry := increment
	for i := len(nonce) - 1; i >= 0 && carry > 0; i-- {
		sum := int(nonce[i]) + carry
		nonce[i] = byte(sum & 0xFF)
		carry = sum >> 8
	}
}

func (c *Cipher) verifyAndUpdateReceiveNonce(receivedNonce []byte) error {
	// For initial packets, accept any valid nonce pattern
	if isZeroNonce(c.recvNonce[:]) {
		copy(c.recvNonce[:], receivedNonce)
		return nil
	}
	
	// Check if this is the next expected nonce (increment by 2)
	expectedNonce := make([]byte, NonceSize)
	copy(expectedNonce, c.recvNonce[:])
	incrementNonceBy(expectedNonce, 2)
	
	// Allow some tolerance for out-of-order packets
	for i := 0; i < 10; i++ {
		if bytesEqual(expectedNonce, receivedNonce) {
			copy(c.recvNonce[:], receivedNonce)
			return nil
		}
		incrementNonceBy(expectedNonce, 2)
	}
	
	return errors.New("nonce too far ahead or replay detected")
}

func isZeroNonce(nonce []byte) bool {
	for _, b := range nonce {
		if b != 0 {
			return false
		}
	}
	return true
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Frame encryption for tunnel protocol
type Frame struct {
	Type   uint8  // 0=data, 1=ping, 2=pong
	Length uint32
	Data   []byte
}

func (c *Cipher) EncryptFrame(frame *Frame) ([]byte, error) {
	// Serialize frame
	frameData := make([]byte, 5+len(frame.Data))
	frameData[0] = frame.Type
	binary.BigEndian.PutUint32(frameData[1:5], frame.Length)
	copy(frameData[5:], frame.Data)
	
	// Encrypt frame data (except length prefix)
	encrypted, err := c.Encrypt(frameData)
	if err != nil {
		return nil, err
	}
	
	// Prepend unencrypted length
	result := make([]byte, 4+len(encrypted))
	binary.BigEndian.PutUint32(result[:4], uint32(len(encrypted)))
	copy(result[4:], encrypted)
	
	return result, nil
}

func (c *Cipher) DecryptFrame(data []byte) (*Frame, error) {
	if len(data) < 4 {
		return nil, errors.New("invalid frame length")
	}
	
	// Extract length and encrypted data
	length := binary.BigEndian.Uint32(data[:4])
	if len(data) < int(4+length) {
		return nil, errors.New("incomplete frame")
	}
	
	encrypted := data[4:4+length]
	
	// Decrypt frame data
	frameData, err := c.Decrypt(encrypted)
	if err != nil {
		return nil, err
	}
	
	if len(frameData) < 5 {
		return nil, errors.New("invalid frame data")
	}
	
	// Parse frame
	frame := &Frame{
		Type:   frameData[0],
		Length: binary.BigEndian.Uint32(frameData[1:5]),
		Data:   frameData[5:],
	}
	
	if len(frame.Data) != int(frame.Length) {
		return nil, errors.New("frame length mismatch")
	}
	
	return frame, nil
}
