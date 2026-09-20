package compiler

import (
	"context"
	"fmt"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/catalog"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
PrimitiveDescriptor defines the Cap'n Proto schema, ports, constructors,
and invokers for a graph primitive. Zero any values exist in the data plane.
*/
type PrimitiveDescriptor struct {
	Op                 string
	InputPorts         map[string]PortType
	OutputPorts        map[string]PortType
	Construct          func(node Node) (server any, client capnp.Client, err error)
	CreateInputSink    map[string]func(a *InvocationAssembler) capnp.Client
	CreateStaticSetter map[string]func(raw any) (func(capnp.Struct), error)
	BindDownstream     func(server any, port string, sinks []capnp.Client) error
	Invoke             func(ctx context.Context, client capnp.Client, setters map[string]func(capnp.Struct), downstreams map[string][]capnp.Client) error
	Done               func(ctx context.Context, client capnp.Client, downstreams map[string][]capnp.Client) error
}

/*
Registry stores the authoritative primitive descriptors for the compiler.
*/
type Registry struct {
	mu           sync.RWMutex
	primitives   map[string]PrimitiveDescriptor
	repo         DefinitionRepository
	dependencies map[string]any
}

func NewRegistry() *Registry {
	r := &Registry{
		primitives:   make(map[string]PrimitiveDescriptor),
		dependencies: make(map[string]any),
	}
	r.registerDefaultPrimitives()
	RegisterGeneratedPrimitives(r)
	return r
}

var (
	defaultRegistryOnce sync.Once
	defaultRegistryInst *Registry
)

func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistryInst = NewRegistry()
	})
	return defaultRegistryInst
}

func (r *Registry) Register(desc PrimitiveDescriptor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.primitives[desc.Op] = desc
}

func (r *Registry) Resolve(op string) (PrimitiveDescriptor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	desc, exists := r.primitives[op]
	if !exists {
		return PrimitiveDescriptor{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("compiler: unknown primitive type %q", op),
			nil,
		))
	}
	return desc, nil
}

func (r *Registry) SetRepository(repo DefinitionRepository) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.repo = repo
}

func (r *Registry) Repository() DefinitionRepository {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.repo
}

func (r *Registry) Inject(name string, dep any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dependencies == nil {
		r.dependencies = make(map[string]any)
	}
	r.dependencies[name] = dep
}

func (r *Registry) Dependency(name string) any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.dependencies == nil {
		return nil
	}
	return r.dependencies[name]
}

func ToFloat64Sink(sinks []capnp.Client) types.Float64Sink {
	if len(sinks) == 0 {
		return types.Float64Sink{}
	}
	if len(sinks) == 1 {
		return types.Float64Sink(sinks[0])
	}
	typedSinks := make([]types.Float64Sink, len(sinks))
	for i, s := range sinks {
		typedSinks[i] = types.Float64Sink(s)
	}
	return types.NewBroadcastFloat64Sink(typedSinks...)
}

func ToInt64Sink(sinks []capnp.Client) types.Int64Sink {
	if len(sinks) == 0 {
		return types.Int64Sink{}
	}
	if len(sinks) == 1 {
		return types.Int64Sink(sinks[0])
	}
	typedSinks := make([]types.Int64Sink, len(sinks))
	for i, s := range sinks {
		typedSinks[i] = types.Int64Sink(s)
	}
	return types.NewBroadcastInt64Sink(typedSinks...)
}

func ToTextSink(sinks []capnp.Client) types.TextSink {
	if len(sinks) == 0 {
		return types.TextSink{}
	}
	if len(sinks) == 1 {
		return types.TextSink(sinks[0])
	}
	typedSinks := make([]types.TextSink, len(sinks))
	for i, s := range sinks {
		typedSinks[i] = types.TextSink(s)
	}
	return types.NewBroadcastTextSink(typedSinks...)
}

func ToBoolSink(sinks []capnp.Client) types.BoolSink {
	if len(sinks) == 0 {
		return types.BoolSink{}
	}
	if len(sinks) == 1 {
		return types.BoolSink(sinks[0])
	}
	typedSinks := make([]types.BoolSink, len(sinks))
	for i, s := range sinks {
		typedSinks[i] = types.BoolSink(s)
	}
	return types.NewBroadcastBoolSink(typedSinks...)
}

func ToDataSink(sinks []capnp.Client) types.DataSink {
	if len(sinks) == 0 {
		return types.DataSink{}
	}
	if len(sinks) == 1 {
		return types.DataSink(sinks[0])
	}
	typedSinks := make([]types.DataSink, len(sinks))
	for i, s := range sinks {
		typedSinks[i] = types.DataSink(s)
	}
	return types.NewBroadcastDataSink(typedSinks...)
}

/*
PassThroughServer provides identity streaming for Source and Sink nodes.
*/
type PassThroughServer struct {
	Downstream types.Float64Sink
}

func (s *PassThroughServer) Write(ctx context.Context, call types.Float64Sink_write) error {
	eval := call.Args().Evaluation()
	val := call.Args().Value()
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
			p.SetEvaluation(eval)
			p.SetValue(val)
			return nil
		})
	}
	return nil
}

func (s *PassThroughServer) Done(ctx context.Context, call types.Float64Sink_done) error {
	if capnp.Client(s.Downstream).IsValid() {
		_, release := s.Downstream.Done(ctx, nil)
		release()
	}
	return nil
}

func (r *Registry) registerDefaultPrimitives() {
	// Source primitive (authoritative data.Source is the Kraken/WireMeasurement ingress)
	sourceDesc := PrimitiveDescriptor{
		Op:         "data.Source",
		InputPorts: map[string]PortType{},
		OutputPorts: map[string]PortType{
			"out":   PortTypeData,
			"value": PortTypeData,
		},
		Construct: func(node Node) (any, capnp.Client, error) {
			srv := data.NewSource()
			return srv, capnp.Client{}, nil
		},
		CreateInputSink: nil,
		BindDownstream: func(server any, port string, sinks []capnp.Client) error {
			srv := server.(*data.SourceImpl)
			dataSink := ToDataSink(sinks)
			srv.Downstream = func(ctx context.Context, ptr capnp.Ptr) error {
				if !capnp.Client(dataSink).IsValid() {
					return nil
				}
				eval, _ := types.EvaluationIDFromContext(ctx)
				if eval == 0 {
					ctx, eval = types.NextEvaluationContext(ctx)
				}
				msg := ptr.Message()
				if msg == nil {
					return nil
				}
				dataBytes, err := msg.Marshal()
				if err != nil {
					return err
				}
				return dataSink.Write(ctx, func(p types.DataSink_write_Params) error {
					p.SetEvaluation(eval)
					return p.SetValue(dataBytes)
				})
			}
			return nil
		},
		Invoke: func(ctx context.Context, client capnp.Client, setters map[string]func(capnp.Struct), downstreams map[string][]capnp.Client) error {
			return nil
		},
		Done: func(ctx context.Context, client capnp.Client, downstreams map[string][]capnp.Client) error {
			return nil
		},
	}
	r.Register(sourceDesc)
	sourceAlias := sourceDesc
	sourceAlias.Op = "source"
	r.Register(sourceAlias)

	// Float64 test source primitive
	float64SourceDesc := PrimitiveDescriptor{
		Op: "test.Float64Source",
		InputPorts: map[string]PortType{
			"in": PortTypeFloat64,
		},
		OutputPorts: map[string]PortType{
			"out": PortTypeFloat64,
		},
		Construct: func(node Node) (any, capnp.Client, error) {
			srv := &PassThroughServer{}
			client := types.Float64Sink_ServerToClient(srv)
			return srv, capnp.Client(client), nil
		},
		CreateInputSink: map[string]func(a *InvocationAssembler) capnp.Client{
			"in": func(a *InvocationAssembler) capnp.Client {
				return capnp.Client(types.NewFloat64Sink(
					func(ctx context.Context, eval uint64, val float64) error {
						return a.Record(ctx, eval, "in", func(s capnp.Struct) {
							types.Float64Sink_write_Params(s).SetValue(val)
						})
					},
					func(ctx context.Context) error {
						return a.RecordDone(ctx, "in")
					},
				))
			},
		},
		BindDownstream: func(server any, port string, sinks []capnp.Client) error {
			srv := server.(*PassThroughServer)
			srv.Downstream = ToFloat64Sink(sinks)
			return nil
		},
		Invoke: func(ctx context.Context, client capnp.Client, setters map[string]func(capnp.Struct), downstreams map[string][]capnp.Client) error {
			c := types.Float64Sink(client)
			return c.Write(ctx, func(p types.Float64Sink_write_Params) error {
				for _, setter := range setters {
					setter(capnp.Struct(p))
				}
				return nil
			})
		},
		Done: func(ctx context.Context, client capnp.Client, downstreams map[string][]capnp.Client) error {
			c := types.Float64Sink(client)
			_, release := c.Done(ctx, nil)
			release()
			return nil
		},
	}
	r.Register(float64SourceDesc)

	// Sink primitive (identity pass-through for graph termination, supporting Data and Float64)
	sinkDesc := PrimitiveDescriptor{
		Op: "data.Sink",
		InputPorts: map[string]PortType{
			"in":    PortTypeData,
			"value": PortTypeFloat64,
		},
		OutputPorts: map[string]PortType{
			"out": PortTypeData,
		},
		Construct: func(node Node) (any, capnp.Client, error) {
			srv := &PassThroughServer{}
			client := types.Float64Sink_ServerToClient(srv)
			return srv, capnp.Client(client), nil
		},
		CreateInputSink: map[string]func(a *InvocationAssembler) capnp.Client{
			"in": func(a *InvocationAssembler) capnp.Client {
				return capnp.Client(types.NewDataSink(
					func(ctx context.Context, eval uint64, val []byte) error {
						return a.Record(ctx, eval, "in", func(s capnp.Struct) {})
					},
					func(ctx context.Context) error {
						return a.RecordDone(ctx, "in")
					},
				))
			},
			"value": func(a *InvocationAssembler) capnp.Client {
				return capnp.Client(types.NewFloat64Sink(
					func(ctx context.Context, eval uint64, val float64) error {
						return a.Record(ctx, eval, "value", func(s capnp.Struct) {
							types.Float64Sink_write_Params(s).SetValue(val)
						})
					},
					func(ctx context.Context) error {
						return a.RecordDone(ctx, "value")
					},
				))
			},
		},
		BindDownstream: func(server any, port string, sinks []capnp.Client) error {
			srv := server.(*PassThroughServer)
			srv.Downstream = ToFloat64Sink(sinks)
			return nil
		},
		Invoke: func(ctx context.Context, client capnp.Client, setters map[string]func(capnp.Struct), downstreams map[string][]capnp.Client) error {
			c := types.Float64Sink(client)
			return c.Write(ctx, func(p types.Float64Sink_write_Params) error {
				for _, setter := range setters {
					setter(capnp.Struct(p))
				}
				return nil
			})
		},
		Done: func(ctx context.Context, client capnp.Client, downstreams map[string][]capnp.Client) error {
			c := types.Float64Sink(client)
			_, release := c.Done(ctx, nil)
			release()
			return nil
		},
	}
	r.Register(sinkDesc)
	sinkAlias := sinkDesc
	sinkAlias.Op = "sink"
	r.Register(sinkAlias)
}

func (r *Registry) Schemas() map[string]catalog.Schema {
	return nil
}
