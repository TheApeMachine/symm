package runtime

import (
	"context"

	"golang.design/x/lockfree/lf"
)

/*
Workspace is a generic runtime which takes care of scheduling workloads
handling observers, telemetry, etc. It runs as an ambient context.
*/
type Workspace struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	queue  *lf.Queue[Schedulable]
	pool   *Pool[func()]
}

func NewWorkspace() *Workspace {
	return &Workspace{}
}

func (workspace *Workspace) Register(worker Worker)

func (workspace *Workspace) Close() error {
	workspace.cancel()
	return nil
}
