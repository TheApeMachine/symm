package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
)

const maxSystemLogs = 100

type LogSeverity string

const (
	LogInfo    LogSeverity = "info"
	LogWarning LogSeverity = "warn"
	LogError   LogSeverity = "error"
)

type LogEntry struct {
	Timestamp int64       `json:"timestamp"`
	Level     LogSeverity `json:"level"`
	Message   string      `json:"message"`
	System    string      `json:"system,omitempty"`
}

var (
	globalLogHookMu sync.RWMutex
	globalLogHook   func(*System, LogEntry)
)

/*
SetGlobalLogHook sets a process-wide hook invoked on every log emitted by any System.
*/
func SetGlobalLogHook(hook func(*System, LogEntry)) {
	globalLogHookMu.Lock()
	defer globalLogHookMu.Unlock()

	globalLogHook = hook
}

type RuntimeSystem interface {
	Name() string
	Context() context.Context
	Status() Stage
	Transition(Stage)
	Error(...error) error
	Info(msg string, args ...any)
	Warning(msg string, args ...any)
	AddCloser(io.Closer)
	Close() error
	Logs() []LogEntry
	OnLog(func(LogEntry))
}

type System struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	name    string
	status  *StatusTracker
	closers []io.Closer
	logs    []LogEntry
	logMu   sync.RWMutex
	onLog   func(LogEntry)
}

type Closer func() error

func (closer Closer) Close() error {
	if closer == nil {
		return nil
	}

	return closer()
}

func NewSystem(
	ctx context.Context,
	name string,
	closers ...io.Closer,
) *System {
	ctx, cancel := context.WithCancel(ctx)

	return &System{
		ctx:     ctx,
		cancel:  cancel,
		name:    name,
		status:  NewStatus(),
		closers: closers,
		logs:    make([]LogEntry, 0, 16),
	}
}

func (system *System) Name() string             { return system.name }
func (system *System) Context() context.Context { return system.ctx }

func (system *System) appendLog(entry LogEntry) {
	system.logMu.Lock()

	if len(system.logs) >= maxSystemLogs {
		system.logs = system.logs[1:]
	}

	system.logs = append(system.logs, entry)
	hook := system.onLog
	system.logMu.Unlock()

	if hook != nil {
		hook(entry)
	}

	globalLogHookMu.RLock()
	globalHook := globalLogHook
	globalLogHookMu.RUnlock()

	if globalHook != nil {
		globalHook(system, entry)
	}
}

func (system *System) Info(msg string, args ...any) {
	formatted := msg

	if len(args) > 0 {
		formatted = fmt.Sprintf(msg, args...)
	}

	errnie.Info(fmt.Sprintf("[%s] %s", system.name, formatted))

	entry := LogEntry{
		Timestamp: time.Now().UnixMilli(),
		Level:     LogInfo,
		Message:   formatted,
		System:    system.name,
	}

	system.appendLog(entry)
}

func (system *System) Warning(msg string, args ...any) {
	formatted := msg

	if len(args) > 0 {
		formatted = fmt.Sprintf(msg, args...)
	}

	errnie.Warn(fmt.Sprintf("[%s] %s", system.name, formatted))

	entry := LogEntry{
		Timestamp: time.Now().UnixMilli(),
		Level:     LogWarning,
		Message:   formatted,
		System:    system.name,
	}

	system.appendLog(entry)
}

func (system *System) Transition(stage Stage) {
	old := system.status.Current()
	system.status.Transition(stage)

	if system.status.Current() == stage && old != stage {
		msg := fmt.Sprintf("%s: %s -> %s", system.name, old, stage)
		errnie.Info(msg)

		entry := LogEntry{
			Timestamp: time.Now().UnixMilli(),
			Level:     LogInfo,
			Message:   msg,
			System:    system.name,
		}

		system.appendLog(entry)
	}
}

func (system *System) Status() Stage { return system.status.Current() }

func (system *System) Error(errs ...error) error {
	for _, added := range errs {
		if added == nil {
			continue
		}

		system.err = errors.Join(system.err, errnie.Error(added))

		entry := LogEntry{
			Timestamp: time.Now().UnixMilli(),
			Level:     LogError,
			Message:   added.Error(),
			System:    system.name,
		}

		system.appendLog(entry)

		switch system.status.Current() {
		case DONE, FATAL:
			return system.Close()
		default:
			system.Transition(ERROR)
		}

		categorized, ok := errnie.AsErrnie(added)

		if ok && errnie.IsInternal(categorized) {
			return system.Close()
		}
	}

	return system.err
}

func (system *System) Logs() []LogEntry {
	system.logMu.RLock()
	defer system.logMu.RUnlock()

	copied := make([]LogEntry, len(system.logs))
	copy(copied, system.logs)

	return copied
}

func (system *System) ClearLogs() {
	system.logMu.Lock()
	defer system.logMu.Unlock()

	system.logs = nil
}

func (system *System) OnLog(fn func(LogEntry)) {
	system.logMu.Lock()
	defer system.logMu.Unlock()

	system.onLog = fn
}

func (system *System) AddCloser(closer io.Closer) {
	system.closers = append(system.closers, closer)
}

func (system *System) System() *System { return system }

func (system *System) Close() error {
	if system.cancel != nil {
		system.cancel()
	}

	closers := system.closers
	system.closers = nil

	for _, closer := range closers {
		if closer == nil {
			continue
		}

		if sysGetter, ok := closer.(interface{ System() *System }); ok && sysGetter.System() == system {
			continue
		}

		system.Error(closer.Close())
	}

	return system.Error()
}
