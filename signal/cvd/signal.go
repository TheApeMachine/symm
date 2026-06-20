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
				datura.Acquire("cvd-classifier", datura.APPJSON).Poke(
					[]string{"absorption", "drive", "balance", "starvation"},
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

	result := datura.Acquire("cvd", datura.APPJSON)
	result.WithRole("measurement")
	result.WithScope("cvd")

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
