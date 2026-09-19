package compiler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/catalog"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/execution"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Factory is a function that takes a declarative Node definition (including any
constructor configuration in InputData) and instantiates its executable closure.
*/
type Factory func(node Node) (types.Value[any, any], error)

/*
Registry resolves declarative node names to real nomagique constructors.
*/
type Registry struct {
	mu           sync.RWMutex
	factories    map[string]Factory
	schemas      map[string]catalog.Schema
	repo         DefinitionRepository
	dependencies map[string]any
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

var (
	defaultRegistryOnce sync.Once
	defaultRegistryInst *Registry
)

/*
DefaultRegistry returns the singleton registry populated with all scanned nomagique primitives.
*/
func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		var err error
		defaultRegistryInst, err = NewRegistry(nil)
		if err != nil {
			errnie.Error(err)
		}
	})

	return defaultRegistryInst
}

/*
NewRegistry constructs a Registry populated from scanned schemas and authoritative nomagique constructors.
Errors during catalog loading are explicitly returned and must not be swallowed.
*/
func NewRegistry(schemas map[string]catalog.Schema) (*Registry, error) {
	if schemas == nil {
		var err error
		schemas, err = catalog.Load()
		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"compiler: failed to load authoritative primitive catalog",
				err,
			))
		}
	}

	registry := &Registry{
		factories: make(map[string]Factory),
		schemas:   schemas,
	}

	for op, legacyFactory := range DefaultPrimitiveFactories {
		fn := legacyFactory
		registry.factories[op] = func(Node) (types.Value[any, any], error) {
			return fn(), nil
		}
	}

	closureMul := arithmetic.NewMultiply()
	registry.factories["arithmetic.Multiply"] = func(Node) (types.Value[any, any], error) {
		return func(in any) any {
			return closureMul(toFloatPair(in))
		}, nil
	}

	closureDiv := arithmetic.NewDivide()
	registry.factories["arithmetic.Divide"] = func(Node) (types.Value[any, any], error) {
		return func(in any) any {
			return closureDiv(toFloatPair(in))
		}, nil
	}

	closureAdd := arithmetic.NewAdd()
	registry.factories["arithmetic.Add"] = func(Node) (types.Value[any, any], error) {
		return func(in any) any {
			return closureAdd(toFloatPair(in))
		}, nil
	}

	closureSub := arithmetic.NewSubtract()
	registry.factories["arithmetic.Subtract"] = func(Node) (types.Value[any, any], error) {
		return func(in any) any {
			return closureSub(toFloatPair(in))
		}, nil
	}

	closureElapsed := temporal.NewElapsed()
	registry.factories["temporal.Elapsed"] = func(Node) (types.Value[any, any], error) {
		return func(in any) any {
			return closureElapsed(toInt64(in))
		}, nil
	}

	registry.factories["data.Extract"] = func(node Node) (types.Value[any, any], error) {
		key := ""

		if node.InputData != nil {
			if k, ok := node.InputData["key"].(string); ok && k != "" {
				key = k
			} else if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if p, ok := cfg["path"].(string); ok && p != "" {
					key = p
				} else if k, ok := cfg["key"].(string); ok && k != "" {
					key = k
				}
			}
		}

		if key == "" {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("compiler: node %q (%s) requires 'path' or 'key' configuration", node.ID, node.Type),
				nil,
			))
		}

		closure := data.NewExtract(types.Const(key))

		return func(in any) any {
			return closure(in)
		}, nil
	}

	registry.factories["transport.Collect"] = func(node Node) (types.Value[any, any], error) {
		batchSize := 0

		if node.InputData != nil {
			if b, ok := node.InputData["batchSize"].(float64); ok && b > 0 {
				batchSize = int(b)
			} else if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if b, ok := cfg["batchSize"].(float64); ok && b > 0 {
					batchSize = int(b)
				}
			}
		}

		if batchSize <= 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("compiler: node %q (%s) requires positive 'batchSize' configuration", node.ID, node.Type),
				nil,
			))
		}

		closure := transport.NewCollect[any](types.Const(batchSize))

		return func(in any) any {
			return closure(in)
		}, nil
	}

	registry.factories["transport.Discard"] = func(node Node) (types.Value[any, any], error) {
		closure := transport.NewDiscard[any]()

		return func(in any) any {
			return closure(in)
		}, nil
	}

	registry.factories["transport.Pace"] = func(node Node) (types.Value[any, any], error) {
		delayMs := 0

		if node.InputData != nil {
			if d, ok := node.InputData["delay"].(float64); ok && d > 0 {
				delayMs = int(d)
			} else if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if d, ok := cfg["delay"].(float64); ok && d > 0 {
					delayMs = int(d)
				}
			}
		}

		closure := transport.NewPace[any](types.Const(delayMs))

		return func(in any) any {
			return closure(in)
		}, nil
	}

	registry.factories["transport.Process"] = func(node Node) (types.Value[any, any], error) {
		binary := ""
		var defaultArgs []string

		if node.InputData != nil {
			if b, ok := node.InputData["binary"].(string); ok {
				binary = b
			}
			if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if b, ok := cfg["binary"].(string); ok {
					binary = b
				}
				if args, ok := cfg["args"].([]any); ok {
					for _, a := range args {
						if s, ok := a.(string); ok {
							defaultArgs = append(defaultArgs, s)
						}
					}
				}
			}
		}

		if binary == "" {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("compiler: node %q (%s) requires 'binary' configuration", node.ID, node.Type),
				nil,
			))
		}

		defaultArgValues := make([]types.String, len(defaultArgs))
		for i, a := range defaultArgs {
			defaultArgValues[i] = types.Const(a)
		}

		closure := transport.NewProcess(types.Const(binary), defaultArgValues...)

		return func(in any) any {
			var args []string
			if in != nil {
				if strList, ok := in.([]string); ok {
					args = strList
				} else if str, ok := in.(string); ok {
					args = []string{str}
				}
			}
			return closure(args)
		}, nil
	}

	registry.factories["transport.Shell"] = registry.factories["transport.Process"]

	registry.factories["transport.Message"] = func(node Node) (types.Value[any, any], error) {
		var payload any
		if node.InputData != nil {
			if m, ok := node.InputData["message"]; ok {
				payload = m
			} else if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if m, ok := cfg["message"]; ok {
					payload = m
				}
			}
		}
		closure := transport.NewJSONMessage(types.Const(payload))
		return func(in any) any {
			return closure(in)
		}, nil
	}

	registry.factories["transport.HTTPRequest"] = func(node Node) (types.Value[any, any], error) {
		methodStr := "GET"
		urlStr := ""

		if node.InputData != nil {
			if m, ok := node.InputData["method"].(string); ok && m != "" {
				methodStr = m
			} else if m, ok := node.InputData["method"].(map[string]any); ok {
				if s, ok := m["string"].(string); ok && s != "" {
					methodStr = s
				}
			}

			if u, ok := node.InputData["url"].(string); ok && u != "" {
				urlStr = u
			} else if u, ok := node.InputData["rawURL"].(string); ok && u != "" {
				urlStr = u
			} else if u, ok := node.InputData["url"].(map[string]any); ok {
				if s, ok := u["string"].(string); ok && s != "" {
					urlStr = s
				}
			} else if u, ok := node.InputData["rawURL"].(map[string]any); ok {
				if s, ok := u["string"].(string); ok && s != "" {
					urlStr = s
				}
			}
		}

		methodPort := types.Const(methodStr)
		urlPort := types.Const(urlStr)

		closure := transport.NewHTTPRequest(methodPort, urlPort)
		return func(in any) any {
			if m, ok := in.(map[string]any); ok {
				return closure(m)
			}
			return closure(nil)
		}, nil
	}

	registry.factories["transport.HeaderAuth"] = func(node Node) (types.Value[any, any], error) {
		keyStr := ""
		valStr := ""
		if node.InputData != nil {
			if k, ok := node.InputData["key"].(string); ok {
				keyStr = k
			}
			if v, ok := node.InputData["value"].(string); ok {
				valStr = v
			}
		}
		closure := transport.NewHeaderAuth(types.Const(keyStr), types.Const(valStr))
		return func(in any) any {
			if m, ok := in.(map[string]any); ok {
				return closure(m)
			}
			return in
		}, nil
	}

	registry.factories["transport.WSStream"] = func(node Node) (types.Value[any, any], error) {
		var endpoint string
		var messages []any
		pace := 500 * time.Millisecond

		if node.InputData != nil {
			if ep, ok := node.InputData["endpoint"].(string); ok && ep != "" {
				endpoint = ep
			}
			if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if ep, ok := cfg["endpoint"].(string); ok && ep != "" {
					endpoint = ep
				}
				if pStr, ok := cfg["pace"].(string); ok {
					if d, err := time.ParseDuration(pStr); err == nil {
						pace = d
					}
				}
				if msgs, ok := cfg["messages"].([]any); ok {
					messages = append(messages, msgs...)
				}
			}
		}

		ctx := context.Background()
		if depCtx, ok := registry.Dependency("ctx").(context.Context); ok && depCtx != nil {
			ctx = depCtx
		}

		return func(in any) any {
			msgs := make([]types.Any, 0, len(messages)+1)
			for _, m := range messages {
				msgs = append(msgs, types.Const(m))
			}
			if in != nil {
				msgs = append(msgs, types.Const(in))
			}
			stream := transport.NewWSStream(ctx, types.Const(endpoint), msgs, types.Const(int(pace.Milliseconds())))
			return stream
		}, nil
	}

	registry.factories["data.Select"] = func(node Node) (types.Value[any, any], error) {
		path := ""

		if node.InputData != nil {
			if p, ok := node.InputData["path"].(string); ok {
				path = p
			} else if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if p, ok := cfg["path"].(string); ok {
					path = p
				}
			}
		}

		if path == "" {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("compiler: node %q (%s) requires 'path' configuration", node.ID, node.Type),
				nil,
			))
		}

		closure := data.NewSelect(types.Const(path))

		return func(in any) any {
			return closure(in)
		}, nil
	}

	registry.factories["ui.Broadcast"] = func(node Node) (types.Value[any, any], error) {
		return func(in any) any {
			return in
		}, nil
	}

	registry.factories["temporal.Delay"] = func(node Node) (types.Value[any, any], error) {
		horizon := 0

		if node.InputData != nil {
			if h, ok := node.InputData["horizon"].(float64); ok && h > 0 {
				horizon = int(h)
			} else if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if h, ok := cfg["horizon"].(float64); ok && h > 0 {
					horizon = int(h)
				}
			}
		}

		if horizon <= 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("compiler: node %q (%s) requires positive 'horizon' configuration", node.ID, node.Type),
				nil,
			))
		}

		closure := temporal.NewDelay[float64](types.Const(horizon))

		return func(in any) any {
			var val float64
			if in != nil {
				if f, ok := in.(float64); ok {
					val = f
				}
			}
			return closure(val)
		}, nil
	}

	registry.factories["probability.Distribution"] = func(node Node) (types.Value[any, any], error) {
		closure := probability.NewDistribution()
		return func(in any) any {
			if in == nil {
				return probability.Reading{}
			}
			switch v := in.(type) {
			case []float64:
				return closure(v)
			case float64:
				return closure([]float64{v})
			case []any:
				res := make([]float64, 0, len(v))
				for _, item := range v {
					if f, ok := item.(float64); ok {
						res = append(res, f)
					}
				}
				return closure(res)
			default:
				return probability.Reading{}
			}
		}, nil
	}

	registry.factories["probability.Entropy"] = func(node Node) (types.Value[any, any], error) {
		closure := probability.NewEntropy()
		return func(in any) any {
			switch v := in.(type) {
			case float64:
				return closure(v)
			case probability.Reading:
				return closure(v.Ambiguity)
			default:
				return 0.0
			}
		}, nil
	}

	registry.factories["probability.Concentration"] = func(node Node) (types.Value[any, any], error) {
		closure := probability.NewConcentration()
		return func(in any) any {
			switch v := in.(type) {
			case probability.ShapeInput:
				return closure(v)
			case probability.Reading:
				return closure(probability.ShapeInput{Weights: v.Probabilities})
			case []float64:
				return closure(probability.ShapeInput{Weights: v})
			case float64:
				return closure(probability.ShapeInput{Weights: []float64{v}})
			default:
				return 0.0
			}
		}, nil
	}

	registry.factories["probability.KolmogorovSmirnov"] = func(node Node) (types.Value[any, any], error) {
		closure := probability.NewKolmogorovSmirnov()
		return func(in any) any {
			switch v := in.(type) {
			case probability.DistanceInput:
				return closure(v)
			case probability.Reading:
				return closure(probability.DistanceInput{
					Positions: make([]float64, len(v.Probabilities)),
					WeightsA:  v.Probabilities,
					WeightsB:  v.Probabilities,
				})
			default:
				return 0.0
			}
		}, nil
	}

	registry.factories["store.Grid"] = func(node Node) (types.Value[any, any], error) {
		grid := store.NewGrid[any, any]()

		if node.InputData != nil {
			var metricList []string
			if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if metrics, ok := cfg["metrics"].([]any); ok {
					for _, m := range metrics {
						if s, ok := m.(string); ok {
							metricList = append(metricList, s)
						}
					}
				}
			}

			repo := registry.Repository()
			if len(metricList) > 0 && repo == nil {
				return nil, errnie.Error(errnie.Err(
					errnie.Validation,
					"compiler: store.Grid requires a DefinitionRepository to load configured metrics",
					nil,
				))
			}

			for _, metricName := range metricList {
				metricGraph, err := repo.Load(metricName)
				if err != nil {
					return nil, errnie.Error(errnie.Err(
						errnie.NotFound,
						fmt.Sprintf("compiler: metric definition %q not found for store.Grid", metricName),
						err,
					))
				}

				metricPipeline, err := Compile[any, any](metricGraph, registry, repo)
				if err != nil {
					return nil, errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf("compiler: failed to compile metric definition %q for store.Grid", metricName),
						err,
					))
				}

				grid(transport.NewMessage[any, any](transport.REGISTER, nil, types.Value[any, any](metricPipeline)))
			}
		}

		return func(in any) any {
			if in == nil {
				return nil
			}

			raw := grid(transport.NewMessage[any, any](transport.POKE, in, nil))
			if len(raw) == 0 {
				return nil
			}

			impulse := make([]float64, len(raw))
			for i, r := range raw {
				switch v := r.(type) {
				case float64:
					impulse[i] = v
				case *float64:
					if v != nil {
						impulse[i] = *v
					}
				case int:
					impulse[i] = float64(v)
				case int64:
					impulse[i] = float64(v)
				case []any:
					for _, item := range v {
						if f, ok := item.(float64); ok {
							impulse[i] = f
							break
						}
					}
				}
			}

			return impulse
		}, nil
	}

	registry.factories["execution.Decide"] = func(node Node) (types.Value[any, any], error) {
		minContrast := 0.0
		if node.InputData != nil {
			if c, ok := node.InputData["minContrast"].(float64); ok {
				minContrast = c
			} else if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if c, ok := cfg["minContrast"].(float64); ok {
					minContrast = c
				}
			}
		}
		closure := execution.NewDecide(types.Const(minContrast))
		return func(in any) any {
			if eval, ok := in.(cognition.Evaluation); ok {
				return closure(eval)
			}
			return "wait"
		}, nil
	}

	registry.factories["execution.Gate"] = func(node Node) (types.Value[any, any], error) {
		initialHolding := false
		if node.InputData != nil {
			if h, ok := node.InputData["holding"].(bool); ok {
				initialHolding = h
			} else if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if h, ok := cfg["holding"].(bool); ok {
					initialHolding = h
				}
			}
		}
		closure := execution.NewGate(types.Const(initialHolding))
		return func(in any) any {
			if act, ok := in.(string); ok {
				return closure(act)
			}
			return "wait"
		}, nil
	}

	registry.factories["execution.Submit"] = func(node Node) (types.Value[any, any], error) {
		symbol := ""
		if node.InputData != nil {
			if s, ok := node.InputData["symbol"].(string); ok {
				symbol = s
			} else if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if s, ok := cfg["symbol"].(string); ok {
					symbol = s
				}
			}
		}
		closure := execution.NewSubmit(types.Const(symbol))
		return func(in any) any {
			if act, ok := in.(string); ok {
				return closure(act)
			}
			return nil
		}, nil
	}

	return registry, nil
}

/*
Register associates an operation name with a constructor factory.
*/
func (r *Registry) Register(op string, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.factories[op] = f
}

/*
Resolve maps a declarative JSON node to an instantiated types.Value closure.
It does not inspect node IDs to infer semantic roles.
*/
func (r *Registry) Resolve(node Node) (types.Value[any, any], error) {
	if node.Type == "data.Source" || node.Type == "source" {
		return func(in any) any { return in }, nil
	}

	if node.Type == "data.Sink" || node.Type == "sink" {
		return func(in any) any { return in }, nil
	}

	r.mu.RLock()
	factory, exists := r.factories[node.Type]
	r.mu.RUnlock()

	if exists && factory != nil {
		return factory(node)
	}

	return nil, errnie.Error(errnie.Err(
		errnie.NotFound,
		fmt.Sprintf("compiler: unknown primitive type %q", node.Type),
		nil,
	))
}

/*
Primitives returns the authoritative catalog of primitive schemas for UI/workbench consumers.
*/
func Primitives() (any, error) {
	reg := DefaultRegistry()
	if reg == nil {
		return catalog.Load()
	}

	reg.mu.RLock()
	defer reg.mu.RUnlock()

	if len(reg.schemas) > 0 {
		return reg.schemas, nil
	}

	return catalog.Load()
}

func toFloatPair(in any) [2]float64 {
	if p, ok := in.([2]float64); ok {
		return p
	}
	if s, ok := in.([]any); ok && len(s) == 2 {
		var p [2]float64
		if f0, ok := s[0].(float64); ok {
			p[0] = f0
		}
		if f1, ok := s[1].(float64); ok {
			p[1] = f1
		}
		return p
	}
	if s, ok := in.([]float64); ok && len(s) == 2 {
		return [2]float64{s[0], s[1]}
	}
	panic(fmt.Sprintf("arithmetic: expected [2]float64 or 2-element slice, got %T", in))
}

func toInt64(in any) int64 {
	switch v := in.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case map[string]any:
		extract := data.NewExtract(types.Const("timestamp"))
		return int64(extract(v))
	default:
		panic(fmt.Sprintf("temporal.Elapsed: expected integer or float timestamp, got %T", in))
	}
}
