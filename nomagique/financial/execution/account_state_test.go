package execution

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestAccountRestore(t *testing.T) {
	Convey("An uncertain live submit survives restart and reconciles without duplicate risk", t, func() {
		replay := newAccountReplay(t)
		venue := &venueOrders{uncertain: true}
		replay.live, replay.authorized, replay.durable = true, true, true
		replay.orders = kraken.Orders_ServerToClient(venue)
		replay.checkpoint = runtime.Checkpoint_ServerToClient(&checkpointFixture{})
		_, release, err := replay.step("BTC/USD", 99, 100, true, "WAIT", "WAIT")
		So(err, ShouldBeNil)
		release()
		So(replay.fence(), ShouldBeNil)
		_, release, err = replay.step("BTC/USD", 99, 100, true, "ENTER", "WAIT")
		So(err, ShouldBeNil)
		release()
		So(replay.fence(), ShouldBeNil)
		state, release, err := replay.step("BTC/USD", 99, 100, true, "WAIT", "WAIT")
		So(err, ShouldBeNil)
		reason, err := state.Reason()
		So(err, ShouldBeNil)
		So(reason, ShouldContainSubstring, "response lost")
		release()
		So(venue.submitted, ShouldEqual, 1)
		future, releaseSnapshot := runtime.Snapshot(replay.account).Snapshot(replay.ctx, nil)
		defer releaseSnapshot()
		snapshot, err := future.Struct()
		So(err, ShouldBeNil)
		encoded, err := snapshot.Data()
		So(err, ShouldBeNil)
		replacement := Account_ServerToClient(NewAccount())
		restored, releaseRestore := runtime.Snapshot(replacement).Restore(replay.ctx, func(params runtime.Snapshot_restore_Params) error { return params.SetData(encoded) })
		_, err = restored.Struct()
		So(err, ShouldBeNil)
		releaseRestore()
		So(replay.fence(), ShouldBeNil)
		replay.account.Release()
		replay.account = replacement
		// New-risk permission can disappear during the restart; reconciliation remains active.
		replay.authorized, replay.durable = false, false
		_, release, err = replay.step("BTC/USD", 99, 100, true, "WAIT", "WAIT")
		So(err, ShouldBeNil)
		release()
		So(venue.submitted, ShouldEqual, 1)
		venue.status, venue.quantity, venue.cost, venue.fee = "closed", "0.001", "0.10000001", "0.00080000008"
		state, release, err = replay.step("BTC/USD", 99, 100, true, "WAIT", "WAIT")
		So(err, ShouldBeNil)
		So(state.Open(), ShouldEqual, 1)
		positions, err := state.Positions()
		So(err, ShouldBeNil)
		basis, err := positions.At(0).Basis()
		So(err, ShouldBeNil)
		So(basis, ShouldEqualMoney, "0.10080001008")
		release()
		So(venue.submitted, ShouldEqual, 1)
		So(replay.fence(), ShouldBeNil)
	})
	Convey("A fresh live node restores its durable ledger before handling the current market", t, func() {
		replay := newAccountReplay(t)
		venue := &venueOrders{}
		replay.live, replay.authorized, replay.durable = true, true, true
		replay.orders = kraken.Orders_ServerToClient(venue)
		replay.checkpoint = runtime.Checkpoint_ServerToClient(&checkpointFixture{})
		for _, decision := range []string{"WAIT", "ENTER", "WAIT"} {
			_, release, err := replay.step("BTC/USD", 99, 100, true, decision, "WAIT")
			So(err, ShouldBeNil)
			release()
			So(replay.fence(), ShouldBeNil)
		}
		So(venue.submitted, ShouldEqual, 1)
		replay.account.Release()
		replay.account = Account_ServerToClient(NewAccount())
		replay.authorized, replay.durable = false, false
		venue.quantity, venue.cost, venue.fee, venue.status = "0.001", "0.10000001", "0.00080000008", "closed"
		state, release, err := replay.step("BTC/USD", 99, 100, true, "WAIT", "WAIT")
		So(err, ShouldBeNil)
		defer release()
		So(state.Open(), ShouldEqual, 1)
		symbol, err := state.Symbol()
		So(err, ShouldBeNil)
		So(symbol, ShouldEqual, "BTC/USD")
		So(venue.submitted, ShouldEqual, 1)
		So(replay.fence(), ShouldBeNil)
	})

}
