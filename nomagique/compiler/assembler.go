package compiler

import (
	"context"
	"fmt"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
PortType defines the wire type of a Cap'n Proto input or output port.
*/
type PortType string

const (
	PortTypeFloat64 PortType = "Float64"
	PortTypeInt64   PortType = "Int64"
	PortTypeText    PortType = "Text"
	PortTypeBool    PortType = "Bool"
	PortTypeData    PortType = "Data"
)

const maxPendingEvaluations = 1024

type evaluationState struct {
	setters  map[string]func(capnp.Struct)
	received map[string]bool
}

/*
InvocationAssembler manages the typed activation and argument assembly
for one compiled Cap'n Proto node. It guarantees that inputs from different
evaluations are never combined, incomplete evaluations never fire, and
the generated client Write call is made once all required ports are present.
Zero any values exist in the runtime execution path.
*/
type InvocationAssembler struct {
	mu              sync.Mutex
	nodeID          string
	client          capnp.Client
	inputPorts      map[string]PortType
	staticInputs    map[string]func(capnp.Struct)
	sinkFactories   map[string]func(assembler *InvocationAssembler) capnp.Client
	invokeFn        func(ctx context.Context, setters map[string]func(capnp.Struct)) error
	doneFn          func(ctx context.Context) error
	evaluations     map[uint64]*evaluationState
	inputSinks      map[string]capnp.Client
	wiredInputs     map[string]bool
	completedInputs map[string]bool
}

/*
NewInvocationAssembler creates an InvocationAssembler for a node.
*/
func NewInvocationAssembler(
	nodeID string,
	client capnp.Client,
	inputPorts map[string]PortType,
	sinkFactories map[string]func(assembler *InvocationAssembler) capnp.Client,
	invokeFn func(ctx context.Context, setters map[string]func(capnp.Struct)) error,
	doneFn func(ctx context.Context) error,
) *InvocationAssembler {
	return &InvocationAssembler{
		nodeID:          nodeID,
		client:          client,
		inputPorts:      inputPorts,
		staticInputs:    make(map[string]func(capnp.Struct)),
		sinkFactories:   sinkFactories,
		invokeFn:        invokeFn,
		doneFn:          doneFn,
		evaluations:     make(map[uint64]*evaluationState),
		inputSinks:      make(map[string]capnp.Client),
		wiredInputs:     make(map[string]bool),
		completedInputs: make(map[string]bool),
	}
}

/*
MarkWired records that an input port is wired from an upstream edge.
*/
func (assembler *InvocationAssembler) MarkWired(portName string) {
	assembler.mu.Lock()
	defer assembler.mu.Unlock()
	assembler.wiredInputs[portName] = true
}

/*
SetStaticInput configures an unwired port with a constant setter from node inputData/config.
*/
func (assembler *InvocationAssembler) SetStaticInput(portName string, setter func(capnp.Struct)) {
	assembler.mu.Lock()
	defer assembler.mu.Unlock()
	assembler.staticInputs[portName] = setter
}

/*
InputSink returns the typed Cap'n Proto sink capability for a given input port.
*/
func (assembler *InvocationAssembler) InputSink(portName string) (capnp.Client, error) {
	assembler.mu.Lock()
	defer assembler.mu.Unlock()

	if sink, exists := assembler.inputSinks[portName]; exists {
		return sink, nil
	}

	factory, ok := assembler.sinkFactories[portName]
	if !ok {
		return capnp.Client{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("assembler: node %q has no sink factory for port %q", assembler.nodeID, portName),
			nil,
		))
	}

	client := factory(assembler)
	assembler.inputSinks[portName] = client
	return client, nil
}

/*
Record accepts a pre-typed Cap'n Proto parameter setter for a specific evaluation.
Zero any payloads or assertions exist on this path.
*/
func (assembler *InvocationAssembler) Record(
	ctx context.Context,
	evalID uint64,
	portName string,
	setter func(capnp.Struct),
) error {
	if evalID == 0 {
		var ok bool
		evalID, ok = types.EvaluationIDFromContext(ctx)

		if !ok || evalID == 0 {
			ctx, evalID = types.NextEvaluationContext(ctx)
		}
	}

	var (
		readySetters map[string]func(capnp.Struct)
		shouldInvoke bool
	)

	assembler.mu.Lock()

	state, exists := assembler.evaluations[evalID]
	if !exists {
		if len(assembler.evaluations) >= maxPendingEvaluations {
			// Prune oldest incomplete evaluation to bound memory
			for staleEvalID := range assembler.evaluations {
				delete(assembler.evaluations, staleEvalID)
				break
			}
		}

		state = &evaluationState{
			setters:  make(map[string]func(capnp.Struct)),
			received: make(map[string]bool),
		}

		for inputPort, staticSetter := range assembler.staticInputs {
			state.setters[inputPort] = staticSetter
			state.received[inputPort] = true
		}

		assembler.evaluations[evalID] = state
	}

	state.setters[portName] = setter
	state.received[portName] = true

	allSatisfied := true
	if len(assembler.wiredInputs) > 0 {
		for port := range assembler.wiredInputs {
			if !state.received[port] {
				allSatisfied = false
				break
			}
		}
	}

	if len(assembler.wiredInputs) == 0 {
		for port := range assembler.inputPorts {
			if !state.received[port] {
				allSatisfied = false
				break
			}
		}
	}

	if allSatisfied {
		shouldInvoke = true
		readySetters = state.setters
		delete(assembler.evaluations, evalID)
	}

	assembler.mu.Unlock()

	if shouldInvoke && assembler.invokeFn != nil {
		ctx = types.WithEvaluationID(ctx, evalID)
		return assembler.invokeFn(ctx, readySetters)
	}

	return nil
}

/*
RecordDone records that an input port has completed streaming.
Completion propagates downstream only when all wired input ports have finished.
*/
func (assembler *InvocationAssembler) RecordDone(ctx context.Context, portName string) error {
	var shouldDone bool

	assembler.mu.Lock()
	assembler.completedInputs[portName] = true

	// Prune abandoned incomplete evaluations that were awaiting this port
	for evalID, state := range assembler.evaluations {
		if !state.received[portName] {
			delete(assembler.evaluations, evalID)
		}
	}

	// Check if all wired inputs have completed
	allDone := true
	if len(assembler.wiredInputs) > 0 {
		for port := range assembler.wiredInputs {
			if !assembler.completedInputs[port] {
				allDone = false
				break
			}
		}
	}

	if len(assembler.wiredInputs) == 0 {
		for port := range assembler.inputPorts {
			if !assembler.completedInputs[port] {
				allDone = false
				break
			}
		}
	}

	if allDone {
		shouldDone = true
		// Clear all remaining evaluations on completion
		assembler.evaluations = make(map[uint64]*evaluationState)
	}

	assembler.mu.Unlock()

	if shouldDone && assembler.doneFn != nil {
		return assembler.doneFn(ctx)
	}

	return nil
}

/*
WaitStreaming waits for pending streaming calls on the underlying client.
*/
func (assembler *InvocationAssembler) WaitStreaming() error {
	assembler.mu.Lock()
	sinks := make([]capnp.Client, 0, len(assembler.inputSinks))
	for _, sink := range assembler.inputSinks {
		sinks = append(sinks, sink)
	}
	assembler.mu.Unlock()

	for _, sink := range sinks {
		if err := sink.WaitStreaming(); err != nil {
			return err
		}
	}

	return assembler.client.WaitStreaming()
}
