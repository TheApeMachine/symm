package execution

import (
	"context"
	"fmt"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* venueOrders is the native venue boundary; SDK transport has its own HTTP proof. */
type venueOrders struct {
	submitted                   int
	uncertain                   bool
	identity, side              string
	status, quantity, cost, fee string
}

func (*venueOrders) Write(context.Context, kraken.Orders_write) error { return nil }
func (*venueOrders) Done(context.Context, kraken.Orders_done) error   { return nil }
func (venue *venueOrders) Submit(ctx context.Context, call kraken.Orders_submit) error {
	request, err := call.Args().Request()
	if err != nil {
		return err
	}
	venue.identity, err = request.ClientId()
	if err != nil {
		return err
	}
	venue.side, err = request.Side()
	if err != nil {
		return err
	}
	venue.submitted++
	venue.status, venue.quantity, venue.cost, venue.fee = "open", "0", "0", "0"
	if venue.uncertain {
		return fmt.Errorf("fixture submit response lost after acceptance")
	}
	result, err := call.AllocResults()
	if err != nil {
		return err
	}
	return result.SetId("VENUE-001")
}
func (venue *venueOrders) Inspect(ctx context.Context, call kraken.Orders_inspect) error {
	result, err := call.AllocResults()
	if err != nil {
		return err
	}
	order, err := result.NewOrder()
	if err != nil {
		return err
	}
	return venue.emit(order)
}
func (venue *venueOrders) Find(ctx context.Context, call kraken.Orders_find) error {
	result, err := call.AllocResults()
	if err != nil {
		return err
	}
	result.SetFound(venue.submitted > 0)
	order, err := result.NewOrder()
	if err != nil {
		return err
	}
	return venue.emit(order)
}
func (*venueOrders) Cancel(context.Context, kraken.Orders_cancel) error {
	return fmt.Errorf("fixture expects inventory exit via sell")
}
func (venue *venueOrders) emit(order kraken.Execution) error {
	for _, err := range []error{order.SetId("VENUE-001"), order.SetClientId(venue.identity), order.SetSymbol("BTCUSD"), order.SetSide(venue.side), order.SetStatus(venue.status), order.SetQuantity(venue.quantity), order.SetCost(venue.cost), order.SetFee(venue.fee), order.SetAveragePrice("99.99")} {
		if err != nil {
			return err
		}
	}
	return nil
}

/* checkpointFixture retains immutable checkpoint bytes and can hold acknowledgement. */
type checkpointFixture struct {
	mu   sync.Mutex
	data []byte
	gate chan struct{}
}

func (checkpoint *checkpointFixture) Load(ctx context.Context, call runtime.Checkpoint_load) error {
	checkpoint.mu.Lock()
	defer checkpoint.mu.Unlock()
	result, err := call.AllocResults()
	if err != nil {
		return err
	}
	return result.SetData(checkpoint.data)
}
func (checkpoint *checkpointFixture) Save(ctx context.Context, call runtime.Checkpoint_save) error {
	if checkpoint.gate != nil {
		select {
		case <-checkpoint.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	encoded, err := call.Args().Data()
	if err != nil {
		return err
	}
	checkpoint.mu.Lock()
	defer checkpoint.mu.Unlock()
	checkpoint.data = append([]byte(nil), encoded...)
	return nil
}
func (*checkpointFixture) Flush(context.Context, runtime.Durable_flush) error { return nil }

func (replay *accountReplay) fence() error {
	future, release := runtime.Durable(replay.account).Flush(replay.ctx, nil)
	defer release()
	_, err := future.Struct()
	return err
}

func TestAccountReconcile(t *testing.T) {
	Convey("Live permission gates new risk while one ledger reconciles exact venue fills", t, func() {
		replay := newAccountReplay(t)
		venue := &venueOrders{}
		replay.live, replay.durable = true, true
		replay.orders = kraken.Orders_ServerToClient(venue)
		checkpoint := &checkpointFixture{gate: make(chan struct{})}
		replay.checkpoint = runtime.Checkpoint_ServerToClient(checkpoint)
		closedGate := false
		t.Cleanup(func() {
			if !closedGate {
				close(checkpoint.gate)
			}
		})
		state, release, err := replay.step("BTC/USD", 99, 100, true, "ENTER", "")
		So(err, ShouldBeNil)
		release()
		So(venue.submitted, ShouldEqual, 0)
		replay.authorized = true
		state, release, err = replay.step("BTC/USD", 99, 100, true, "ENTER", "")
		So(err, ShouldBeNil)
		release()
		So(venue.submitted, ShouldEqual, 0)
		// A blocked disk acknowledgement cannot stall the next market observation.
		state, release, err = replay.step("BTC/USD", 99, 100, true, "ENTER", "")
		So(err, ShouldBeNil)
		So(state.Observations(), ShouldEqual, 2)
		release()
		close(checkpoint.gate)
		closedGate = true
		So(replay.fence(), ShouldBeNil)
		state, release, err = replay.step("BTC/USD", 99, 100, true, "ENTER", "")
		So(err, ShouldBeNil)
		release()
		So(venue.submitted, ShouldEqual, 0)
		So(replay.fence(), ShouldBeNil)
		state, release, err = replay.step("BTC/USD", 99, 100, true, "WAIT", "WAIT")
		So(err, ShouldBeNil)
		release()
		So(venue.submitted, ShouldEqual, 1)
		venue.quantity, venue.cost, venue.fee = "0.0004", "0.04000001", "0.00032000008"
		state, release, err = replay.step("BTC/USD", 99, 100, true, "WAIT", "WAIT")
		So(err, ShouldBeNil)
		positions, err := state.Positions()
		So(err, ShouldBeNil)
		first, err := positions.At(0).Basis()
		So(err, ShouldBeNil)
		So(first, ShouldEqualMoney, "0.04032001008")
		release()
		state, release, err = replay.step("BTC/USD", 99, 100, true, "WAIT", "WAIT")
		So(err, ShouldBeNil)
		positions, err = state.Positions()
		So(err, ShouldBeNil)
		repeated, err := positions.At(0).Basis()
		So(err, ShouldBeNil)
		So(repeated, ShouldEqualMoney, first)
		release()
		venue.quantity, venue.cost, venue.fee, venue.status = "0.001", "0.10000001", "0.00080000008", "closed"
		state, release, err = replay.step("BTC/USD", 99, 100, true, "WAIT", "WAIT")
		So(err, ShouldBeNil)
		So(state.Open(), ShouldEqual, 1)
		release()
		replay.authorized, replay.durable = false, false
		state, release, err = replay.step("BTC/USD", 110, 111, true, "WAIT", "EXIT")
		So(err, ShouldBeNil)
		release()
		state, release, err = replay.step("BTC/USD", 110, 111, true, "WAIT", "EXIT")
		So(err, ShouldBeNil)
		release()
		So(venue.submitted, ShouldEqual, 2)
		venue.quantity, venue.cost, venue.fee, venue.status = "0.001", "0.11000000", "0.00088000", "closed"
		state, release, err = replay.step("BTC/USD", 110, 111, true, "WAIT", "WAIT")
		So(err, ShouldBeNil)
		So(state.Open(), ShouldEqual, 0)
		So(state.Outcomes(), ShouldEqual, 1)
		trip, err := state.Closed()
		So(err, ShouldBeNil)
		pnl, err := trip.Pnl()
		So(err, ShouldBeNil)
		So(pnl, ShouldEqualMoney, "0.00831998992")
		release()
		So(replay.fence(), ShouldBeNil)
	})
}
