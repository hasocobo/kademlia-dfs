package storage

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/kademlia-dfs/kademlia-dfs/internal/dht"
)

// ProviderRecord represents a provider record stored in the DHT
type ProviderRecord struct {
	Hash      string `json:"hash"`      // Content hash
	IP        string `json:"ip"`        // Provider IP address
	Port      int    `json:"port"`      // Provider port
	NodeID    string `json:"node_id"`   // Provider node ID
	Timestamp int64  `json:"timestamp"` // Record timestamp
}

// ProviderStore manages provider records
type ProviderStore struct {
	mu       sync.RWMutex
	providers map[string][]*ProviderRecord // hash -> providers
}

// NewProviderStore creates a new provider store
func NewProviderStore() *ProviderStore {
	return &ProviderStore{
		providers: make(map[string][]*ProviderRecord),
	}
}

// AddProvider adds a provider record for a content hash
func (ps *ProviderStore) AddProvider(hash string, node *dht.Node) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	record := &ProviderRecord{
		Hash:      hash,
		IP:        node.IP,
		Port:      node.Port,
		NodeID:    node.ID.String(),
		Timestamp: time.Now().Unix(),
	}

	// Check if provider already exists
	for i, existing := range ps.providers[hash] {
		if existing.NodeID == record.NodeID {
			ps.providers[hash][i] = record
			return
		}
	}

	ps.providers[hash] = append(ps.providers[hash], record)
}

// GetProviders returns all provider records for a content hash
func (ps *ProviderStore) GetProviders(hash string) []*ProviderRecord {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	providers := ps.providers[hash]
	result := make([]*ProviderRecord, len(providers))
	copy(result, providers)
	return result
}

// RemoveProvider removes a provider record
func (ps *ProviderStore) RemoveProvider(hash string, nodeID string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	providers := ps.providers[hash]
	for i, provider := range providers {
		if provider.NodeID == nodeID {
			ps.providers[hash] = append(providers[:i], providers[i+1:]...)
			break
		}
	}
}

// SerializeProviderRecord serializes a provider record to bytes
func SerializeProviderRecord(record *ProviderRecord) ([]byte, error) {
	return json.Marshal(record)
}

// DeserializeProviderRecord deserializes bytes to a provider record
func DeserializeProviderRecord(data []byte) (*ProviderRecord, error) {
	var record ProviderRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

