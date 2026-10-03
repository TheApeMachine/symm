package correlation_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/correlation"
)

func TestCorrelationSignalMetrics(t *testing.T) {
	Convey("Correlation signal measures asynchronous price-path correlation", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)

		instrument := correlation.NewSignal(ctx, arena)
		instrument.Transition(nmruntime.READY)

		now := time.Now()

		for step := range 20 {
			// Feed BTC
			btc := arena.NewMeasurement("ingress")
			btc.Label = "BTC/USD"
			btc.SeqIdx = int64(step*2 + 1)
			btc.At = now.Add(time.Duration(step*100) * time.Millisecond)
			btc.From = btc.At
			btc.WriteMetric("last", 50000.0+float64(step)*10.0)

			resBTC := instrument.Step(btc)
			So(resBTC, ShouldNotBeNil)

			// Feed ETH
			eth := arena.NewMeasurement("ingress")
			eth.Label = "ETH/USD"
			eth.SeqIdx = int64(step*2 + 2)
			eth.At = now.Add(time.Duration(step*100+20) * time.Millisecond)
			eth.From = eth.At
			eth.WriteMetric("last", 3000.0+float64(step)*2.0)

			resETH := instrument.Step(eth)
			So(resETH, ShouldNotBeNil)

			So(resBTC.Maturity, ShouldBeGreaterThanOrEqualTo, 0)
			So(resETH.Maturity, ShouldBeGreaterThanOrEqualTo, 0)
		}
	})
}
