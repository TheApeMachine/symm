package transport

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
DisruptorServer publishes observations into an LMAX ring and reports each
sequence to the stages mounted behind its barriers.

A stage is wired by connecting a node to one of the stage ports. Everything
wired to one port is one handler group and runs concurrently against the same
sequence; nothing on a later port observes a sequence until every handler on
the earlier port has passed it. Those are the ring's own barriers, so a graph
that needs two things ordered wires them to consecutive stages, and a graph
that needs them concurrent wires them to the same stage. Where a graph needs
a hazard serialized it wires a stage that serializes, rather than the engine
holding a lock on its behalf.

The ring begins WAITING and publishes nothing until admit arrives, so a
subscription universe still being constructed cannot become input.
*/
type DisruptorServer struct {
	*runtime.System
	capacity int64
	writers  int64
	ring     *runtime.Workspace
	stages   [4]*relay
}

func NewDisruptor(ctx context.Context) *DisruptorServer {
	server := &DisruptorServer{
		System:   runtime.NewSystem(ctx, "transport.disruptor"),
		capacity: 1024,
		writers:  1,
	}

	for index := range server.stages {
		server.stages[index] = &relay{}
	}

	server.Transition(runtime.WAITING)
	return server
}

/*
Write admits one observation into the ring, building the ring on the first
observation so its capacity and writer count come from the graph.
*/
func (server *DisruptorServer) Write(ctx context.Context, call Disruptor_write) error {
	if capacity := call.Args().Capacity(); capacity > 0 {
		server.capacity = capacity
	}

	if writers := call.Args().Writers(); writers > 0 {
		server.writers = writers
	}

	if server.ring == nil {
		if err := server.build(); err != nil {
			return err
		}
	}

	if call.Args().Admit() && server.Status() != runtime.READY {
		server.ring.Admit()
		server.Transition(runtime.READY)
		server.Info("ring admitted")
	}

	payload, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[transport.disruptor.Write] failed to read data argument",
			err,
		))
	}

	if len(payload) == 0 || server.Status() != runtime.READY {
		return nil
	}

	return server.ring.Push(append([]byte(nil), payload...))
}

/*
Done reports what each stage observed of the last sequence it passed, so the
nodes wired to a stage port receive the observation that stage handled.
*/
func (server *DisruptorServer) Done(ctx context.Context, call Disruptor_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[transport.disruptor.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))

	if server.ring != nil {
		results.SetBacklog(server.ring.Backlog())
	}

	setters := [4]func([]byte) error{
		results.SetStage1,
		results.SetStage2,
		results.SetStage3,
		results.SetStage4,
	}

	for index, stage := range server.stages {
		observed := stage.take()

		if len(observed) == 0 {
			continue
		}

		if err := setters[index](observed); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[transport.disruptor.Done] failed to set stage output",
				err,
			))
		}
	}

	return nil
}

/*
build constructs the ring with one handler group per stage port.
*/
func (server *DisruptorServer) build() error {
	stages := make([][]runtime.Step, 0, len(server.stages))

	for _, stage := range server.stages {
		stages = append(stages, []runtime.Step{stage})
	}

	ring, err := runtime.NewWorkspace(
		server.Context(),
		"transport.disruptor",
		uint32(server.capacity),
		uint8(server.writers),
		stages,
	)

	if err != nil {
		return err
	}

	server.ring = ring
	server.AddCloser(ring)

	return nil
}

/*
relay is one stage's handler. It records the observation the stage passed, so
Done can report it to whatever the graph wired to that stage port.

A handler runs on the ring's own goroutine while Done is called by whoever
steps the node, so the recorded observation crosses a goroutine boundary and
is guarded. The guard covers a slice swap, never the stage's work.
*/
type relay struct {
	mutex    sync.Mutex
	observed []byte
}

func (stage *relay) Step(ctx context.Context, payload []byte) ([]byte, error) {
	observed := append([]byte(nil), payload...)

	stage.mutex.Lock()
	stage.observed = observed
	stage.mutex.Unlock()

	return payload, nil
}

func (stage *relay) take() []byte {
	stage.mutex.Lock()
	defer stage.mutex.Unlock()

	observed := stage.observed
	stage.observed = nil

	return observed
}
