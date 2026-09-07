package cmd

import (
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

func TestAccountFundsReserve(t *testing.T) {
	Convey("Given finite authoritative cash and concurrent placement claims", t, func() {
		funds := accountFunds{}
		at := time.Unix(100, 0)
		source := &types.EquityReading{At: at, From: at, Version: 1, Cash: "150", AvailableCash: "150", Equity: "150", NetFunding: "0", Complete: true}
		funds.Observe(source)
		So(funds.Reserve("first", big.NewRat(100, 1)), ShouldBeTrue)
		So(funds.Reserve("second", big.NewRat(100, 1)), ShouldBeFalse)
		So(funds.Observe(source).Cash, ShouldEqual, "50")
		Convey("Stale balance replies cannot release a completed reservation", func() {
			funds.Release("first", at.Add(time.Second))
			stale := *source
			stale.Version++
			stale.At = at.Add(2 * time.Second)
			So(funds.Observe(&stale).Cash, ShouldEqual, "50")
			fresh := stale
			fresh.Version++
			fresh.From = at.Add(2 * time.Second)
			fresh.Cash, fresh.AvailableCash = "50", "50"
			So(funds.Observe(&fresh).Cash, ShouldEqual, "50")
			So(funds.Observe(&fresh).Committed, ShouldEqual, "0")
		})
		Convey("Exchange holds remain unavailable independently of local reservations", func() {
			funds.Release("first", time.Time{})
			held := *source
			held.Version++
			held.AvailableCash = "70"
			So(funds.Observe(&held).Committed, ShouldEqual, "80")
			So(funds.Reserve("too-much", big.NewRat(100, 1)), ShouldBeFalse)
			So(funds.Reserve("fundable", big.NewRat(60, 1)), ShouldBeTrue)
			So(funds.Observe(&held).Cash, ShouldEqual, "10")
		})
		Convey("A proven pre-submission refusal releases its commitment", func() {
			funds.Release("first", time.Time{})
			So(funds.Observe(source).Cash, ShouldEqual, "150")
		})
		Convey("Concurrent check-and-reserve cannot commit the same dollar twice", func() {
			funds.Release("first", time.Time{})
			var accepted atomic.Uint64
			var workers sync.WaitGroup
			for _, identity := range []string{"one", "two", "three", "four"} {
				workers.Go(func() {
					if funds.Reserve(identity, big.NewRat(100, 1)) {
						accepted.Add(1)
					}
				})
			}
			workers.Wait()
			So(accepted.Load(), ShouldEqual, 1)
		})
	})
}

func TestAccountFundsPublish(t *testing.T) {
	Convey("Given an authoritative mark with malformed venue numbers", t, func() {
		for _, field := range []string{"equity", "funding", "available", "cash"} {
			Convey("Reject "+field+" and recover only on a later valid mark", func() {
				funds := &accountFunds{}
				reading := &types.EquityReading{Version: 1, Cash: "150", AvailableCash: "150",
					Equity: "150", NetFunding: "0", Complete: true}

				switch field {
				case "equity":
					reading.Equity = "invalid"
				case "funding":
					reading.NetFunding = "invalid"
				case "available":
					reading.AvailableCash = "invalid"
				case "cash":
					reading.Cash = "invalid"
				}

				state := funds.Observe(reading)
				So(state.Complete, ShouldBeFalse)
				So(state.Reason, ShouldNotBeEmpty)
				So(funds.Reserve("blocked", big.NewRat(1, 1)), ShouldBeFalse)
				reading = &types.EquityReading{Version: 2, Cash: "150", AvailableCash: "150",
					Equity: "150", NetFunding: "0", Complete: true}
				So(funds.Observe(reading).Complete, ShouldBeTrue)
				So(funds.Reserve("valid", big.NewRat(1, 1)), ShouldBeTrue)
			})
		}
	})
}

func TestAccountFundsReject(t *testing.T) {
	Convey("Given a corrupted cash state held by an existing reader", t, func() {
		funds := &accountFunds{}
		previous := &strategy.AccountState{Complete: true, Cash: "invalid"}
		funds.state.Store(previous)
		So(funds.Reserve("blocked", big.NewRat(1, 1)), ShouldBeFalse)
		So(previous.Complete, ShouldBeTrue)
		So(funds.state.Load().Complete, ShouldBeFalse)
		So(funds.state.Load().Reason, ShouldEqual, "funds: invalid authoritative cash")
	})
}

func BenchmarkAccountFundsObserve(b *testing.B) {
	funds := accountFunds{}
	source := &types.EquityReading{At: time.Unix(100, 0), Version: 1, Cash: "150", AvailableCash: "150", Equity: "150", NetFunding: "0", Complete: true}
	funds.Observe(source)
	b.ReportAllocs()
	for b.Loop() {
		funds.Observe(source)
	}
}

func BenchmarkAccountFundsPublish(b *testing.B) {
	funds := accountFunds{reservations: map[string]cashReservation{
		"pending": {cost: big.NewRat(10, 1)},
	}}
	source := &types.EquityReading{At: time.Unix(100, 0), Version: 1,
		Cash: "150", AvailableCash: "140", Equity: "150", NetFunding: "0", Complete: true}
	b.ReportAllocs()

	for b.Loop() {
		funds.publish(source)
	}
}
