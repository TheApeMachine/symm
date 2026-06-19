package manifold

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
		"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	symmsignal "github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/signal/compute"
)

const manifoldBatchCapacity = 8192

/*
Signal classifies the 3D manifold state for one symbol.

PressureGradNorm captures cross-axis basis and beta dislocations.
CoherenceMag2 captures systemic herding / superfluid collapse.
GuidanceSpeed is the pilot-wave trend velocity from aligned Ψ.
ViscosityProxy inverts divergence — laminar when large, turbulent when small.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	tree        *dmt.Tree
	field       *Field
	serial      *compute.SerialPool
}

/*
NewSignal composes the manifold-state pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	field := errnie.Does(func() (*Field, error) {
		return NewField()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"manifold: failed to create field",
			err,
		))
	}).Value()

	serial := compute.NewSerialPool(
		ctx,
		manifoldBatchCapacity,
		100*time.Millisecond,
	)

	if field != nil {
		field.bindSerial(serial)
	}

	manifoldstate := algorithm.NewManifoldstate()

	return &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		field:       field,
		serial:      serial,
		algo: nomagique.Number(
			manifoldstate,
			probability.NewClassifier(
				datura.Acquire("manifold-classifier", datura.APPJSON).Poke(
					[]string{"herdScore", "shockScore", "driftScore", "noiseScore"},
					"inputs",
				),
			),
		),
	}
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

	symmsignal.PublishMeasurement(signal.tree, "manifold", query)

	return query
}

/*
FieldSnapshot builds the manifold dashboard payload from the last integrated field.
*/
func (signal *Signal) FieldSnapshot(eventAt time.Time) (map[string]any, error) {
	if signal == nil || signal.field == nil {
		return nil, nil
	}

	if !signal.field.hasPublishableSnapshot() {
		return nil, nil
	}

	return signal.field.snapshotPayload(eventAt)
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	if signal.serial != nil {
		signal.serial.Close()
	}

	if signal.field != nil {
		signal.field.Close()
	}

	return err
}
