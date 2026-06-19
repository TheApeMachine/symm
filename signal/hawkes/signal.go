package hawkes

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	symmsignal "github.com/theapemachine/symm/signal"
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

	excitation := algorithm.NewExcitation()

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		algo: nomagique.Number(
			excitation,
			probability.NewClassifier(
				datura.Acquire("hawkes-classifier", datura.APPJSON).Poke(
					[]string{"frenzy", "saturation", "organic", "exhaustion"},
					"inputs",
				),
			),
		),
	}

	return signal
}

func (signal *Signal) Measure(query *datura.Artifact) *datura.Artifact {
	scope, _ := query.Scope()

	if scope == "" {
		return nil
	}

	symmsignal.ReplayScopeIngest(signal.tree, scope, query, signal.algo)

	if datura.Peek[int](query, "classifier", "category") <= 0 {
		return nil
	}

	symmsignal.PublishMeasurement(signal.tree, "hawkes", query)

	return query
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
