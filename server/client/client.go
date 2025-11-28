package client

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Client struct {
	ID          string    `json:"id"`
	Secret      string    `json:"secret"`
	Name        string    `json:"name"`
	Created     time.Time `json:"created"`
	LastSeen    time.Time `json:"last_seen"`
	Active      bool      `json:"active"`
	Blocked     bool      `json:"blocked"`
	BytesUp     int64     `json:"bytes_up"`
	BytesDown   int64     `json:"bytes_down"`
	MaxBandwidth int64     `json:"max_bandwidth"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type Manager struct {
	clients map[string]*Client
	mutex   sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
	}
}

func (m *Manager) CreateClient(name string, maxBandwidth int64, expiresAt *time.Time) *Client {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	client := &Client{
		ID:           uuid.New().String(),
		Secret:       generateSecret(),
		Name:         name,
		Created:      time.Now(),
		Active:       false,
		Blocked:      false,
		MaxBandwidth: maxBandwidth,
		ExpiresAt:    expiresAt,
	}

	m.clients[client.ID] = client
	return client
}

func (m *Manager) GetClient(id string) (*Client, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	client, exists := m.clients[id]
	return client, exists
}

func (m *Manager) ListClients() []*Client {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	clients := make([]*Client, 0, len(m.clients))
	for _, client := range m.clients {
		clients = append(clients, client)
	}
	return clients
}

func (m *Manager) DeleteClient(id string) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if _, exists := m.clients[id]; exists {
		delete(m.clients, id)
		return true
	}
	return false
}

func (m *Manager) BlockClient(id string) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if client, exists := m.clients[id]; exists {
		client.Blocked = true
		return true
	}
	return false
}

func (m *Manager) UnblockClient(id string) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if client, exists := m.clients[id]; exists {
		client.Blocked = false
		return true
	}
	return false
}

func (m *Manager) UpdateTraffic(id string, bytesUp, bytesDown int64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if client, exists := m.clients[id]; exists {
		client.BytesUp += bytesUp
		client.BytesDown += bytesDown
		client.LastSeen = time.Now()
	}
}

func (m *Manager) SetActive(id string, active bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if client, exists := m.clients[id]; exists {
		client.Active = active
		if active {
			client.LastSeen = time.Now()
		}
	}
}

func (m *Manager) IsAuthorized(id, secret string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	client, exists := m.clients[id]
	if !exists || client.Blocked {
		return false
	}
	
	// Check expiration
	if client.ExpiresAt != nil && time.Now().After(*client.ExpiresAt) {
		return false
	}
	
	return client.Secret == secret
}

func (m *Manager) SaveToJSON(filename string) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	data, err := json.MarshalIndent(m.clients, "", "  ")
	if err != nil {
		return err
	}
	
	return writeFile(filename, data)
}

func (m *Manager) LoadFromJSON(filename string) error {
	data, err := readFile(filename)
	if err != nil {
		return err
	}
	
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	return json.Unmarshal(data, &m.clients)
}

func generateSecret() string {
	return uuid.New().String() + uuid.New().String()
}

// Stub functions for file operations
func writeFile(filename string, data []byte) error {
    // Basic filesystem implementation; can be replaced with DB/Redis backend later
    return os.WriteFile(filename, data, 0600)
}

func readFile(filename string) ([]byte, error) {
    // Basic filesystem implementation; can be replaced with DB/Redis backend later
    return os.ReadFile(filename)
}
