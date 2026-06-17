package cvd

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	feed "github.com/theapemachine/symm/signal"
)

type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	surpriseTree *dmt.Tree
	trade        *feed.Trade
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
	surpriseTree, _ := dmt.NewTree("")

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		surpriseTree: surpriseTree,
		algo: nomagique.Number(
			flow,
			probability.NewClassifier(
				flow.AbsorptionReading(),
				flow.DriveReading(),
				flow.BalanceReading(),
				flow.StarvationReading(),
			),
			probability.NewDMTSurprise(
				surpriseTree,
				5,
			),
		),
		trade: feed.NewTrade(ctx),
	}

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "trade":
		signal.trade.Update(artifact)
	case "measurement":
		signal.Measure(artifact)
	}

	return nil
}

func (signal *Signal) Measure(in *datura.Artifact) (logic.Measurement, error) {
	scope := datura.Peek[string](in, "scope")
	signal.trade.Scope = scope
	signal.trade.WireProfile = feed.TradeWireFlow
	signal.trade.ResetReadHead()

	out := datura.Acquire("cvd-out", datura.Artifact_Type_json).WithScope(scope)

	if out == nil {
		return logic.Measurement{}, nil
	}

	errnie.Does(func() (int64, error) {
		return transport.Copy(signal.algo, signal.trade)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.IO, "failed to copy to algo", err))
	})

	if err := transport.NewFlipFlop(out, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	snapshot := signal.trade.Snapshot(scope)
	position := logic.PositionTypeNone

	if snapshot.Net > 0 {
		position = logic.PositionTypeLong
	}

	if snapshot.Net < 0 {
		position = logic.PositionTypeShort
	}

	categoryIndex := datura.Peek[int](out, "classifier.category")

	if categoryIndex == 0 {
		return logic.Measurement{}, nil
	}

	confidence := datura.Peek[float64](out, "classifier.confidence")

	if !logic.ScalarFinite(confidence) || confidence <= 0 {
		return logic.Measurement{}, nil
	}

	strength := datura.Peek[float64](out, "flow.net_fraction")

	if categoryIndex == 3 {
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
		Category:   cvdCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   position,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](out, "transition.surprise"),
		ObservedAt: snapshot.Observed,
	}.UnlessPublishable(), nil
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

	if signal.surpriseTree != nil {
		_ = signal.surpriseTree.Close()
	}

	return nil
}
