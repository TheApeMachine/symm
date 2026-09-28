package broker

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAnomalyMonitor(t *testing.T) {
	Convey("Given an AnomalyMonitor instance", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		monitor := NewAnomalyMonitor(ctx, 128)
		defer monitor.Close()

		Convey("When no anomalies have been recorded", func() {
			Convey("Then initial counts are zero and health is 1.0", func() {
				So(monitor.Count("BTC/USD"), ShouldEqual, 0)
				So(monitor.Total(), ShouldEqual, 0)
				So(monitor.Health("BTC/USD"), ShouldEqual, 1.0)
			})
		})

		Convey("When transient crossed book jitter occurs (<= 3 ticks)", func() {
			monitor.Record("BTC/USD", AnomalyCrossedBook)
			monitor.Record("BTC/USD", AnomalyCrossedBook)
			monitor.Record("BTC/USD", AnomalyCrossedBook)

			Convey("Then severe fault is not triggered and health degrades gradually", func() {
				So(monitor.HasSevereFault("BTC/USD"), ShouldBeFalse)
				So(monitor.Health("BTC/USD"), ShouldBeGreaterThan, 0.0)
			})
		})

		Convey("When crossed book persists for more than 3 consecutive ticks", func() {
			var faultedSymbol string
			var recoveredSymbol string

			monitor.SetOnFault(func(symbol string) {
				faultedSymbol = symbol
			})
			monitor.SetOnRecover(func(symbol string) {
				recoveredSymbol = symbol
			})

			monitor.Record("BTC/USD", AnomalyCrossedBook)
			monitor.Record("BTC/USD", AnomalyCrossedBook)
			monitor.Record("BTC/USD", AnomalyCrossedBook)
			monitor.Record("BTC/USD", AnomalyCrossedBook) // 4th tick (>3 consecutive)

			Convey("Then severe fault triggers immediately and health drops to 0.0", func() {
				So(monitor.HasSevereFault("BTC/USD"), ShouldBeTrue)
				So(monitor.Health("BTC/USD"), ShouldEqual, 0.0)
				So(faultedSymbol, ShouldEqual, "BTC/USD")
			})

			Convey("When the order book uncrosses and standardizes for 3 clean ticks", func() {
				monitor.RecordClean("BTC/USD")
				So(monitor.HasSevereFault("BTC/USD"), ShouldBeTrue)

				monitor.RecordClean("BTC/USD")
				So(monitor.HasSevereFault("BTC/USD"), ShouldBeTrue)

				monitor.RecordClean("BTC/USD") // 3rd clean tick -> stabilization threshold met
				So(monitor.HasSevereFault("BTC/USD"), ShouldBeFalse)
				So(recoveredSymbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}
