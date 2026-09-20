package network

import "fmt"

// NetworkError is the unified error type for the network package,
// consolidating the former NetworkError and TransportError.
type NetworkError struct {
	Subsystem string               // "transport", "conn", etc.
	Op        string               // operation that failed
	Err       error                // underlying cause
	Msg       string               // human-readable summary
	Mode      TransportFailureMode // failure classification (optional)
	Systemic  bool                 // true when the failure is systemic
}

// NewNetworkError builds a NetworkError.
func NewNetworkError(subsystem string, err error, op string) *NetworkError {
	msg := subsystem

	if op != "" {
		msg += ": " + op
	}

	if err != nil {
		msg += ": " + err.Error()
	}

	return &NetworkError{
		Subsystem: subsystem,
		Op:        op,
		Err:       err,
		Msg:       msg,
	}
}

func (e *NetworkError) Error() string {
	if e == nil {
		return ""
	}

	if e.Mode != "" && e.Mode != TransportFailureNone {
		if e.Err == nil {
			if e.Op != "" {
				return fmt.Sprintf("[%s] %s (mode=%s)", e.Subsystem, e.Op, e.Mode)
			}

			return fmt.Sprintf("[%s] (mode=%s)", e.Subsystem, e.Mode)
		}

		if e.Op != "" {
			return fmt.Sprintf("[%s] %s (mode=%s): %v", e.Subsystem, e.Op, e.Mode, e.Err)
		}

		return fmt.Sprintf("[%s] (mode=%s): %v", e.Subsystem, e.Mode, e.Err)
	}

	if e.Op != "" {
		if e.Err == nil {
			return fmt.Sprintf("[%s] %s", e.Subsystem, e.Op)
		}

		return fmt.Sprintf("[%s] %s: %v", e.Subsystem, e.Op, e.Err)
	}

	if e.Msg != "" {
		return fmt.Sprintf("[%s] %s", e.Subsystem, e.Msg)
	}

	if e.Err == nil {
		return fmt.Sprintf("[%s]", e.Subsystem)
	}

	return fmt.Sprintf("[%s]: %v", e.Subsystem, e.Err)
}

func (e *NetworkError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// TransportStatusError is returned by monitor and breaker state transitions.
type TransportStatusError string

const (
	ErrTransportCircuitOpen TransportStatusError = "transport: circuit open"
)

// Error implements the error interface for TransportStatusError.
func (transportErr TransportStatusError) Error() string {
	return string(transportErr)
}
