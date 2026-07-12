package trader

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/ohlc"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/types"
)

/*
Signal wires every market signal used by the trader runtime.
*/
type Signal struct {
	CrossSection *types.CrossSection
	Ticker       []types.Signal[any]
	Trade        []types.Signal[any]
	Book         []types.Signal[any]
	OHLC         []types.Signal[any]
	Level3       []types.Signal[any]
}

/*
NewSignal constructs the full signal set for one runtime.
*/
func NewSignal(ctx context.Context) *Signal {
	crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			err.Error(),
			err,
		))
	}

	fluidSignal := fluid.NewSignal[any](ctx)
	depthflowSignal := depthflow.NewSignal[any](ctx)
	exhaustSignal := exhaust.NewSignal[any](ctx)
	toxicitySignal := toxicity.NewSignal[any](ctx)

	return &Signal{
		CrossSection: crossSection,
		Ticker: []types.Signal[any]{
			correlation.NewSignal[any](ctx),
			fluidSignal,
			leadlag.NewSignal[any](ctx),
			liquidity.NewSignal[any](ctx),
			pumpdump.NewSignal[any](ctx),
			sentiment.NewSignal[any](ctx),
		},
		Trade: []types.Signal[any]{
			cvd.NewSignal[any](ctx),
			depthflowSignal,
			exhaustSignal,
			fluidSignal,
			hawkes.NewSignal[any](ctx),
		},
		Book: []types.Signal[any]{
			depthflowSignal,
			exhaustSignal,
			fluidSignal,
		},
		OHLC: []types.Signal[any]{
			ohlc.NewSignal[any](ctx),
		},
		Level3: []types.Signal[any]{
			toxicitySignal,
		},
	}
}
