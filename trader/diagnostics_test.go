package trader

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCryptoDiagnostics(t *testing.T) {
	Convey("Given a crypto trader with no frames yet", t, func() {
		crypto := diagnosticCrypto()

		Convey("It should report a flowing empty measurement plane", func() {
			snapshot := crypto.Diagnostics()

			So(snapshot.Status, ShouldEqual, "flowing")
			So(snapshot.Lanes, ShouldBeEmpty)
			So(snapshot.Tickers, ShouldEqual, uint64(0))
			So(snapshot.Trades, ShouldEqual, uint64(0))
			So(snapshot.Level3, ShouldEqual, uint64(0))
			So(len(snapshot.Stages), ShouldBeGreaterThan, 10)
			So(snapshot.Stages[0].Name, ShouldEqual, "price")
			So(snapshot.Stages[2].Name, ShouldEqual, "crypto")
			So(snapshot.Hops[0].From, ShouldEqual, "price")
			So(snapshot.Hops[0].To, ShouldEqual, "crypto")
		})
	})

	Convey("Given observed price and measurement clocks", t, func() {
		crypto := diagnosticCrypto()
		crypto.tickers.Store(4)
		crypto.trades.Store(2)
		crypto.level3.Store(1)
		crypto.clocks.observe("price", time.Millisecond)
		crypto.clocks.observeHop("price", "crypto", time.Millisecond)
		crypto.clocks.observe("measurements", 2*time.Millisecond)
		crypto.clocks.observe("resonance", 3*time.Millisecond)
		crypto.clocks.observeHop("crypto", "measurements", time.Millisecond)

		Convey("It should publish those accumulators on the diagnostics wire", func() {
			snapshot := crypto.Diagnostics()
			price := snapshot.Stages[0]
			measurements := stageNamed(snapshot.Stages, "measurements")
			resonance := stageNamed(snapshot.Stages, "resonance")
			hop := hopNamed(snapshot.Hops, "price", "crypto")

			So(snapshot.Tickers, ShouldEqual, uint64(4))
			So(snapshot.Trades, ShouldEqual, uint64(2))
			So(snapshot.Level3, ShouldEqual, uint64(1))
			So(price.Name, ShouldEqual, "price")
			So(price.Count, ShouldEqual, uint64(1))
			So(price.LastNs, ShouldBeGreaterThan, uint64(0))
			So(price.LastAtNs, ShouldBeGreaterThan, int64(0))
			So(measurements.Count, ShouldEqual, uint64(1))
			So(resonance.Kind, ShouldEqual, "logic")
			So(resonance.Count, ShouldEqual, uint64(1))
			So(hop.Count, ShouldEqual, uint64(1))
			So(hop.LastNs, ShouldBeGreaterThan, uint64(0))
		})
	})
}

func TestCryptoBindDiagnostics(t *testing.T) {
	Convey("Given measurements attached to a crypto trader", t, func() {
		crypto := diagnosticCrypto()
		crypto.measurements = &Measurements{}

		Convey("It should share the clock bank and arm the publisher interval", func() {
			crypto.bindDiagnostics()

			So(crypto.measurements.clocks, ShouldEqual, &crypto.clocks)
			So(crypto.diagnosticInterval, ShouldBeGreaterThan, time.Duration(0))
			So(crypto.startedAt.IsZero(), ShouldBeFalse)
		})
	})
}

func TestCryptoPublishDiagnostics(t *testing.T) {
	Convey("Given a crypto trader that emits replaceable diagnostics", t, func() {
		ui := make(chan []byte, 1)
		ctx, cancel := context.WithCancel(t.Context())
		crypto := diagnosticCrypto()
		crypto.ctx = ctx
		crypto.cancel = cancel
		crypto.ui = ui
		crypto.diagnosticInterval = 10 * time.Millisecond
		var wait sync.WaitGroup
		wait.Add(1)
		go func() {
			defer wait.Done()
			crypto.publishDiagnostics()
		}()
		Reset(func() {
			cancel()
			wait.Wait()
		})

		Convey("It should publish a diagnostics frame on the heartbeat", func() {
			var payload []byte

			select {
			case payload = <-ui:
			case <-time.After(time.Second):
			}

			So(payload, ShouldNotBeNil)
			So(string(payload), ShouldContainSubstring, "diagnostics")
			So(string(payload), ShouldContainSubstring, "flowing")
		})
	})
}

func diagnosticCrypto() *Crypto {
	ctx, cancel := context.WithCancel(context.Background())

	return &Crypto{
		ctx:    ctx,
		cancel: cancel,
	}
}

func hopNamed(hops []HopSnapshot, from string, to string) HopSnapshot {
	for _, hop := range hops {
		if hop.From == from && hop.To == to {
			return hop
		}
	}

	return HopSnapshot{}
}

func stageNamed(stages []ClockSnapshot, name string) ClockSnapshot {
	for _, stage := range stages {
		if stage.Name == name {
			return stage
		}
	}

	return ClockSnapshot{}
}
