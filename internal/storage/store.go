package storage

import (
	"errors"
	"sync"

	"github.com/kademlia-dfs/kademlia-dfs/internal/dht"
)

// KVStore is a simple key-value store that implements dht.Storage
type KVStore struct {
	mu    sync.RWMutex
	store map[string][]byte
}

// NewKVStore creates a new key-value store
func NewKVStore() *KVStore {
	return &KVStore{
		store: make(map[string][]byte),
	}
}

// Store stores a key-value pair
func (kv *KVStore) Store(key dht.NodeID, value []byte) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.store[key.String()] = value
	return nil
}

// Retrieve retrieves a value by key
func (kv *KVStore) Retrieve(key dht.NodeID) ([]byte, error) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()
	value, exists := kv.store[key.String()]
	if !exists {
		return nil, ErrKeyNotFound
	}
	return value, nil
}

// Remove removes a key-value pair
func (kv *KVStore) Remove(key dht.NodeID) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	delete(kv.store, key.String())
	return nil
}

var (
	ErrKeyNotFound = errors.New("key not found")
)

