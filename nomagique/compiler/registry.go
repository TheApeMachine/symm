package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/schemas"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/execution"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/store/tables"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/ui"
)

/*
Constructor instantiates a Cap'n Proto capability client from raw config bytes.
*/
type Constructor func(ctx context.Context, config []byte) (capnp.Client, error)

/*
Factory pairs a Cap'n Proto capability constructor with its interface type ID.
*/
type Factory struct {
	InterfaceID uint64
	New         Constructor
}

/*
Registry maps primitive operation strings to Cap'n Proto capability factories.
*/
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
	}
}

func (r *Registry) Register(op string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[op] = factory
}

func (r *Registry) Resolve(op string) (Factory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.factories[op]
	if !exists {
		return Factory{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("compiler: unknown primitive type %q", op),
			nil,
		))
	}

	return factory, nil
}

func (r *Registry) ResolveMust(op string) Factory {
	f, err := r.Resolve(op)
	if err != nil {
		panic(err)
	}
	return f
}

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *Registry
)

func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		// Register all Cap'n Proto package schemas
		reg := schemas.DefaultRegistry
		algo.RegisterSchema(reg)
		arithmetic.RegisterSchema(reg)
		calculus.RegisterSchema(reg)
		cognition.RegisterSchema(reg)
		data.RegisterSchema(reg)
		execution.RegisterSchema(reg)
		geometry.RegisterSchema(reg)
		learning.RegisterSchema(reg)
		associative.RegisterSchema(reg)
		probability.RegisterSchema(reg)
		statistic.RegisterSchema(reg)
		hawkes.RegisterSchema(reg)
		store.RegisterSchema(reg)
		tables.RegisterSchema(reg)
		temporal.RegisterSchema(reg)
		transport.RegisterSchema(reg)
		ui.RegisterSchema(reg)

		r := NewRegistry()

		// Arithmetic
		r.Register("arithmetic.Add", Factory{
			InterfaceID: arithmetic.Add_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(arithmetic.Add_ServerToClient(arithmetic.NewAdd())), nil
			},
		})
		r.Register("arithmetic.Subtract", Factory{
			InterfaceID: arithmetic.Subtract_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(arithmetic.Subtract_ServerToClient(arithmetic.NewSubtract())), nil
			},
		})
		r.Register("arithmetic.Multiply", Factory{
			InterfaceID: arithmetic.Multiply_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(arithmetic.Multiply_ServerToClient(arithmetic.NewMultiply())), nil
			},
		})
		r.Register("arithmetic.Divide", Factory{
			InterfaceID: arithmetic.Divide_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(arithmetic.Divide_ServerToClient(arithmetic.NewDivide())), nil
			},
		})

		// Calculus
		r.Register("calculus.Atanh", Factory{
			InterfaceID: calculus.Atanh_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Atanh_ServerToClient(calculus.NewAtanh())), nil
			},
		})
		r.Register("calculus.Tanh", Factory{
			InterfaceID: calculus.Tanh_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Tanh_ServerToClient(calculus.NewTanh())), nil
			},
		})
		r.Register("calculus.Square", Factory{
			InterfaceID: calculus.Square_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Square_ServerToClient(calculus.NewSquare())), nil
			},
		})
		r.Register("calculus.Sqrt", Factory{
			InterfaceID: calculus.Sqrt_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Sqrt_ServerToClient(calculus.NewSqrt())), nil
			},
		})
		r.Register("calculus.Absolute", Factory{
			InterfaceID: calculus.Absolute_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Absolute_ServerToClient(calculus.NewAbsolute())), nil
			},
		})
		r.Register("calculus.Exp", Factory{
			InterfaceID: calculus.Exp_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Exp_ServerToClient(calculus.NewExp())), nil
			},
		})
		r.Register("calculus.Log", Factory{
			InterfaceID: calculus.Log_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Log_ServerToClient(calculus.NewLog())), nil
			},
		})
		r.Register("calculus.Floor", Factory{
			InterfaceID: calculus.Floor_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Floor_ServerToClient(calculus.NewFloor())), nil
			},
		})
		r.Register("calculus.Negate", Factory{
			InterfaceID: calculus.Negate_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Negate_ServerToClient(calculus.NewNegate())), nil
			},
		})
		r.Register("calculus.Reciprocal", Factory{
			InterfaceID: calculus.Reciprocal_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Reciprocal_ServerToClient(calculus.NewReciprocal())), nil
			},
		})
		r.Register("calculus.Sign", Factory{
			InterfaceID: calculus.Sign_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Sign_ServerToClient(calculus.NewSign())), nil
			},
		})
		r.Register("calculus.Polarize", Factory{
			InterfaceID: calculus.Polarize_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Polarize_ServerToClient(calculus.NewPolarize())), nil
			},
		})
		r.Register("calculus.Erfc", Factory{
			InterfaceID: calculus.Erfc_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Erfc_ServerToClient(calculus.NewErfc())), nil
			},
		})
		r.Register("calculus.Bound", Factory{
			InterfaceID: calculus.Bound_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Bound_ServerToClient(calculus.NewBound())), nil
			},
		})
		r.Register("calculus.SecondDifference", Factory{
			InterfaceID: calculus.SecondDifference_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.SecondDifference_ServerToClient(calculus.NewSecondDifference())), nil
			},
		})
		r.Register("calculus.RelativeChange", Factory{
			InterfaceID: calculus.RelativeChange_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.RelativeChange_ServerToClient(calculus.NewRelativeChange())), nil
			},
		})
		r.Register("calculus.Maximum", Factory{
			InterfaceID: calculus.Maximum_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Maximum_ServerToClient(calculus.NewMaximum())), nil
			},
		})
		r.Register("calculus.Minimum", Factory{
			InterfaceID: calculus.Minimum_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Minimum_ServerToClient(calculus.NewMinimum())), nil
			},
		})

		// Statistic
		r.Register("statistic.Mean", Factory{
			InterfaceID: statistic.Mean_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(statistic.Mean_ServerToClient(statistic.NewMean())), nil
			},
		})
		r.Register("statistic.Variance", Factory{
			InterfaceID: statistic.Variance_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(statistic.Variance_ServerToClient(statistic.NewVariance())), nil
			},
		})
		r.Register("statistic.CausalMean", Factory{
			InterfaceID: statistic.CausalMean_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(statistic.CausalMean_ServerToClient(statistic.NewCausalMean())), nil
			},
		})
		r.Register("statistic.CausalVariance", Factory{
			InterfaceID: statistic.CausalVariance_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(statistic.CausalVariance_ServerToClient(statistic.NewCausalVariance())), nil
			},
		})
		r.Register("statistic.Threshold", Factory{
			InterfaceID: statistic.Threshold_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				var config struct {
					Band  float64 `json:"band"`
					Rest  float64 `json:"rest"`
					Lower float64 `json:"lower"`
					Upper float64 `json:"upper"`
				}
				if len(cfg) > 0 {
					_ = json.Unmarshal(cfg, &config)
				}
				return capnp.Client(statistic.Threshold_ServerToClient(statistic.NewThreshold(
					config.Band, config.Rest, config.Lower, config.Upper,
				))), nil
			},
		})
		r.Register("statistic.ZScore", Factory{
			InterfaceID: statistic.ZScore_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(statistic.ZScore_ServerToClient(statistic.NewZScore())), nil
			},
		})
		r.Register("statistic.ResidualBaseline", Factory{
			InterfaceID: statistic.ResidualBaseline_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(statistic.ResidualBaseline_ServerToClient(statistic.NewResidualBaseline())), nil
			},
		})
		r.Register("statistic.ResidualDivergence", Factory{
			InterfaceID: statistic.ResidualDivergence_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(statistic.ResidualDivergence_ServerToClient(statistic.NewResidualDivergence())), nil
			},
		})
		r.Register("statistic.EMA", Factory{
			InterfaceID: statistic.EMA_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(statistic.EMA_ServerToClient(statistic.NewEMA())), nil
			},
		})

		// Hawkes
		r.Register("hawkes.Assemble", Factory{
			InterfaceID: hawkes.Assemble_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(hawkes.Assemble_ServerToClient(hawkes.NewAssemble())), nil
			},
		})
		r.Register("hawkes.Process", Factory{
			InterfaceID: hawkes.Process_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(hawkes.Process_ServerToClient(hawkes.NewProcess())), nil
			},
		})
		r.Register("hawkes.EventCount", Factory{
			InterfaceID: hawkes.EventCount_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(hawkes.EventCount_ServerToClient(hawkes.NewEventCount())), nil
			},
		})
		r.Register("hawkes.BuyCount", Factory{
			InterfaceID: hawkes.BuyCount_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(hawkes.BuyCount_ServerToClient(hawkes.NewBuyCount())), nil
			},
		})
		r.Register("hawkes.SellCount", Factory{
			InterfaceID: hawkes.SellCount_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(hawkes.SellCount_ServerToClient(hawkes.NewSellCount())), nil
			},
		})
		r.Register("hawkes.BuyFraction", Factory{
			InterfaceID: hawkes.BuyFraction_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(hawkes.BuyFraction_ServerToClient(hawkes.NewBuyFraction())), nil
			},
		})
		r.Register("hawkes.SellFraction", Factory{
			InterfaceID: hawkes.SellFraction_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(hawkes.SellFraction_ServerToClient(hawkes.NewSellFraction())), nil
			},
		})
		r.Register("hawkes.ArrivalRate", Factory{
			InterfaceID: hawkes.ArrivalRate_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(hawkes.ArrivalRate_ServerToClient(hawkes.NewArrivalRate())), nil
			},
		})
		r.Register("hawkes.BuyRate", Factory{
			InterfaceID: hawkes.BuyRate_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(hawkes.BuyRate_ServerToClient(hawkes.NewBuyRate())), nil
			},
		})
		r.Register("hawkes.SellRate", Factory{
			InterfaceID: hawkes.SellRate_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(hawkes.SellRate_ServerToClient(hawkes.NewSellRate())), nil
			},
		})
		r.Register("hawkes.ConditionalIntensity", Factory{
			InterfaceID: hawkes.ConditionalIntensity_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(hawkes.ConditionalIntensity_ServerToClient(hawkes.NewConditionalIntensity())), nil
			},
		})
		r.Register("hawkes.SpectralRadius", Factory{
			InterfaceID: hawkes.SpectralRadius_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(hawkes.SpectralRadius_ServerToClient(hawkes.NewSpectralRadius())), nil
			},
		})

		// Temporal
		r.Register("temporal.Delay", Factory{
			InterfaceID: temporal.Delay_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(temporal.Delay_ServerToClient(temporal.NewDelay())), nil
			},
		})
		r.Register("temporal.Elapsed", Factory{
			InterfaceID: temporal.Elapsed_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(temporal.Elapsed_ServerToClient(temporal.NewElapsed())), nil
			},
		})
		r.Register("temporal.LogReturns", Factory{
			InterfaceID: temporal.LogReturns_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(temporal.LogReturns_ServerToClient(temporal.NewLogReturns())), nil
			},
		})
		r.Register("temporal.Transition", Factory{
			InterfaceID: temporal.Transition_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(temporal.Transition_ServerToClient(temporal.NewTransition())), nil
			},
		})
		r.Register("temporal.Velocity", Factory{
			InterfaceID: temporal.Velocity_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(temporal.Velocity_ServerToClient(temporal.NewVelocity())), nil
			},
		})

		// Geometry & Algo
		r.Register("geometry.Intersection", Factory{
			InterfaceID: geometry.Intersection_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(geometry.Intersection_ServerToClient(geometry.NewIntersection())), nil
			},
		})
		r.Register("algo.GaussJordan", Factory{
			InterfaceID: algo.GaussJordan_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(algo.GaussJordan_ServerToClient(algo.NewGaussJordan())), nil
			},
		})
		r.Register("algo.HayashiYoshida", Factory{
			InterfaceID: algo.HayashiYoshida_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(algo.HayashiYoshida_ServerToClient(algo.NewHayashiYoshida())), nil
			},
		})
		r.Register("algo.RLS", Factory{
			InterfaceID: algo.RLS_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(algo.RLS_ServerToClient(algo.NewRLS())), nil
			},
		})

		// Cognition
		r.Register("cognition.Associate", Factory{
			InterfaceID: cognition.Associate_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Associate_ServerToClient(cognition.NewAssociate())), nil
			},
		})
		r.Register("cognition.Attractor", Factory{
			InterfaceID: cognition.Attractor_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Attractor_ServerToClient(cognition.NewAttractor())), nil
			},
		})
		r.Register("cognition.Classification", Factory{
			InterfaceID: cognition.Classification_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Classification_ServerToClient(cognition.NewClassification())), nil
			},
		})
		r.Register("cognition.Lookahead", Factory{
			InterfaceID: cognition.Lookahead_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Lookahead_ServerToClient(cognition.NewLookahead())), nil
			},
		})

		// Data
		r.Register("data.Extract", Factory{
			InterfaceID: data.Extract_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(data.Extract_ServerToClient(data.NewExtract())), nil
			},
		})
		r.Register("data.Select", Factory{
			InterfaceID: data.Select_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(data.Select_ServerToClient(data.NewSelect())), nil
			},
		})
		r.Register("data.Quality", Factory{
			InterfaceID: data.Quality_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(data.Quality_ServerToClient(data.NewQuality())), nil
			},
		})
		r.Register("data.Equation", Factory{
			InterfaceID: data.Equation_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(data.Equation_ServerToClient(data.NewEquation())), nil
			},
		})
		r.Register("data.Series", Factory{
			InterfaceID: data.Series_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(data.Series_ServerToClient(data.NewSeries())), nil
			},
		})

		// Execution
		r.Register("execution.Decide", Factory{
			InterfaceID: execution.Decide_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(execution.Decide_ServerToClient(execution.NewDecide())), nil
			},
		})
		r.Register("execution.Gate", Factory{
			InterfaceID: execution.Gate_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(execution.Gate_ServerToClient(execution.NewGate())), nil
			},
		})
		r.Register("execution.Submit", Factory{
			InterfaceID: execution.Submit_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(execution.Submit_ServerToClient(execution.NewSubmit())), nil
			},
		})

		// Associative & Learning
		r.Register("associative.Grid", Factory{
			InterfaceID: associative.Grid_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(associative.Grid_ServerToClient(associative.NewGrid())), nil
			},
		})

		// Cognition
		r.Register("cognition.Associate", Factory{
			InterfaceID: cognition.Associate_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Associate_ServerToClient(cognition.NewAssociate())), nil
			},
		})
		r.Register("cognition.Attractor", Factory{
			InterfaceID: cognition.Attractor_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Attractor_ServerToClient(cognition.NewAttractor())), nil
			},
		})
		r.Register("cognition.BasinKey", Factory{
			InterfaceID: cognition.BasinKey_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.BasinKey_ServerToClient(cognition.NewBasinKey())), nil
			},
		})
		r.Register("cognition.Classification", Factory{
			InterfaceID: cognition.Classification_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Classification_ServerToClient(cognition.NewClassification())), nil
			},
		})
		r.Register("cognition.Lookahead", Factory{
			InterfaceID: cognition.Lookahead_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Lookahead_ServerToClient(cognition.NewLookahead())), nil
			},
		})
		r.Register("cognition.Memory", Factory{
			InterfaceID: cognition.Memory_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Memory_ServerToClient(cognition.NewMemory())), nil
			},
		})
		r.Register("cognition.Pack", Factory{
			InterfaceID: cognition.Pack_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Pack_ServerToClient(cognition.NewPack())), nil
			},
		})
		r.Register("cognition.ParseBasinKey", Factory{
			InterfaceID: cognition.ParseBasinKey_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.ParseBasinKey_ServerToClient(cognition.NewParseBasinKey())), nil
			},
		})
		r.Register("cognition.Reinforce", Factory{
			InterfaceID: cognition.Reinforce_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Reinforce_ServerToClient(cognition.NewReinforce())), nil
			},
		})
		r.Register("cognition.SensoryKey", Factory{
			InterfaceID: cognition.SensoryKey_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.SensoryKey_ServerToClient(cognition.NewSensoryKey())), nil
			},
		})
		r.Register("cognition.Surprisal", Factory{
			InterfaceID: cognition.Surprisal_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Surprisal_ServerToClient(cognition.NewSurprisal())), nil
			},
		})
		r.Register("cognition.Weight", Factory{
			InterfaceID: cognition.Weight_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(cognition.Weight_ServerToClient(cognition.NewWeight())), nil
			},
		})

		// Probability
		r.Register("probability.Concentration", Factory{
			InterfaceID: probability.Concentration_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(probability.Concentration_ServerToClient(probability.NewConcentration())), nil
			},
		})
		r.Register("probability.Distribution", Factory{
			InterfaceID: probability.Distribution_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(probability.Distribution_ServerToClient(probability.NewDistribution())), nil
			},
		})
		r.Register("probability.Entropy", Factory{
			InterfaceID: probability.Entropy_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(probability.Entropy_ServerToClient(probability.NewEntropy())), nil
			},
		})
		r.Register("probability.Geomean", Factory{
			InterfaceID: probability.Geomean_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(probability.Geomean_ServerToClient(probability.NewGeomean())), nil
			},
		})
		r.Register("probability.KolmogorovSmirnov", Factory{
			InterfaceID: probability.KolmogorovSmirnov_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(probability.KolmogorovSmirnov_ServerToClient(probability.NewKolmogorovSmirnov())), nil
			},
		})
		r.Register("probability.ShannonAmbiguity", Factory{
			InterfaceID: probability.ShannonAmbiguity_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(probability.ShannonAmbiguity_ServerToClient(probability.NewShannonAmbiguity())), nil
			},
		})

		// Store
		r.Register("store.Constant", Factory{
			InterfaceID: store.Constant_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(store.Constant_ServerToClient(store.NewConstant())), nil
			},
		})
		r.Register("store.Grid", Factory{
			InterfaceID: store.Grid_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(store.Grid_ServerToClient(store.NewGrid())), nil
			},
		})
		r.Register("store.Key", Factory{
			InterfaceID: store.Key_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(store.Key_ServerToClient(store.NewKey())), nil
			},
		})
		r.Register("store.Radix", Factory{
			InterfaceID: store.Radix_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(store.Radix_ServerToClient(store.NewRadix())), nil
			},
		})

		// Transport
		r.Register("transport.Process", Factory{
			InterfaceID: transport.Process_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(transport.Process_ServerToClient(transport.NewProcess())), nil
			},
		})
		r.Register("transport.Stream", Factory{
			InterfaceID: transport.Stream_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(transport.Stream_ServerToClient(transport.NewStream())), nil
			},
		})

		// UI
		r.Register("ui.HTTPServer", Factory{
			InterfaceID: ui.HTTPServer_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(ui.HTTPServer_ServerToClient(ui.NewHTTPServer())), nil
			},
		})

		// Boundary primitives
		r.Register("data.Source", Factory{
			InterfaceID: 0,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client{}, nil
			},
		})
		r.Register("source", Factory{
			InterfaceID: 0,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client{}, nil
			},
		})
		r.Register("data.Sink", Factory{
			InterfaceID: 0,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client{}, nil
			},
		})
		r.Register("sink", Factory{
			InterfaceID: 0,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client{}, nil
			},
		})

		// Test sources
		r.Register("test.Float64Source", Factory{
			InterfaceID: calculus.Floor_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Floor_ServerToClient(&passthroughSource{})), nil
			},
		})

		defaultRegistry = r
	})

	return defaultRegistry
}

type passthroughSource struct {
	out float64
}

func (s *passthroughSource) Write(ctx context.Context, call calculus.Floor_write) error {
	s.out = call.Args().In()
	return nil
}

func (s *passthroughSource) Done(ctx context.Context, call calculus.Floor_done) error {
	res, err := call.AllocResults()
	if err != nil {
		return err
	}
	res.SetOut(s.out)
	s.out = 0
	return nil
}
