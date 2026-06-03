package perspectives

import (
	"strings"
	"sync"
	"sync/atomic"
)

/*
TelemetryRegistry holds the dynamic dashboard source manifest the UI builds from.
Sources register when measurements arrive; layout payloads read from this registry.
*/
type TelemetryRegistry struct {
	mu        sync.RWMutex
	sources   []string
	labels    map[string]string
	version   atomic.Uint64
	listeners []func()
}

var defaultTelemetryRegistry = NewTelemetryRegistry()

/*
DefaultTelemetryRegistry is the process-wide telemetry manifest.
*/
func DefaultTelemetryRegistry() *TelemetryRegistry {
	return defaultTelemetryRegistry
}

/*
NewTelemetryRegistry constructs an empty telemetry manifest.
*/
func NewTelemetryRegistry() *TelemetryRegistry {
	return &TelemetryRegistry{
		labels: make(map[string]string),
	}
}

/*
ResetTelemetryRegistryForTest clears the default registry between isolated runs.
*/
func ResetTelemetryRegistryForTest() {
	defaultTelemetryRegistry = NewTelemetryRegistry()
}

/*
BootstrapTelemetryManifest seeds the dashboard manifest before the first layout frame.
Runtime measurements can still register additional sources later.
*/
func BootstrapTelemetryManifest() {
	for name, label := range sourceDisplayLabels {
		DefaultTelemetryRegistry().Register(name, label)
	}
}

/*
ObserveMeasurement registers the measurement source when it has a wire name.
*/
func (registry *TelemetryRegistry) ObserveMeasurement(measurement Measurement) {
	name := measurement.Source.String()

	if name == "" {
		return
	}

	registry.Register(name, SourceDisplayLabel(name))
}

/*
Register adds one source when it is not already present.
*/
func (registry *TelemetryRegistry) Register(name, label string) bool {
	trimmed := strings.TrimSpace(name)

	if trimmed == "" {
		return false
	}

	if label == "" {
		label = SourceDisplayLabel(trimmed)
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	if _, exists := registry.labels[trimmed]; exists {
		return false
	}

	registry.sources = append(registry.sources, trimmed)
	registry.labels[trimmed] = label
	registry.version.Add(1)
	registry.notifyLocked()

	return true
}

/*
Names returns registered sources in first-seen order.
*/
func (registry *TelemetryRegistry) Names() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	names := make([]string, len(registry.sources))
	copy(names, registry.sources)

	return names
}

/*
LabelMap returns source-name to display-label entries for layout payloads.
*/
func (registry *TelemetryRegistry) LabelMap() map[string]string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	labels := make(map[string]string, len(registry.labels))

	for name, label := range registry.labels {
		labels[name] = label
	}

	return labels
}

/*
Label returns the display label for one registered source name.
*/
func (registry *TelemetryRegistry) Label(name string) string {
	registry.mu.RLock()
	label, ok := registry.labels[name]
	registry.mu.RUnlock()

	if ok {
		return label
	}

	return SourceDisplayLabel(name)
}

/*
Version increments whenever the manifest changes.
*/
func (registry *TelemetryRegistry) Version() uint64 {
	return registry.version.Load()
}

/*
Subscribe registers a listener invoked after each manifest change.
*/
func (registry *TelemetryRegistry) Subscribe(listener func()) {
	if listener == nil {
		return
	}

	registry.mu.Lock()
	registry.listeners = append(registry.listeners, listener)
	registry.mu.Unlock()
}

func (registry *TelemetryRegistry) notifyLocked() {
	listeners := make([]func(), len(registry.listeners))
	copy(listeners, registry.listeners)

	for _, listener := range listeners {
		listener()
	}
}
