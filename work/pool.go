package work

import (
	"context"
	"runtime"

	"github.com/phuslu/log"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

const (
	defaultMinWorkers = 2
)

// DefaultMaxWorkers caps short-lived jobs (regime eval, scheduled work).
// Long-lived loops (feed, UI server, websocket pumps) stay on raw goroutines.
func DefaultMaxWorkers() int {
	n := runtime.NumCPU() * 2
	if n < defaultMinWorkers {
		return defaultMinWorkers
	}
	return n
}

// NewPool returns the process-wide qpool used for bounded concurrent jobs.
func NewPool(ctx context.Context) *qpool.Q {
	cfg := qpool.NewConfig()
	cfg.Scaler = nil
	cfg.TelemetryPublish = publishTelemetry
	return qpool.NewQ(ctx, defaultMinWorkers, DefaultMaxWorkers(), cfg)
}

func publishTelemetry(ev qpool.Event) {
	fields := []any{"component", ev.Component, "op", ev.Op}
	for _, field := range ev.Fields {
		fields = append(fields, field.Key, field.Value)
	}
	if ev.Err != nil {
		_ = errnie.Error(ev.Err, append(fields, "event", "qpool")...)
		return
	}
	msg := ev.Message
	if msg == "" {
		msg = ev.Op
	}
	switch ev.Level {
	case log.WarnLevel:
		errnie.Warn(msg, append(fields, "event", "qpool")...)
	case log.DebugLevel, log.TraceLevel:
		errnie.Debug(msg, append(fields, "event", "qpool")...)
	default:
		errnie.Info(msg, append(fields, "event", "qpool")...)
	}
}
