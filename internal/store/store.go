// Package store guarda el último snapshot de Google en memoria y en un JSON
// en disco, para poder servir aunque Google no responda.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"home-dashboard/internal/agenda"
)

// Snapshot es el estado completo que muestra el dashboard.
type Snapshot struct {
	Events   []agenda.Event `json:"events"`
	Tasks    []agenda.Task  `json:"tasks"`
	SyncedAt time.Time      `json:"synced_at"`
	Err      string         `json:"err,omitempty"` // último error de sincronización
}

// Store es seguro para uso concurrente.
type Store struct {
	mu   sync.RWMutex
	snap Snapshot
	hash string
	path string // vacío = no persistir
}

// New crea el store y carga el cache de disco si existe.
func New(path string) *Store {
	s := &Store{path: path}
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &s.snap)
		}
	}
	s.rehash()
	return s
}

// Get devuelve el snapshot actual. Los slices no deben modificarse.
func (s *Store) Get() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

// Hash identifica el contenido visible; cambia solo si cambian los datos o
// el estado de error.
func (s *Store) Hash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hash
}

// Update reemplaza los datos tras una sincronización correcta.
func (s *Store) Update(events []agenda.Event, tasks []agenda.Task, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap = Snapshot{Events: events, Tasks: tasks, SyncedAt: at}
	s.rehash()
	return s.save()
}

// Fail registra un error de sincronización conservando los datos anteriores.
func (s *Store) Fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Err = err.Error()
	s.rehash()
	_ = s.save()
}

func (s *Store) rehash() {
	b, _ := json.Marshal(struct {
		E      []agenda.Event
		T      []agenda.Task
		Failed bool
	}{s.snap.Events, s.snap.Tasks, s.snap.Err != ""})
	sum := sha256.Sum256(b)
	s.hash = hex.EncodeToString(sum[:6])
}

func (s *Store) save() error {
	if s.path == "" {
		return nil
	}
	b, err := json.Marshal(s.snap)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
