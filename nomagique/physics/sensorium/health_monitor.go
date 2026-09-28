package sensorium

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
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
func (s PhysicsSnapshot) Marshal() ([]byte, error) {
	if s.Schema != "sensorium-physics-health/v1" || s.Population < 0 || !s.Reading.IsFinite() {
		return nil, fmt.Errorf("invalid physics snapshot contract")
	}
	return sonic.Marshal(s)
}

// PhysicsMonitor is a replaceable observational boundary, never an integrator.
// Watching requests at most five snapshots/second from the existing producer.
// When unwatched it does not force particle/grid readback on the live engine.
type PhysicsMonitor struct {
	untilNs atomic.Int64
	lastNs  atomic.Int64
	data    atomic.Pointer[[]byte]
	failure atomic.Pointer[string]
}

func (m *PhysicsMonitor) WantsSnapshot() bool {
	now := time.Now().UnixNano()
	until := m.untilNs.Load()
	last := m.lastNs.Load()

	return now < until && (now-last) >= int64(200*time.Millisecond)
}

func (m *PhysicsMonitor) Observe(s PhysicsSnapshot) error {
	raw, err := s.Marshal()

	if err != nil {
		errStr := err.Error()
		m.failure.Store(&errStr)

		return err
	}

	m.data.Store(&raw)
	m.failure.Store(nil)
	m.lastNs.Store(time.Now().UnixNano())

	return nil
}

func (m *PhysicsMonitor) Reject(err error) {
	if err == nil {
		return
	}

	errStr := err.Error()
	m.failure.Store(&errStr)
}

func (m *PhysicsMonitor) Poll() ([]byte, int) {
	m.untilNs.Store(time.Now().Add(5 * time.Second).UnixNano())

	if failPtr := m.failure.Load(); failPtr != nil && *failPtr != "" {
		b, err := json.Marshal(map[string]string{"error": *failPtr})

		if err != nil {
			return []byte(`{"error":"marshal failure"}`), http.StatusUnprocessableEntity
		}

		return b, http.StatusUnprocessableEntity
	}

	dataPtr := m.data.Load()

	if dataPtr == nil || len(*dataPtr) == 0 {
		return []byte(`{"error":"No published physics reading yet"}`), http.StatusServiceUnavailable
	}

	return append([]byte(nil), (*dataPtr)...), http.StatusOK
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
		if _, err := w.Write([]byte(PhysicsMonitorHTML)); err != nil {
			return
		}
	case "/physics/health":
		b, status := m.Poll()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write(b); err != nil {
			return
		}
	default:
		http.NotFound(w, r)
	}
}
