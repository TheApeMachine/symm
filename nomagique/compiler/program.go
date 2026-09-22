package compiler

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type NodeID uint32
type FieldID uint16

/*
CompiledMethod stores method dispatch coordinates.
*/
type CompiledMethod struct {
	InterfaceID uint64
	MethodID    uint16
	ParamsSize  capnp.ObjectSize
	ResultSize  capnp.ObjectSize
}

/*
CompiledField describes a port field on write or done.
*/
type CompiledField struct {
	SchemaField        schema.Field
	Name               string
	Which              schema.Type_Which
	Offset             uint32
	Index              FieldID
	InUnion            bool
	DiscriminantValue  uint16
	DiscriminantOffset uint32
}

/*
NodeIdentity provides stable construction identity for capability reuse across recompiles.
*/
type NodeIdentity struct {
	ID           string
	Type         string
	InterfaceID  uint64
	ConfigDigest [32]byte
}

/*
CompiledNode is an immutable execution plan node containing capability and compiled schema only.
It retains NO concrete Go server, NO server any, NO map[string]any.
*/
type CompiledNode struct {
	Resource     bool // Constructed capability without the write/done evaluation protocol.
	Source       bool
	ID           string
	Index        NodeID
	Client       capnp.Client
	Write        CompiledMethod
	Done         CompiledMethod
	Inputs       map[string]CompiledField
	Outputs      map[string]CompiledField
	InputIndices map[string]FieldID
	RequiredMask uint64
	ArgsTemplate capnp.Struct
	Identity     NodeIdentity
}

/*
Route connects a source node result field to a destination node argument field.
*/
type Route struct {
	FromNode       NodeID
	FromField      FieldID
	ToNode         NodeID
	ToField        FieldID
	Copy           Copier
	FromInUnion    bool
	FromDiscVal    uint16
	FromDiscOffset uint32
	// Delivered reports whether the producer actually carried a value for this
	// route. A producer that reports several values may carry only some of
	// them, and a consumer waiting on one that did not arrive must keep
	// waiting rather than run on whatever its arguments held.
	Delivered func(src capnp.Struct) bool
}

/*
UIPlan holds the lowered structural UI route trees extracted from a compiled graph.
*/
type UIPlan struct {
	Routes []UIRoutePlan `json:"routes"`
}

/*
UIRoutePlan describes a route and its root component tree.
*/
type UIRoutePlan struct {
	Path       string       `json:"path"`
	Title      string       `json:"title,omitempty"`
	Components []UINodePlan `json:"components"`
}

/*
UINodePlan describes one lowered component node in a UI tree.
*/
type UINodePlan struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	ClassName string         `json:"className,omitempty"`
	Props     map[string]any `json:"props,omitempty"`
	Children  []UINodePlan   `json:"children,omitempty"`
}

/*
BindingPlan holds cross-domain observable data bindings connecting backend outputs
to frontend UI component inputs.
*/
type BindingPlan struct {
	Bindings []BindingEntry `json:"bindings"`
}

/*
BindingEntry represents one cross-domain edge from a backend output to a UI input.
*/
type BindingEntry struct {
	SourceNode string `json:"sourceNode"`
	SourcePort string `json:"sourcePort"`
	TargetNode string `json:"targetNode"`
	TargetProp string `json:"targetProp"`
}

/*
Program is an immutable compiled executable Cap'n Proto execution plan.
*/
type Program struct {
	Version  string
	Nodes    []CompiledNode
	Routes   []Route
	Roots    []NodeID
	NodeMap  map[string]NodeID
	UI       *UIPlan
	Bindings *BindingPlan
	results  map[string]capnp.Struct
	mu       sync.Mutex // ensures one graph evaluation at a time per program
}

/*
Release releases Cap'n Proto capability references held by this program.
*/
func (p *Program) Release() {
	for i := range p.Nodes {
		if p.Nodes[i].Client.IsValid() {
			p.Nodes[i].Client.Release()
		}
	}
}

/* Flush fences evaluation and persists durable owners before Release. */
func (p *Program) Flush(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var failures []error

	for _, node := range p.Nodes {
		if !Implements(node.Identity.InterfaceID, runtime.Durable_TypeID) {
			continue
		}

		future, release := runtime.Durable(node.Client).Flush(ctx, nil)
		_, err := future.Struct()
		release()

		if err != nil {
			failures = append(failures, errnie.Error(errnie.Err(errnie.IO, "compiler: flush node "+node.ID, err)))
		}
	}

	return errors.Join(failures...)
}

/*
Start evaluates the compiled program continuously until the context is
cancelled. Each pass is one observation through the graph: sources publish
what they have, every node downstream of them steps once, and the pass ends.

An evaluation that fails is reported and the run continues, because a single
bad observation must not take the graph down; a cancelled context ends the
run cleanly.
*/
func (p *Program) Start(ctx context.Context) {
	var idle time.Duration

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := p.Execute(ctx, nil); err != nil {
			if ctx.Err() != nil {
				return
			}

			errnie.Error(errnie.Err(
				errnie.Internal,
				"compiler: graph evaluation failed",
				err,
			))
		}

		// A node that owns external I/O reports what it has without blocking,
		// so an evaluation over idle sources returns immediately. Back off
		// when a pass carried no payload, so a graph waiting on its venues
		// releases the processor instead of spinning on empty reads.
		//
		// The backoff paces the scheduler, never market time: it bounds how
		// long an arrived observation waits to be noticed, and every horizon,
		// window and baseline a graph measures still comes from the
		// observations themselves.
		if p.carriedPayload() {
			idle = 0
			continue
		}

		idle = nextBackoff(idle)

		select {
		case <-ctx.Done():
			return
		case <-time.After(idle):
		}
	}
}

/*
carriedPayload reports whether the last evaluation moved any bytes out of a
node that owns an external source.
*/
func (p *Program) carriedPayload() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	for index := range p.Nodes {
		node := &p.Nodes[index]

		if !node.Source || !node.Client.IsValid() {
			continue
		}

		result, found := p.results[node.ID]

		if !found || !result.IsValid() {
			continue
		}

		for _, field := range node.Outputs {
			if field.Which != schema.Type_Which_data {
				continue
			}

			pointer, err := result.Ptr(uint16(field.Offset))

			if err == nil && pointer.IsValid() && len(pointer.Data()) > 0 {
				return true
			}
		}
	}

	return false
}

/*
nextBackoff doubles an idle wait up to a ceiling, so a quiet graph settles
into infrequent polling while a graph that just went quiet reacts promptly.
*/
func nextBackoff(current time.Duration) time.Duration {
	const (
		minimum = time.Millisecond
		maximum = 64 * time.Millisecond
	)

	if current <= 0 {
		return minimum
	}

	doubled := current * 2

	if doubled > maximum {
		return maximum
	}

	return doubled
}

/*
Result returns the cloned output struct of a node from the latest evaluation.
*/
func (p *Program) Result(nodeID string) (capnp.Struct, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.results == nil {
		return capnp.Struct{}, false
	}
	res, ok := p.results[nodeID]
	return res, ok
}

/*
Float64Result retrieves a float64 output value from the latest evaluation.
*/
func (p *Program) Float64Result(nodeID, fieldName string) (float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.results == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"program: no results available",
			nil,
		))
	}

	res, ok := p.results[nodeID]

	if !ok {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("program: node %q result not found", nodeID),
			nil,
		))
	}

	nodeIdx, ok := p.NodeMap[nodeID]

	if !ok {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("program: node %q not in node map", nodeID),
			nil,
		))
	}

	node := p.Nodes[nodeIdx]
	field, ok := node.Outputs[fieldName]

	if !ok {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("program: output field %q not found on node %q", fieldName, nodeID),
			nil,
		))
	}

	if field.InUnion && res.Uint16(capnp.DataOffset(field.DiscriminantOffset*2)) != field.DiscriminantValue {
		return 0, errnie.Error(errnie.Err(errnie.Validation, "program: output "+fieldName+" is absent", nil))
	}

	return math.Float64frombits(res.Uint64(capnp.DataOffset(field.Offset * 8))), nil
}

/*
arguments returns the argument struct a node is being given this observation,
allocating it the first time something is written into it.
*/
func (p *Program) arguments(frames []nodeFrame, index NodeID) (capnp.Struct, error) {
	if frames[index].args.IsValid() {
		return frames[index].args, nil
	}

	node := &p.Nodes[index]

	_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return capnp.Struct{}, errnie.Error(errnie.Err(
			errnie.Internal, "compiler: failed to allocate evaluation message", err,
		))
	}

	arguments, err := capnp.NewRootStruct(segment, node.Write.ParamsSize)

	if err != nil {
		return capnp.Struct{}, errnie.Error(errnie.Err(
			errnie.Internal, "compiler: failed to allocate argument struct", err,
		))
	}

	if node.ArgsTemplate.IsValid() {
		if err := arguments.CopyFrom(node.ArgsTemplate); err != nil {
			return capnp.Struct{}, errnie.Error(errnie.Err(
				errnie.Internal, "compiler: copy argument template", err,
			))
		}
	}

	frames[index].args = arguments
	return arguments, nil
}

type nodeFrame struct {
	args     capnp.Struct
	ready    uint64
	executed bool
}

/*
Execute runs one evaluation observation through the immutable program.
Overlapping evaluations must not race through the same capability, so execution
acquires the evaluation lock for the duration of the frame.
*/
func (p *Program) Execute(
	ctx context.Context,
	initialInputs map[NodeID]capnp.Struct,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	nodeCount := len(p.Nodes)
	frames := make([]nodeFrame, nodeCount)
	p.results = make(map[string]capnp.Struct, nodeCount)

	// Build outgoing routes index
	outgoing := make([][]Route, nodeCount)
	for _, r := range p.Routes {
		if int(r.FromNode) < nodeCount {
			outgoing[r.FromNode] = append(outgoing[r.FromNode], r)
		}
	}

	// 1. Argument structs are allocated when a node is about to be given
	// something, not for every node on every observation. A graph is mostly
	// idle at any instant — one venue reports while the rest are quiet — so
	// allocating the whole graph's arguments per pass costs in proportion to
	// the graph instead of to the traffic.

	// 2. Populate initial inputs if supplied
	for nodeIdx, initStruct := range initialInputs {
		if int(nodeIdx) < nodeCount && initStruct.IsValid() {
			frames[nodeIdx].args = initStruct
			frames[nodeIdx].ready = p.Nodes[nodeIdx].RequiredMask
		}
	}

	// 3. Find initial runnable nodes.
	//
	// A node whose inputs are all wired waits for them: it runs when an
	// upstream delivers, never on whatever its arguments held from a previous
	// observation. A node with nothing wired into it is an origin — it owns a
	// clock, a socket, a process — and runs every pass because only it knows
	// whether it has something to report. Work is therefore proportional to
	// what arrived rather than to the size of the graph.
	var queue []NodeID
	for i := 0; i < nodeCount; i++ {
		node := &p.Nodes[i]

		if node.Resource {
			continue
		}

		if _, seeded := initialInputs[NodeID(i)]; seeded {
			queue = append(queue, NodeID(i))
			continue
		}

		if node.RequiredMask == 0 {
			queue = append(queue, NodeID(i))
		}
	}

	// 4. Execute DAG topologically
	for len(queue) > 0 {
		currIdx := queue[0]
		queue = queue[1:]

		if frames[currIdx].executed {
			continue
		}

		node := &p.Nodes[currIdx]
		client := node.Client

		var resStruct capnp.Struct

		if client.IsValid() {
			if _, err := p.arguments(frames, currIdx); err != nil {
				return err
			}

			// Step A: SendStreamCall(write)
			if frames[currIdx].args.IsValid() {
				send := capnp.Send{
					Method: capnp.Method{
						InterfaceID: node.Write.InterfaceID,
						MethodID:    node.Write.MethodID,
					},
					ArgsSize: node.Write.ParamsSize,
					PlaceArgs: func(s capnp.Struct) error {
						return s.CopyFrom(frames[currIdx].args)
					},
				}
				if err := client.SendStreamCall(ctx, send); err != nil {
					return errnie.Error(errnie.Err(
						errnie.IO,
						fmt.Sprintf("compiler: write call failed on node %q", node.ID),
						err,
					))
				}
			}

			// Step B: WaitStreaming() evaluation fence
			if err := client.WaitStreaming(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("compiler: streaming fence failed on node %q", node.ID),
					err,
				))
			}

			// Step C: SendCall(done)
			doneSend := capnp.Send{
				Method: capnp.Method{
					InterfaceID: node.Done.InterfaceID,
					MethodID:    node.Done.MethodID,
				},
				ArgsSize: node.Done.ParamsSize,
			}
			ans, release := client.SendCall(ctx, doneSend)
			res, err := ans.Struct()

			if err != nil {
				release()
				return errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("compiler: done call failed on node %q", node.ID),
					err,
				))
			}

			if res.IsValid() {
				resStruct = res

				// Retain a result independently of the RPC answer's lifetime.
				_, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

				if err != nil {
					release()
					return errnie.Error(errnie.Err(errnie.Internal, "compiler: allocate result message", err))
				}

				cloned, err := capnp.NewRootStruct(segment, node.Done.ResultSize)

				if err != nil {
					release()
					return errnie.Error(errnie.Err(errnie.Internal, "compiler: allocate result struct", err))
				}

				if err := cloned.CopyFrom(res); err != nil {
					release()
					return errnie.Error(errnie.Err(errnie.Internal, "compiler: clone result", err))
				}

				p.results[node.ID] = cloned
			}

			// Step D: Route results to downstreams
			for _, r := range outgoing[currIdx] {
				destIdx := r.ToNode
				if int(destIdx) < nodeCount {
					if r.FromInUnion {
						if !resStruct.IsValid() {
							continue
						}
						disc := resStruct.Uint16(capnp.DataOffset(r.FromDiscOffset * 2))
						if disc != r.FromDiscVal {
							// Inactive union branch: skip routing to this downstream
							continue
						}
					}

					if r.Copy != nil {
						destArgs, err := p.arguments(frames, destIdx)

						if err != nil {
							release()
							return err
						}

						if !resStruct.IsValid() {
							release()
							return errnie.Error(errnie.Err(
								errnie.Internal,
								fmt.Sprintf("compiler: field copy failed from %q to %q (invalid struct)", node.ID, p.Nodes[destIdx].ID),
								nil,
							))
						}
						// Nothing arrived for this route, so the consumer is
						// still waiting rather than ready with a default.
						if r.Delivered != nil && !r.Delivered(resStruct) {
							continue
						}

						if err := r.Copy(resStruct, destArgs); err != nil {
							release()
							return errnie.Error(errnie.Err(
								errnie.Internal,
								fmt.Sprintf("compiler: field copy failed from %q to %q", node.ID, p.Nodes[destIdx].ID),
								err,
							))
						}
					}
					frames[destIdx].ready |= (1 << r.ToField)
					destReq := p.Nodes[destIdx].RequiredMask
					if (frames[destIdx].ready&destReq) == destReq && !frames[destIdx].executed {
						queue = append(queue, destIdx)
					}
				}
			}

			release()
		} else {
			// Virtual boundary node (e.g. data source injected from outside or boundary sink)
			p.results[node.ID] = frames[currIdx].args
			for _, r := range outgoing[currIdx] {
				destIdx := r.ToNode
				if int(destIdx) < nodeCount {
					if r.FromInUnion {
						if !frames[currIdx].args.IsValid() {
							continue
						}
						disc := frames[currIdx].args.Uint16(capnp.DataOffset(r.FromDiscOffset * 2))
						if disc != r.FromDiscVal {
							continue
						}
					}

					destArgs, err := p.arguments(frames, destIdx)

					if err != nil {
						return err
					}

					if r.Copy != nil && frames[currIdx].args.IsValid() && destArgs.IsValid() {
						if err := r.Copy(frames[currIdx].args, destArgs); err != nil {
							return errnie.Error(errnie.Err(
								errnie.Internal, "compiler: copy boundary result", err,
							))
						}
					}
					frames[destIdx].ready |= (1 << r.ToField)
					destReq := p.Nodes[destIdx].RequiredMask
					if (frames[destIdx].ready&destReq) == destReq && !frames[destIdx].executed {
						queue = append(queue, destIdx)
					}
				}
			}
		}

		frames[currIdx].executed = true
	}

	return nil
}
