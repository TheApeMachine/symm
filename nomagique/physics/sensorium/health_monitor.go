package sensorium

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// PhysicsSnapshot is a versioned, finite-only wire contract. The full Reading
// is copied by value; no GPU buffer or caller-owned particle slice is retained.
// Material and grid energies are alternate representations, NOT additive stores.
type PhysicsSnapshot struct {
	Schema     string  `json:"schema"`
	Version    uint64  `json:"version"`
	At         int64   `json:"at_unix_nano"`
	Population int     `json:"population"`
	Reading    Reading `json:"reading"`
}

func NewPhysicsSnapshot(version uint64, at time.Time, population int, reading Reading) (PhysicsSnapshot, error) {
	if population < 0 || !reading.IsFinite() {
		return PhysicsSnapshot{}, fmt.Errorf("refuse invalid physics snapshot")
	}
	return PhysicsSnapshot{"sensorium-physics-health/v1", version, at.UnixNano(), population, reading}, nil
}
func (s PhysicsSnapshot) Marshal() ([]byte, error) {
	if s.Schema != "sensorium-physics-health/v1" || s.Population < 0 || !s.Reading.IsFinite() {
		return nil, fmt.Errorf("invalid physics snapshot contract")
	}
	return json.Marshal(s)
}

// PhysicsMonitor is a replaceable observational boundary, never an integrator.
// Watching requests at most five snapshots/second from the existing producer.
// When unwatched it does not force particle/grid readback on the live engine.
type PhysicsMonitor struct {
	mu          sync.Mutex
	until, last time.Time
	data        []byte
	failure     string
}

func (m *PhysicsMonitor) WantsSnapshot() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	return now.Before(m.until) && now.Sub(m.last) >= 200*time.Millisecond
}
func (m *PhysicsMonitor) Observe(s PhysicsSnapshot) error {
	bytes, err := s.Marshal()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.failure = err.Error()
		return err
	}
	m.data = bytes
	m.failure = ""
	m.last = time.Now()
	return nil
}
func (m *PhysicsMonitor) Reject(err error) {
	if err == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failure = err.Error()
}

func (m *PhysicsMonitor) Poll() ([]byte, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.until = time.Now().Add(5 * time.Second)
	if m.failure != "" {
		b, _ := json.Marshal(map[string]string{"error": m.failure})
		return b, http.StatusUnprocessableEntity
	}
	if len(m.data) == 0 {
		return []byte(`{"error":"No published physics reading yet"}`), http.StatusServiceUnavailable
	}
	return append([]byte(nil), m.data...), http.StatusOK
}

// ServeHTTP also makes the same component directly testable without Fiber.
func (m *PhysicsMonitor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case "/physics", "/physics/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(PhysicsMonitorHTML))
	case "/physics/health":
		b, status := m.Poll()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(b)
	default:
		http.NotFound(w, r)
	}
}
