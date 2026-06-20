package hawkes

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

	var frame *datura.Artifact

	samples := 0

	for _, role := range []string{"ticker", "book", "trade", "ohlc"} {
		probe := datura.Acquire("trader", datura.APPJSON)
		probe.WithRole(role)
		probe.WithScope(scope)

		for stored := range signal.tree.Seek(probe.Prefix("role", "scope")) {
			packed, err := stored.Message().MarshalPacked()

			stored.Release()

			if errnie.Error(err) != nil {
				continue
			}

			replay := datura.Acquire("trader", datura.APPJSON)

			if _, err := replay.Write(packed); errnie.Error(err) != nil {
				replay.Release()
				continue
			}

			errnie.Error(transport.NewFlipFlop(replay, signal.algo))
			samples++

			if frame != nil {
				frame.Release()
			}

			frame = replay
		}

		probe.Release()
	}

	result := datura.Acquire("hawkes", datura.APPJSON)
	result.WithRole("measurement")
	result.WithScope("hawkes")

	if frame == nil || samples == 0 {
		result.WithPayload([]byte("{}"))
		return result
	}

	payload := frame.DecryptPayload()

	frame.Release()

	if len(payload) == 0 {
		result.WithPayload([]byte("{}"))
		return result
	}

	result.WithPayload(payload)
	result.Merge("samples", float64(samples))
	result.Merge("calibrated", true)

	return result
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
