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

type evaluationState struct {
	setters  map[string]func(capnp.Struct)
	received map[string]bool
}

/*
InvocationAssembler manages the typed activation and argument assembly
for one compiled Cap'n Proto node. It guarantees that inputs from different
evaluations are never combined, incomplete evaluations never fire, and
the generated client Write call is made once all required ports are present.
*/
type InvocationAssembler struct {
	mu           sync.Mutex
	nodeID       string
	client       capnp.Client
	inputPorts   map[string]PortType
	staticInputs map[string]func(capnp.Struct)
	createSetter map[string]func(val any) func(capnp.Struct)
	invokeFn     func(ctx context.Context, setters map[string]func(capnp.Struct)) error
	doneFn       func(ctx context.Context) error
	evaluations  map[uint64]*evaluationState
	inputSinks   map[string]capnp.Client
}

/*
NewInvocationAssembler creates an InvocationAssembler for a node.
*/
func NewInvocationAssembler(
	nodeID string,
	client capnp.Client,
	inputPorts map[string]PortType,
	createSetter map[string]func(val any) func(capnp.Struct),
	invokeFn func(ctx context.Context, setters map[string]func(capnp.Struct)) error,
	doneFn func(ctx context.Context) error,
) *InvocationAssembler {
	return &InvocationAssembler{
		nodeID:       nodeID,
		client:       client,
		inputPorts:   inputPorts,
		staticInputs: make(map[string]func(capnp.Struct)),
		createSetter: createSetter,
		invokeFn:     invokeFn,
		doneFn:       doneFn,
		evaluations:  make(map[uint64]*evaluationState),
		inputSinks:   make(map[string]capnp.Client),
	}
}

/*
SetStaticInput configures an unwired port with a constant setter from node inputData/config.
*/
func (a *InvocationAssembler) SetStaticInput(portName string, setter func(capnp.Struct)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.staticInputs[portName] = setter
}

/*
InputSink returns the typed Cap'n Proto sink capability for a given input port.
*/
func (a *InvocationAssembler) InputSink(portName string) (capnp.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if sink, exists := a.inputSinks[portName]; exists {
		return sink, nil
	}

	portType, ok := a.inputPorts[portName]
	if !ok {
		return capnp.Client{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("assembler: node %q has no input port %q", a.nodeID, portName),
			nil,
		))
	}

	switch portType {
	case PortTypeFloat64:
		sink := types.NewFloat64Sink(
			func(ctx context.Context, val float64) error {
				return a.recordInput(ctx, portName, val)
			},
			func(ctx context.Context) error {
				return a.recordDone(ctx)
			},
		)
		client := capnp.Client(sink)
		a.inputSinks[portName] = client
		return client, nil

	default:
		return capnp.Client{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("assembler: unsupported port type %s for port %s on node %s", portType, portName, a.nodeID),
			nil,
		))
	}
}

func (a *InvocationAssembler) recordInput(ctx context.Context, portName string, val any) error {
	evalID, ok := types.EvaluationIDFromContext(ctx)
	if !ok {
		ctx, evalID = types.NextEvaluationContext(ctx)
	}

	var (
		readySetters map[string]func(capnp.Struct)
		shouldInvoke bool
	)

	a.mu.Lock()
	state, exists := a.evaluations[evalID]
	if !exists {
		state = &evaluationState{
			setters:  make(map[string]func(capnp.Struct)),
			received: make(map[string]bool),
		}
		// Populate any static inputs configured on unwired controls
		for p, setter := range a.staticInputs {
			state.setters[p] = setter
			state.received[p] = true
		}
		a.evaluations[evalID] = state
	}

	if setterGen, ok := a.createSetter[portName]; ok {
		state.setters[portName] = setterGen(val)
		state.received[portName] = true
	}

	// Check if all declared input ports are satisfied
	allSatisfied := true
	for p := range a.inputPorts {
		if !state.received[p] {
			allSatisfied = false
			break
		}
	}

	if allSatisfied {
		shouldInvoke = true
		readySetters = state.setters
		delete(a.evaluations, evalID)
	}
	a.mu.Unlock()

	if shouldInvoke && a.invokeFn != nil {
		return a.invokeFn(ctx, readySetters)
	}

	return nil
}

func (a *InvocationAssembler) recordDone(ctx context.Context) error {
	if a.doneFn != nil {
		return a.doneFn(ctx)
	}
	return nil
}

/*
WaitStreaming waits for pending streaming calls on the underlying client.
*/
func (a *InvocationAssembler) WaitStreaming() error {
	a.mu.Lock()
	sinks := make([]capnp.Client, 0, len(a.inputSinks))
	for _, s := range a.inputSinks {
		sinks = append(sinks, s)
	}
	a.mu.Unlock()

	for _, s := range sinks {
		if err := s.WaitStreaming(); err != nil {
			return err
		}
	}
	return a.client.WaitStreaming()
}
