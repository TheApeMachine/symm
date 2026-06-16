package cvd

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

/*
Signal measures cumulative volume delta from executed trade flow.

| Category            | Net Fraction | Price vs Flow | Market "Feel"           |
|---------------------|--------------|---------------|-------------------------|
| Hidden Absorption   | High         | Divergent     | Iceberg / Passive Depth |
| Aggressive Drive    | High         | Aligned       | Steamroller / Trend     |
| Stochastic Balance  | Low          | Mixed         | Two-Sided / Choppy      |
| Volume Starvation   | N/A          | Thin          | No Flow / Idle          |
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	flow        *algorithm.Flow
	trade       *Trade
}

/*
NewSignal composes the CVD pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	flow := algorithm.NewFlow()

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		flow:        flow,
		algo: nomagique.Number(
			flow,
			probability.NewClassifier(
				flow.AbsorptionReading(),
				flow.DriveReading(),
				flow.BalanceReading(),
				flow.StarvationReading(),
			),
			probability.NewTransitionSurprise(5, 1.0/float64(feedRingCapacity)),
		),
		trade: NewTrade(ctx),
	}

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch artifact.Peek("role") {
	case "trade":
		signal.trade.Update(
			datura.As[krakenmarket.TradeUpdates](artifact),
		)
	case "measurement":
		signal.Measure(artifact)
	}

	return nil
}

func (signal *Signal) Measure(in *datura.Artifact) (logic.Measurement, error) {
	scope := in.Peek("scope")
	signal.trade.scope = scope

	frame := make([]byte, 8192)

	n, readErr := signal.trade.Read(frame)

	if n == 0 {
		return logic.Measurement{}, nil
	}

	if readErr != nil && readErr != io.EOF {
		return logic.Measurement{}, readErr
	}

	if _, err := signal.algo.Write(frame[:n]); err != nil {
		return logic.Measurement{}, err
	}

	out := datura.Acquire("cvd-out", datura.Artifact_Type_json)

	n, err := signal.algo.Read(frame)

	if err != nil && err != io.EOF {
		return logic.Measurement{}, err
	}

	if _, err := out.Write(frame[:n]); err != nil {
		return logic.Measurement{}, err
	}

	snapshot := signal.trade.Snapshot(scope)
	position := logic.PositionTypeNone

	if snapshot.Net > 0 {
		position = logic.PositionTypeLong
	}

	if snapshot.Net < 0 {
		position = logic.PositionTypeShort
	}

	strength := datura.Peek[float64](out, "flow.net_fraction")

	if datura.Peek[int](out, "classifier.category") == 3 {
		strength = datura.Peek[float64](out, "flow.balance")
	}

	return logic.Measurement{
		Source:     logic.SourceCVD,
		Symbol:     scope,
		Price:      snapshot.Price,
		Strength:   strength,
		Volume:     snapshot.Volume,
		Spread:     0,
		Elapsed:    snapshot.Elapsed,
		Category:   cvdCategory(datura.Peek[int](out, "classifier.category")),
		Regime:     logic.RegimeTypeNone,
		Position:   position,
		Confidence: datura.Peek[float64](out, "classifier.confidence"),
		Surprise:   datura.Peek[float64](out, "transition.surprise"),
		ObservedAt: snapshot.Observed,
	}, nil
}

func cvdCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryHiddenAbsorption
	case 2:
		return logic.CategoryAggressiveDrive
	case 3:
		return logic.CategoryStochasticBalance
	case 4:
		return logic.CategoryVolumeStarvation
	default:
		return logic.CategoryStochasticBalance
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
