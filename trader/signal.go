package trader

import (
	"context"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/pumpdump"
)

type wiredSignal struct {
	measurer market.Signal
	origin   logic.SourceType
}

type MeasureStats struct {
	Rows         int
	Measurements int
	Calibrating  int
}

/*
Signal runs every spectrum signal against the ingest roles it declares.
Each signal advances its own tree cursor so Measure only replays new frames.
*/
type Signal struct {
	ctx            context.Context
	cancel         context.CancelFunc
	pool           *qpool.Q[any]
	tree           *dmt.Tree
	signals        []wiredSignal
	measureCursors sync.Map
}

func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		tree:   tree,
		signals: []wiredSignal{
			{pumpdump.NewSignal(ctx, pool, tree), logic.SourcePumpDump},
		},
	}
}

func (signal *Signal) Measure() []*datura.Artifact {
	measurements := make([]*datura.Artifact, 0)

	for _, wired := range signal.signals {
		for _, role := range wired.measurer.IngestRoles() {
			for artifact := range signal.tree.Seek([]byte(role + "/update")) {
				measurement := wired.measurer.Measure(artifact)

				if measurement != nil {
					measurement.WithRole("measurement")
					_ = measurement.SetOrigin(string(wired.origin))
					measurements = append(measurements, measurement)
				}
			}
		}
	}

	return measurements
}

func (signal *Signal) Close() error {
	signal.cancel()

	for _, wired := range signal.signals {
		wired.measurer.Close()
	}

	return nil
}
