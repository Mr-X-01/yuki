package crypto

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
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
	aead   cipher.AEAD
	key    [KeySize]byte
	sendNonce [NonceSize]byte
	recvNonce [NonceSize]byte
}

type Frame struct {
	Type   uint8  // 0=data, 1=ping, 2=pong
	Length uint32
	Data   []byte
}

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, errors.New("invalid key size")
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}

	c := &Cipher{aead: aead}
	copy(c.key[:], key)
	
	if _, err := rand.Read(c.sendNonce[:]); err != nil {
		return nil, err
	}
	if _, err := rand.Read(c.recvNonce[:]); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	incrementNonce(c.sendNonce[:])
	
	ciphertext := c.aead.Seal(nil, c.sendNonce[:], plaintext, nil)
	
	result := make([]byte, NonceSize+len(ciphertext))
	copy(result[:NonceSize], c.sendNonce[:])
	copy(result[NonceSize:], ciphertext)
	
	return result, nil
}

func (c *Cipher) Decrypt(data []byte) ([]byte, error) {
	if len(data) < NonceSize+TagSize {
		return nil, errors.New("invalid ciphertext length")
	}

	nonce := data[:NonceSize]
	ciphertext := data[NonceSize:]
	
	expectedNonce := c.recvNonce
	incrementNonce(expectedNonce[:])
	
	if subtle.ConstantTimeCompare(nonce, expectedNonce[:]) != 1 {
		return nil, errors.New("invalid nonce sequence")
	}
	
	copy(c.recvNonce[:], nonce)
	
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}
	
	return plaintext, nil
}

func (c *Cipher) EncryptFrame(frame *Frame) ([]byte, error) {
	frameData := make([]byte, 5+len(frame.Data))
	frameData[0] = frame.Type
	binary.BigEndian.PutUint32(frameData[1:5], frame.Length)
	copy(frameData[5:], frame.Data)
	
	encrypted, err := c.Encrypt(frameData)
	if err != nil {
		return nil, err
	}
	
	result := make([]byte, 4+len(encrypted))
	binary.BigEndian.PutUint32(result[:4], uint32(len(encrypted)))
	copy(result[4:], encrypted)
	
	return result, nil
}

func (c *Cipher) DecryptFrame(data []byte) (*Frame, error) {
	if len(data) < 4 {
		return nil, errors.New("invalid frame length")
	}
	
	length := binary.BigEndian.Uint32(data[:4])
	if len(data) < int(4+length) {
		return nil, errors.New("incomplete frame")
	}
	
	encrypted := data[4:4+length]
	
	frameData, err := c.Decrypt(encrypted)
	if err != nil {
		return nil, err
	}
	
	if len(frameData) < 5 {
		return nil, errors.New("invalid frame data")
	}
	
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

func incrementNonce(nonce []byte) {
	for i := len(nonce) - 1; i >= 0; i-- {
		nonce[i]++
		if nonce[i] != 0 {
			break
		}
	}
}
