package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// ContentHash represents a SHA-256 hash of file content
type ContentHash [32]byte

// String returns the hex representation of the hash
func (h ContentHash) String() string {
	return hex.EncodeToString(h[:])
}

// ContentHashFromString creates a ContentHash from a hex string
func ContentHashFromString(s string) (ContentHash, error) {
	var h ContentHash
	bytes, err := hex.DecodeString(s)
	if err != nil {
		return h, err
	}
	if len(bytes) != 32 {
		return h, fmt.Errorf("invalid hash length: expected 32 bytes, got %d", len(bytes))
	}
	copy(h[:], bytes)
	return h, nil
}

// ContentStore provides content-addressable storage
type ContentStore struct {
	mu       sync.RWMutex
	basePath string
	files    map[ContentHash]string // hash -> file path
}

// NewContentStore creates a new content store
func NewContentStore(basePath string) (*ContentStore, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &ContentStore{
		basePath: basePath,
		files:    make(map[ContentHash]string),
	}, nil
}

// AddFile adds a file to the content store and returns its hash
func (cs *ContentStore) AddFile(filePath string) (ContentHash, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return ContentHash{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ContentHash{}, fmt.Errorf("failed to compute hash: %w", err)
	}

	var contentHash ContentHash
	copy(contentHash[:], hash.Sum(nil))

	// Copy file to storage
	storagePath := filepath.Join(cs.basePath, contentHash.String())
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		src, err := os.Open(filePath)
		if err != nil {
			return ContentHash{}, fmt.Errorf("failed to open source file: %w", err)
		}
		defer src.Close()

		dst, err := os.Create(storagePath)
		if err != nil {
			return ContentHash{}, fmt.Errorf("failed to create storage file: %w", err)
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			return ContentHash{}, fmt.Errorf("failed to copy file: %w", err)
		}
	}

	cs.mu.Lock()
	cs.files[contentHash] = storagePath
	cs.mu.Unlock()

	return contentHash, nil
}

// AddBytes adds raw bytes to the content store and returns its hash
func (cs *ContentStore) AddBytes(data []byte) (ContentHash, error) {
	hash := sha256.Sum256(data)
	var contentHash ContentHash
	copy(contentHash[:], hash[:])

	storagePath := filepath.Join(cs.basePath, contentHash.String())
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		if err := os.WriteFile(storagePath, data, 0644); err != nil {
			return ContentHash{}, fmt.Errorf("failed to write file: %w", err)
		}
	}

	cs.mu.Lock()
	cs.files[contentHash] = storagePath
	cs.mu.Unlock()

	return contentHash, nil
}

// GetFile retrieves a file by its hash
func (cs *ContentStore) GetFile(hash ContentHash) ([]byte, error) {
	cs.mu.RLock()
	storagePath, exists := cs.files[hash]
	cs.mu.RUnlock()

	if !exists {
		// Try to find it in storage directory
		storagePath = filepath.Join(cs.basePath, hash.String())
		if _, err := os.Stat(storagePath); os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", hash.String())
		}
	}

	data, err := os.ReadFile(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return data, nil
}

// HasFile checks if a file exists in the store
func (cs *ContentStore) HasFile(hash ContentHash) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if _, exists := cs.files[hash]; exists {
		return true
	}

	storagePath := filepath.Join(cs.basePath, hash.String())
	_, err := os.Stat(storagePath)
	return err == nil
}

// GetFilePath returns the file path for a given hash
func (cs *ContentStore) GetFilePath(hash ContentHash) (string, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if path, exists := cs.files[hash]; exists {
		return path, nil
	}

	storagePath := filepath.Join(cs.basePath, hash.String())
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		return "", fmt.Errorf("file not found: %s", hash.String())
	}

	return storagePath, nil
}

// DeleteFile removes a file from the store
func (cs *ContentStore) DeleteFile(hash ContentHash) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	storagePath, exists := cs.files[hash]
	if !exists {
		storagePath = filepath.Join(cs.basePath, hash.String())
	}

	if err := os.Remove(storagePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	delete(cs.files, hash)
	return nil
}

