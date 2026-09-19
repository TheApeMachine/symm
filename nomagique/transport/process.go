package transport

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Process represents a generic external process execution atom.
It accepts dynamic command-line arguments and returns the raw stdout bytes.
No structs, pure Value closure.
*/
type Process types.Value[[]string, []byte]

/*
NewProcess constructs a Process closure that executes binary with defaultArgs and input args.
*/
func NewProcess(binary string, defaultArgs ...string) Process {
	var mu sync.Mutex

	return func(args []string) []byte {
		mu.Lock()
		defer mu.Unlock()

		fullArgs := make([]string, 0, len(defaultArgs)+len(args))
		fullArgs = append(fullArgs, defaultArgs...)
		fullArgs = append(fullArgs, args...)

		cmd := exec.Command(binary, fullArgs...)
		if errors.Is(cmd.Err, exec.ErrDot) {
			cmd.Err = nil
		}

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		stdout, err := cmd.Output()
		if err != nil {
			details := strings.TrimSpace(stderr.String())
			if details == "" {
				details = strings.TrimSpace(string(stdout))
			}

			if details == "" {
				details = err.Error()
			}

			errnie.Error(errnie.Err(
				errnie.IO,
				"process: command execution failed: "+details,
				err,
			))
			return nil
		}

		return stdout
	}
}

/*
NewProcessWithContext creates a Process closure bound to a cancellation context.
*/
func NewProcessWithContext(ctx context.Context, binary string, defaultArgs ...string) Process {
	var mu sync.Mutex

	return func(args []string) []byte {
		mu.Lock()
		defer mu.Unlock()

		select {
		case <-ctx.Done():
			errnie.Error(errnie.Err(
				errnie.Timeout,
				"process: context cancelled before execution",
				ctx.Err(),
			))
			return nil
		default:
		}

		fullArgs := make([]string, 0, len(defaultArgs)+len(args))
		fullArgs = append(fullArgs, defaultArgs...)
		fullArgs = append(fullArgs, args...)

		cmd := exec.CommandContext(ctx, binary, fullArgs...)
		if errors.Is(cmd.Err, exec.ErrDot) {
			cmd.Err = nil
		}

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		stdout, err := cmd.Output()
		if err != nil {
			details := strings.TrimSpace(stderr.String())
			if details == "" {
				details = strings.TrimSpace(string(stdout))
			}

			if details == "" {
				details = err.Error()
			}

			errnie.Error(errnie.Err(
				errnie.IO,
				"process: command execution failed: "+details,
				err,
			))
			return nil
		}

		return stdout
	}
}
