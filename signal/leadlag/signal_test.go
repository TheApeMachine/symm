package leadlag_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/leadlag"
)

func TestLeadLagTickerMetrics(t *testing.T) {
	Convey("Leadlag ticker instrument computes asynchronous cross lead-lag", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)

		instrument := leadlag.NewSignal(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Now()

		for step := 0; step < 20; step++ {
			// Feed BTC
			btc := arena.NewMeasurement("ingress")
			btc.Label = "BTC/USD"
			btc.SeqIdx = int64(step*2 + 1)
			btc.At = now.Add(time.Duration(step*100) * time.Millisecond)
			btc.From = btc.At
			btcPrice := 50000.0 + float64(step)*10.0
			btc.SetMetric("last", data.NewMetric[float64](
				"last",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				btcPrice,
				1.0,
			).Write(btcPrice))

			resBTC := instrument.Step(btc)
			So(resBTC, ShouldNotBeNil)

			// Feed ETH with lag
			eth := arena.NewMeasurement("ingress")
			eth.Label = "ETH/USD"
			eth.SeqIdx = int64(step*2 + 2)
			eth.At = now.Add(time.Duration(step*100+30) * time.Millisecond)
			eth.From = eth.At
			ethPrice := 3000.0 + float64(step)*5.0
			eth.SetMetric("last", data.NewMetric[float64](
				"last",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				ethPrice,
				1.0,
			).Write(ethPrice))

			resETH := instrument.Step(eth)
			So(resETH, ShouldNotBeNil)

			So(resBTC.Maturity, ShouldBeGreaterThanOrEqualTo, 0)
			So(resETH.Maturity, ShouldBeGreaterThanOrEqualTo, 0)
		}
	})
}
