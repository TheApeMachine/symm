package hawkes

import (
	"context"
	"io"
	"math"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
)

/*
Signal measures trade-cluster self-excitation and Hawkes thermal clustering.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	tree        *dmt.Tree
}

/*
NewSignal composes the Hawkes excitation pipeline for tree replay measurement.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	excitation := algorithm.NewExcitation(
		datura.Acquire("hawkes-excitation", datura.APPJSON),
	)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		algo: nomagique.Number(
			algorithm.NewTradeExcitationSample(
				datura.Acquire("hawkes-trade", datura.APPJSON),
			),
			excitation,
			probability.NewClassifier(
				datura.Acquire("hawkes-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"frenzy", "saturation", "organic", "exhaustion"},
				}),
			),
		),
	}

	return signal
}

func (signal *Signal) IngestRoles() []string {
	return []string{"trade"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.algo == nil {
		return nil
	}

	channel := datura.Peek[string](datapoint, "channel")

	if channel != "" && channel != "trade" {
		return nil
	}

	if errnie.Error(transport.NewFlipFlop(
		datapoint, signal.algo,
	)) != nil {
		return nil
	}

	confidence := datura.Peek[float64](datapoint, "output", "confidence")
	uniformConfidence := 1.0 / 4.0

	if confidence <= 0 ||
		math.IsNaN(confidence) ||
		math.IsInf(confidence, 0) ||
		confidence <= uniformConfidence+1e-12 {
		return nil
	}

	return datapoint
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
