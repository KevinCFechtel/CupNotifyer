package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const currentVersion = 1

type Store interface {
	IsSent(targetID, imageID, updateID string) bool
	MarkSent(targetID, imageID, updateID string) error
}

type fileData struct {
	Version int                          `json:"version"`
	Targets map[string]map[string]string `json:"targets"`
}

type FileStore struct {
	mu   sync.RWMutex
	path string
	data fileData
}

func NewFileStore(path string) (*FileStore, error) {
	store := &FileStore{
		path: path,
		data: fileData{
			Version: currentVersion,
			Targets: make(map[string]map[string]string),
		},
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}
	if err := json.Unmarshal(raw, &store.data); err != nil {
		return nil, fmt.Errorf("decode state file: %w", err)
	}
	if store.data.Version != currentVersion {
		return nil, fmt.Errorf("unsupported state version %d", store.data.Version)
	}
	if store.data.Targets == nil {
		store.data.Targets = make(map[string]map[string]string)
	}
	return store, nil
}

func (s *FileStore) IsSent(targetID, imageID, updateID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Targets[targetID][imageID] == updateID
}

func (s *FileStore) MarkSent(targetID, imageID, updateID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.Targets[targetID] == nil {
		s.data.Targets[targetID] = make(map[string]string)
	}
	previous, existed := s.data.Targets[targetID][imageID]
	s.data.Targets[targetID][imageID] = updateID

	if err := s.save(); err != nil {
		if existed {
			s.data.Targets[targetID][imageID] = previous
		} else {
			delete(s.data.Targets[targetID], imageID)
		}
		return err
	}
	return nil
}

func (s *FileStore) save() error {
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	raw = append(raw, '\n')

	temp, err := os.CreateTemp(directory, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("set state permissions: %w", err)
	}
	if _, err := temp.Write(raw); err != nil {
		temp.Close()
		return fmt.Errorf("write state: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync state: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close state: %w", err)
	}
	if err := os.Rename(tempName, s.path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}
	return nil
}
