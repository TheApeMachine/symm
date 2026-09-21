package compiler

import (
	"context"
	"fmt"
	"math"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	"github.com/theapemachine/errnie"
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
	IsSource     bool
	IsSink       bool
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
}

/*
Program is an immutable compiled executable Cap'n Proto execution plan.
*/
type Program struct {
	Version string
	Nodes   []CompiledNode
	Routes  []Route
	Roots   []NodeID
	NodeMap map[string]NodeID
	results map[string]capnp.Struct
	mu      sync.Mutex // ensures one graph evaluation at a time per program
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

/*
Start begins running the compiled program until context is cancelled.
*/
func (p *Program) Start(ctx context.Context) {
	<-ctx.Done()
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
		return 0, fmt.Errorf("program: no results available")
	}
	res, ok := p.results[nodeID]
	if !ok {
		return 0, fmt.Errorf("program: node %q result not found", nodeID)
	}
	nodeIdx, ok := p.NodeMap[nodeID]
	if !ok {
		return 0, fmt.Errorf("program: node %q not in node map", nodeID)
	}
	node := p.Nodes[nodeIdx]
	field, ok := node.Outputs[fieldName]
	if !ok {
		field, ok = node.Outputs["out"]
		if !ok {
			field, ok = node.Inputs[fieldName]
			if !ok {
				field, ok = node.Inputs["in"]
				if !ok {
					field, ok = node.Inputs["value"]
					if !ok {
						return 0, fmt.Errorf("program: field %q not found on node %q", fieldName, nodeID)
					}
				}
			}
		}
	}
	return math.Float64frombits(res.Uint64(capnp.DataOffset(field.Offset * 8))), nil
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

	// 1. Initialize per-node argument structs
	for i := 0; i < nodeCount; i++ {
		node := &p.Nodes[i]
		if node.Client.IsValid() || node.Write.ParamsSize.DataSize > 0 || node.Write.ParamsSize.PointerCount > 0 {
			_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
			if err != nil {
				return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc evaluation message", err))
			}
			st, err := capnp.NewRootStruct(seg, node.Write.ParamsSize)
			if err != nil {
				return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc argument struct", err))
			}
			if node.ArgsTemplate.IsValid() {
				_ = st.CopyFrom(node.ArgsTemplate)
			}
			frames[i].args = st
		}
	}

	// 2. Populate initial inputs if supplied
	for nodeIdx, initStruct := range initialInputs {
		if int(nodeIdx) < nodeCount && initStruct.IsValid() {
			if frames[nodeIdx].args.IsValid() {
				_ = frames[nodeIdx].args.CopyFrom(initStruct)
			} else {
				frames[nodeIdx].args = initStruct
			}
			frames[nodeIdx].ready = p.Nodes[nodeIdx].RequiredMask
		}
	}

	// 3. Find initial runnable nodes
	var queue []NodeID
	for i := 0; i < nodeCount; i++ {
		node := &p.Nodes[i]
		if node.IsSource {
			if _, ok := initialInputs[NodeID(i)]; ok {
				queue = append(queue, NodeID(i))
			}
			continue
		}
		req := node.RequiredMask
		if (frames[i].ready & req) == req {
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
			if err == nil && res.IsValid() {
				resStruct = res

				// Save clone for result retrieval
				_, seg, cloneErr := capnp.NewMessage(capnp.SingleSegment(nil))
				if cloneErr == nil {
					cloneRes, initErr := capnp.NewRootStruct(seg, node.Done.ResultSize)
					if initErr == nil {
						_ = cloneRes.CopyFrom(res)
						p.results[node.ID] = cloneRes
					}
				}
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
						if !resStruct.IsValid() || !frames[destIdx].args.IsValid() {
							release()
							return errnie.Error(errnie.Err(
								errnie.Internal,
								fmt.Sprintf("compiler: field copy failed from %q to %q (invalid struct)", node.ID, p.Nodes[destIdx].ID),
								nil,
							))
						}
						if err := r.Copy(resStruct, frames[destIdx].args); err != nil {
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

					if r.Copy != nil && frames[currIdx].args.IsValid() && frames[destIdx].args.IsValid() {
						_ = r.Copy(frames[currIdx].args, frames[destIdx].args)
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
