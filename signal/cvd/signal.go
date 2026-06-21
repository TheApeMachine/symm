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
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
)

/*
Signal measures cumulative volume delta flow and classifies trade pressure regimes.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriteCloser
	tree        *dmt.Tree
}

/*
NewSignal composes the CVD flow pipeline for tree replay measurement.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	flow := equation.NewFlow()

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		algo: nomagique.Number(
			flow,
			probability.NewClassifier(
				datura.Acquire("cvd-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"absorption", "drive", "balance", "starvation"},
				}),
			),
		),
	}

	return signal
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	scope, _ := datapoint.Scope()

	state := datura.Acquire(
		"pumpdump", datura.APPJSON,
	).WithRole(
		"measurement",
	).WithScope(
		scope,
	).WithPayload(
		datapoint.DecryptPayload(),
	)

	if errnie.Error(transport.NewFlipFlop(
		state, signal.algo,
	)) != nil {
		state.Release()
		return nil
	}

	return state
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
