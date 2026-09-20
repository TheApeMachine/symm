package compiler

import (
	"context"
	"fmt"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/catalog"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
PrimitiveDescriptor defines the Cap'n Proto schema, ports, constructors,
and invokers for a graph primitive.
*/
type PrimitiveDescriptor struct {
	Op             string
	InputPorts     map[string]PortType
	OutputPorts    map[string]PortType
	Construct      func(node Node) (server any, client capnp.Client, err error)
	CreateSetter   map[string]func(val any) func(capnp.Struct)
	BindDownstream func(server any, port string, sinks []capnp.Client) error
	Invoke         func(ctx context.Context, client capnp.Client, setters map[string]func(capnp.Struct)) error
	Done           func(ctx context.Context, client capnp.Client) error
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

func toFloat64Sink(sinks []capnp.Client) types.Float64Sink {
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

/*
PassThroughServer provides identity streaming for Source and Sink nodes.
*/
type PassThroughServer struct {
	Downstream types.Float64Sink
}

func (s *PassThroughServer) Write(ctx context.Context, call types.Float64Sink_write) error {
	val := call.Args().Value()
	if capnp.Client(s.Downstream).IsValid() {
		return s.Downstream.Write(ctx, func(p types.Float64Sink_write_Params) error {
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
	// Source primitive (Float64 stream)
	sourceDesc := PrimitiveDescriptor{
		Op: "data.Source",
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
		CreateSetter: map[string]func(val any) func(capnp.Struct){
			"in": func(val any) func(capnp.Struct) {
				v := val.(float64)
				return func(s capnp.Struct) {
					types.Float64Sink_write_Params(s).SetValue(v)
				}
			},
		},
		BindDownstream: func(server any, port string, sinks []capnp.Client) error {
			srv := server.(*PassThroughServer)
			srv.Downstream = toFloat64Sink(sinks)
			return nil
		},
		Invoke: func(ctx context.Context, client capnp.Client, setters map[string]func(capnp.Struct)) error {
			c := types.Float64Sink(client)
			return c.Write(ctx, func(p types.Float64Sink_write_Params) error {
				if setter, ok := setters["in"]; ok {
					setter(capnp.Struct(p))
				}
				return nil
			})
		},
		Done: func(ctx context.Context, client capnp.Client) error {
			c := types.Float64Sink(client)
			_, release := c.Done(ctx, nil)
			release()
			return nil
		},
	}
	r.Register(sourceDesc)
	sourceAlias := sourceDesc
	sourceAlias.Op = "source"
	r.Register(sourceAlias)

	// Sink primitive (Float64 stream)
	sinkDesc := PrimitiveDescriptor{
		Op: "data.Sink",
		InputPorts: map[string]PortType{
			"value": PortTypeFloat64,
		},
		OutputPorts: map[string]PortType{
			"out": PortTypeFloat64,
		},
		Construct: func(node Node) (any, capnp.Client, error) {
			srv := &PassThroughServer{}
			client := types.Float64Sink_ServerToClient(srv)
			return srv, capnp.Client(client), nil
		},
		CreateSetter: map[string]func(val any) func(capnp.Struct){
			"value": func(val any) func(capnp.Struct) {
				v := val.(float64)
				return func(s capnp.Struct) {
					types.Float64Sink_write_Params(s).SetValue(v)
				}
			},
		},
		BindDownstream: func(server any, port string, sinks []capnp.Client) error {
			srv := server.(*PassThroughServer)
			srv.Downstream = toFloat64Sink(sinks)
			return nil
		},
		Invoke: func(ctx context.Context, client capnp.Client, setters map[string]func(capnp.Struct)) error {
			c := types.Float64Sink(client)
			return c.Write(ctx, func(p types.Float64Sink_write_Params) error {
				if setter, ok := setters["value"]; ok {
					setter(capnp.Struct(p))
				}
				return nil
			})
		},
		Done: func(ctx context.Context, client capnp.Client) error {
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

	// calculus.Atanh
	r.Register(PrimitiveDescriptor{
		Op: "calculus.Atanh",
		InputPorts: map[string]PortType{
			"a": PortTypeFloat64,
		},
		OutputPorts: map[string]PortType{
			"out": PortTypeFloat64,
		},
		Construct: func(node Node) (any, capnp.Client, error) {
			srv := calculus.NewAtanh()
			client := calculus.Atanh_ServerToClient(srv)
			return srv, capnp.Client(client), nil
		},
		CreateSetter: map[string]func(val any) func(capnp.Struct){
			"a": func(val any) func(capnp.Struct) {
				v := val.(float64)
				return func(s capnp.Struct) {
					calculus.Atanh_write_Params(s).SetA(v)
				}
			},
		},
		BindDownstream: func(server any, port string, sinks []capnp.Client) error {
			srv := server.(*calculus.AtanhServer)
			srv.Downstream = toFloat64Sink(sinks)
			return nil
		},
		Invoke: func(ctx context.Context, client capnp.Client, setters map[string]func(capnp.Struct)) error {
			c := calculus.Atanh(client)
			return c.Write(ctx, func(p calculus.Atanh_write_Params) error {
				if setter, ok := setters["a"]; ok {
					setter(capnp.Struct(p))
				}
				return nil
			})
		},
		Done: func(ctx context.Context, client capnp.Client) error {
			c := calculus.Atanh(client)
			_, release := c.Done(ctx, nil)
			release()
			return nil
		},
	})

	// arithmetic.Add
	r.Register(PrimitiveDescriptor{
		Op: "arithmetic.Add",
		InputPorts: map[string]PortType{
			"a": PortTypeFloat64,
			"b": PortTypeFloat64,
		},
		OutputPorts: map[string]PortType{
			"out": PortTypeFloat64,
		},
		Construct: func(node Node) (any, capnp.Client, error) {
			srv := arithmetic.NewAdd()
			client := arithmetic.Add_ServerToClient(srv)
			return srv, capnp.Client(client), nil
		},
		CreateSetter: map[string]func(val any) func(capnp.Struct){
			"a": func(val any) func(capnp.Struct) {
				v := val.(float64)
				return func(s capnp.Struct) {
					arithmetic.Add_write_Params(s).SetA(v)
				}
			},
			"b": func(val any) func(capnp.Struct) {
				v := val.(float64)
				return func(s capnp.Struct) {
					arithmetic.Add_write_Params(s).SetB(v)
				}
			},
		},
		BindDownstream: func(server any, port string, sinks []capnp.Client) error {
			srv := server.(*arithmetic.AddServer)
			srv.Downstream = toFloat64Sink(sinks)
			return nil
		},
		Invoke: func(ctx context.Context, client capnp.Client, setters map[string]func(capnp.Struct)) error {
			c := arithmetic.Add(client)
			return c.Write(ctx, func(p arithmetic.Add_write_Params) error {
				if setter, ok := setters["a"]; ok {
					setter(capnp.Struct(p))
				}
				if setter, ok := setters["b"]; ok {
					setter(capnp.Struct(p))
				}
				return nil
			})
		},
		Done: func(ctx context.Context, client capnp.Client) error {
			c := arithmetic.Add(client)
			_, release := c.Done(ctx, nil)
			release()
			return nil
		},
	})

	// arithmetic.Subtract
	r.Register(PrimitiveDescriptor{
		Op: "arithmetic.Subtract",
		InputPorts: map[string]PortType{
			"a": PortTypeFloat64,
			"b": PortTypeFloat64,
		},
		OutputPorts: map[string]PortType{
			"out": PortTypeFloat64,
		},
		Construct: func(node Node) (any, capnp.Client, error) {
			srv := arithmetic.NewSubtract()
			client := arithmetic.Subtract_ServerToClient(srv)
			return srv, capnp.Client(client), nil
		},
		CreateSetter: map[string]func(val any) func(capnp.Struct){
			"a": func(val any) func(capnp.Struct) {
				v := val.(float64)
				return func(s capnp.Struct) {
					arithmetic.Subtract_write_Params(s).SetA(v)
				}
			},
			"b": func(val any) func(capnp.Struct) {
				v := val.(float64)
				return func(s capnp.Struct) {
					arithmetic.Subtract_write_Params(s).SetB(v)
				}
			},
		},
		BindDownstream: func(server any, port string, sinks []capnp.Client) error {
			srv := server.(*arithmetic.SubtractServer)
			srv.Downstream = toFloat64Sink(sinks)
			return nil
		},
		Invoke: func(ctx context.Context, client capnp.Client, setters map[string]func(capnp.Struct)) error {
			c := arithmetic.Subtract(client)
			return c.Write(ctx, func(p arithmetic.Subtract_write_Params) error {
				if setter, ok := setters["a"]; ok {
					setter(capnp.Struct(p))
				}
				if setter, ok := setters["b"]; ok {
					setter(capnp.Struct(p))
				}
				return nil
			})
		},
		Done: func(ctx context.Context, client capnp.Client) error {
			c := arithmetic.Subtract(client)
			_, release := c.Done(ctx, nil)
			release()
			return nil
		},
	})

	// arithmetic.Multiply
	r.Register(PrimitiveDescriptor{
		Op: "arithmetic.Multiply",
		InputPorts: map[string]PortType{
			"a": PortTypeFloat64,
			"b": PortTypeFloat64,
		},
		OutputPorts: map[string]PortType{
			"out": PortTypeFloat64,
		},
		Construct: func(node Node) (any, capnp.Client, error) {
			srv := arithmetic.NewMultiply()
			client := arithmetic.Multiply_ServerToClient(srv)
			return srv, capnp.Client(client), nil
		},
		CreateSetter: map[string]func(val any) func(capnp.Struct){
			"a": func(val any) func(capnp.Struct) {
				v := val.(float64)
				return func(s capnp.Struct) {
					arithmetic.Multiply_write_Params(s).SetA(v)
				}
			},
			"b": func(val any) func(capnp.Struct) {
				v := val.(float64)
				return func(s capnp.Struct) {
					arithmetic.Multiply_write_Params(s).SetB(v)
				}
			},
		},
		BindDownstream: func(server any, port string, sinks []capnp.Client) error {
			srv := server.(*arithmetic.MultiplyServer)
			srv.Downstream = toFloat64Sink(sinks)
			return nil
		},
		Invoke: func(ctx context.Context, client capnp.Client, setters map[string]func(capnp.Struct)) error {
			c := arithmetic.Multiply(client)
			return c.Write(ctx, func(p arithmetic.Multiply_write_Params) error {
				if setter, ok := setters["a"]; ok {
					setter(capnp.Struct(p))
				}
				if setter, ok := setters["b"]; ok {
					setter(capnp.Struct(p))
				}
				return nil
			})
		},
		Done: func(ctx context.Context, client capnp.Client) error {
			c := arithmetic.Multiply(client)
			_, release := c.Done(ctx, nil)
			release()
			return nil
		},
	})

	// arithmetic.Divide
	r.Register(PrimitiveDescriptor{
		Op: "arithmetic.Divide",
		InputPorts: map[string]PortType{
			"a": PortTypeFloat64,
			"b": PortTypeFloat64,
		},
		OutputPorts: map[string]PortType{
			"out": PortTypeFloat64,
		},
		Construct: func(node Node) (any, capnp.Client, error) {
			srv := arithmetic.NewDivide()
			client := arithmetic.Divide_ServerToClient(srv)
			return srv, capnp.Client(client), nil
		},
		CreateSetter: map[string]func(val any) func(capnp.Struct){
			"a": func(val any) func(capnp.Struct) {
				v := val.(float64)
				return func(s capnp.Struct) {
					arithmetic.Divide_write_Params(s).SetA(v)
				}
			},
			"b": func(val any) func(capnp.Struct) {
				v := val.(float64)
				return func(s capnp.Struct) {
					arithmetic.Divide_write_Params(s).SetB(v)
				}
			},
		},
		BindDownstream: func(server any, port string, sinks []capnp.Client) error {
			srv := server.(*arithmetic.DivideServer)
			srv.Downstream = toFloat64Sink(sinks)
			return nil
		},
		Invoke: func(ctx context.Context, client capnp.Client, setters map[string]func(capnp.Struct)) error {
			c := arithmetic.Divide(client)
			return c.Write(ctx, func(p arithmetic.Divide_write_Params) error {
				if setter, ok := setters["a"]; ok {
					setter(capnp.Struct(p))
				}
				if setter, ok := setters["b"]; ok {
					setter(capnp.Struct(p))
				}
				return nil
			})
		},
		Done: func(ctx context.Context, client capnp.Client) error {
			c := arithmetic.Divide(client)
			_, release := c.Done(ctx, nil)
			release()
			return nil
		},
	})
}

func (r *Registry) Schemas() map[string]catalog.Schema {
	return nil
}
