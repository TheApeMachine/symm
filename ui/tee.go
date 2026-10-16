package ui

import (
	"context"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/lf"
)

/*
UITee is a concrete off-ramp that accepts *data.Value
and yields encoded binary frames for the dashboard websocket.
It satisfies runtime.Tee.
*/
type UITee struct {
	*nmruntime.System
	egress  *lf.Queue[*data.Value]
	filters []func(value *data.Value) bool
	route   string
	symbol  string
}

/*
NewUITee creates a new wait-free UITee off-ramp.
*/
func NewUITee(
	ctx context.Context,
	label string,
	filters ...func(value *data.Value) bool,
) *UITee {
	if len(filters) == 0 {
		filters = []func(value *data.Value) bool{types.Filters}
	}

	tee := &UITee{
		filters: filters,
		egress:  lf.NewQueue[*data.Value](),
		route:   types.Route(),
		symbol:  types.Focus(),
	}

	tee.System = nmruntime.NewSystem(ctx, label, tee)
	return tee
}

/*
Push receives values from the workspace.
*/
func (tee *UITee) Push(value *data.Value) {
	if tee.Status() != nmruntime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return
	}

	if value == nil {
		return
	}

	for _, filter := range tee.filters {
		if !filter(value) {
			return
		}
	}

	tee.egress.Enqueue(value)
}

/*
Next yields the next available serialized frame.
*/
func (tee *UITee) Next() unsafe.Pointer {
	if tee.Status() != nmruntime.READY {
		errnie.Warn("[tee] pulling from a non-ready system may have unintended consequences")
		return nil
	}

	value, ok := tee.egress.Dequeue()
	if !ok || value == nil {
		return nil
	}

	encoded := value.Bytes()
	return unsafe.Pointer(&encoded)
}

/*
Run is a noop for the UITee.
*/
func (tee *UITee) Run() {}

/*
Pending returns the total backlog of un-transmitted items across ingress and egress.
*/
func (tee *UITee) Pending() int64 {
	return int64(tee.egress.Length())
}
