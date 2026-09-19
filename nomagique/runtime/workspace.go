package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/compiler"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Workspace is the runtime execution boundary. It loads a composed signal graph
from JSON (via the compiler Builder) and pipes raw market ticks into it.

This completely replaces the old LMAX Disruptor queues, guaranteeing
topological order execution synchronously with zero allocations.
*/
type Workspace struct {
	*System
	pipeline types.Value[any, any]
	sink     chan any // The output of the pipeline
}

/*
NewWorkspace initializes the runtime boundary by dynamically compiling
the target JSON signal definition.
*/
func NewWorkspace(ctx context.Context, label string, jsonPath string) *Workspace {
	workload := &Workspace{
		sink: make(chan any, 1024),
	}

	// If a logical name was provided, translate it to the definitions folder
	if !strings.HasSuffix(jsonPath, ".json") {
		filename := strings.ReplaceAll(jsonPath, ":", "_") + ".json"
		_, goFile, _, _ := goruntime.Caller(0)
		root := filepath.Join(filepath.Dir(goFile), "..", "..")
		jsonPath = filepath.Join(root, "signal", "definitions", filename)
	}

	builder, err := compiler.NewBuilder(jsonPath)
	if err != nil {
		workload.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("[workspace] failed to load signal definition %s", jsonPath),
			err,
		))
		return nil
	}

	pipeline, err := builder.Compose()
	if err != nil {
		workload.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("[workspace] failed to dynamically wire signal graph %s", jsonPath),
			err,
		))
		return nil
	}

	workload.pipeline = pipeline
	workload.System = NewSystem(ctx, label, workload)
	return workload
}

/*
Next admits a market tick into the dynamic signal graph.
The JSON graph processes the tick synchronously.
*/
func (workspace *Workspace) Next(tick any) {
	if workspace.pipeline == nil {
		return
	}

	// 1. Hot Path: Execute the topological JSON graph
	start := time.Now()
	result := workspace.pipeline(tick)
	elapsed := time.Since(start)

	// 2. Monitoring (Optional)
	if elapsed > 100*time.Millisecond {
		errnie.Warn(fmt.Sprintf("[workspace] slow pipeline execution: %v", elapsed))
	}

	// 3. Emit to sink
	if result != nil {
		select {
		case workspace.sink <- result:
		default:
			// Backpressure handling (drop or warn)
			errnie.Warn("[workspace] sink buffer full, dropping signal")
		}
	}
}

/*
Sink exposes the output channel for strategies to consume the final signal values.
*/
func (workspace *Workspace) Sink() <-chan any {
	return workspace.sink
}
