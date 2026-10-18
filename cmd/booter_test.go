package cmd_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/morphology"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
)

func TestBooterSignals(t *testing.T) {
	Convey("Given initialized and ready signals", t, func() {
		ctx := context.Background()

		corrSig := correlation.NewSignal(ctx)
		corrSig.Transition(runtime.READY)

		cvdSig := cvd.NewSignal(ctx)
		cvdSig.Transition(runtime.READY)

		depthSig := depthflow.NewSignal(ctx)
		depthSig.Transition(runtime.READY)

		hawkesSig := hawkes.NewSignal(ctx)
		hawkesSig.Transition(runtime.READY)

		leadSig := leadlag.NewSignal(ctx)
		leadSig.Transition(runtime.READY)

		liqSig := liquidity.NewSignal(ctx)
		liqSig.Transition(runtime.READY)

		morphSig := morphology.NewSignal(ctx)
		morphSig.Transition(runtime.READY)

		pdSig := pumpdump.NewSignal(ctx)
		pdSig.Transition(runtime.READY)

		sentSig := sentiment.NewSignal(ctx)
		sentSig.Transition(runtime.READY)

		toxSig := toxicity.NewSignal(ctx)
		toxSig.Transition(runtime.READY)

		Convey("When stepping a market trade Value through signals", func() {
			val := data.NewValue("kraken", "trade", "BTC/USD")
			val.Timestamp = uint64(time.Now().UnixNano())
			rawTrade := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":67500.5,"qty":0.25,"ord_type":"limit","trade_id":1001}]}`)
			val.Append(rawTrade)

			val = corrSig.Step(val)
			So(val, ShouldNotBeNil)

			val = cvdSig.Step(val)
			So(val, ShouldNotBeNil)

			val = hawkesSig.Step(val)
			So(val, ShouldNotBeNil)

			val = leadSig.Step(val)
			So(val, ShouldNotBeNil)

			val = pdSig.Step(val)
			So(val, ShouldNotBeNil)

			val = sentSig.Step(val)
			So(val, ShouldNotBeNil)

			val = depthSig.Step(val)
			So(val, ShouldNotBeNil)

			val = liqSig.Step(val)
			So(val, ShouldNotBeNil)

			val = morphSig.Step(val)
			So(val, ShouldNotBeNil)

			val = toxSig.Step(val)
			So(val, ShouldNotBeNil)

			Convey("Then the single Value accumulates layers and indices", func() {
				So(len(val.Layers), ShouldBeGreaterThan, 1)
				So(len(val.Index), ShouldEqual, len(val.Layers))

				count := 0
				for _, vec := range data.Layer[[]float64](val, "gross_notional") {
					So(len(vec), ShouldEqual, 4)
					count++
				}
				So(count, ShouldEqual, 1)
			})
		})
	})
}
