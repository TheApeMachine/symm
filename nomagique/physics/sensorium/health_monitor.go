package sensorium

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/bytedance/sonic"
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
func (physicsSnapshot PhysicsSnapshot) Marshal() ([]byte, error) {
	if physicsSnapshot.Schema != "sensorium-physics-health/v1" || physicsSnapshot.Population < 0 || !physicsSnapshot.Reading.IsFinite() {
		return nil, fmt.Errorf("invalid physics snapshot contract")
	}
	return sonic.Marshal(physicsSnapshot)
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

func (physicsMonitor *PhysicsMonitor) WantsSnapshot() bool {
	physicsMonitor.mu.Lock()
	defer physicsMonitor.mu.Unlock()
	now := time.Now()
	return now.Before(physicsMonitor.until) && now.Sub(physicsMonitor.last) >= 200*time.Millisecond
}
func (physicsMonitor *PhysicsMonitor) Observe(s PhysicsSnapshot) error {
	bytes, err := s.Marshal()
	physicsMonitor.mu.Lock()
	defer physicsMonitor.mu.Unlock()
	if err != nil {
		physicsMonitor.failure = err.Error()
		return err
	}
	physicsMonitor.data = bytes
	physicsMonitor.failure = ""
	physicsMonitor.last = time.Now()
	return nil
}
func (physicsMonitor *PhysicsMonitor) Reject(err error) {
	if err == nil {
		return
	}
	physicsMonitor.mu.Lock()
	defer physicsMonitor.mu.Unlock()
	physicsMonitor.failure = err.Error()
}

func (physicsMonitor *PhysicsMonitor) Poll() ([]byte, int) {
	physicsMonitor.mu.Lock()
	defer physicsMonitor.mu.Unlock()
	physicsMonitor.until = time.Now().Add(5 * time.Second)
	if physicsMonitor.failure != "" {
		b, _ := json.Marshal(map[string]string{"error": physicsMonitor.failure})
		return b, http.StatusUnprocessableEntity
	}
	if len(physicsMonitor.data) == 0 {
		return []byte(`{"error":"No published physics reading yet"}`), http.StatusServiceUnavailable
	}
	return append([]byte(nil), physicsMonitor.data...), http.StatusOK
}

// ServeHTTP also makes the same component directly testable without Fiber.
func (physicsMonitor *PhysicsMonitor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		b, status := physicsMonitor.Poll()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(b)
	default:
		http.NotFound(w, r)
	}
}
