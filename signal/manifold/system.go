package manifold

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/signal/compute"
)

type System struct {
	base   *signal.System
	field  *Field
	worker *compute.BatchWorker
}

const manifoldBatchCapacity = 8192

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	field, err := newField()

	if errnie.Error(err) != nil {
		return nil
	}

	system := &System{field: field}

	base := signal.NewSystem(
		ctx,
		pool,
		logic.SourceManifold,
		func(symbol string, entity *logic.Entity) market.Signal {
			return NewSignal(symbol, entity, system)
		},
		logic.EntityTrade,
		logic.EntityTick,
		logic.EntityBook,
	)

	if base == nil {
		field.Close()
		return nil
	}

	system.base = base
	system.worker = compute.NewBatchWorker(
		ctx,
		manifoldBatchCapacity,
		signal.ResolveComputeBatchInterval(),
	)
	system.field.bindWorker(system.worker)
	system.base.OnSymbols(system.field.RegisterSymbols)
	system.base.OnBook(system.ingestFuturesBook)
	system.field.SetSnapshotPublisher(system.publishSnapshot)

	return system
}

/*
Tick runs the shared signal bus loop. Field snapshots publish from market feeds.
*/
func (system *System) Tick() error {
	return system.base.Tick()
}

func (system *System) Close() error {
	if system.worker != nil {
		system.worker.Close()
	}

	system.field.Close()

	return system.base.Close()
}

func (system *System) ingestFuturesBook(
	book *krakenmarket.BookUpdate,
) (handled bool, err error) {
	if book == nil {
		return false, nil
	}

	if _, identityErr := krakenmarket.FuturesIdentityFromProduct(book.Symbol); identityErr != nil {
		return false, nil
	}

	eventAt := book.Timestamp

	if eventAt.IsZero() {
		errnie.Debug(fmt.Sprintf(
			"manifold: futures book %q missing timestamp, using synthetic time",
			book.Symbol,
		))
		eventAt = time.Now()
	}

	return true, system.field.enqueueFuturesBook(*book, eventAt)
}
