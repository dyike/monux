package service

import (
	"sync"

	"github.com/dyike/monux/internal/monitor"
)

// SynchronizedBackend serializes all access to a platform backend. The same
// wrapper is shared by direct HTTP operations and the peer-aware controller.
type SynchronizedBackend struct {
	backend monitor.Backend
	mu      sync.Mutex
}

func NewSynchronizedBackend(backend monitor.Backend) *SynchronizedBackend {
	return &SynchronizedBackend{backend: backend}
}

func (b *SynchronizedBackend) CurrentInput() (monitor.Input, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.backend.CurrentInput()
}

func (b *SynchronizedBackend) SetInput(input monitor.Input) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.backend.SetInput(input)
}

func (b *SynchronizedBackend) Detect() ([]monitor.Display, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.backend.Detect()
}

func (b *SynchronizedBackend) SupportedInputs() ([]monitor.Input, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.backend.SupportedInputs()
}
