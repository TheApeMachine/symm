package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

func TestMarketCut(t *testing.T) {
	Convey("Given a MarketState tracking streaming cuts", t, func() {
		state := NewMarketState()
		at := time.Unix(100, 0)
		So(state, ShouldNotBeNil)

		Convey("Updating with ticker and trade folds into a unified cut", func() {
			correlation := data.NewMeasurement[float64]("correlation", map[string]data.Metric[float64]{})
			correlation.Label, correlation.At, correlation.From = "BTC/USD", at, at
			tickerEnv := &Envelope{
				Key:         "BTC/USD",
				TypeID:      EnvelopeTicker,
				TickerData:  kraken.TickerData{Symbol: "BTC/USD"},
				Correlation: correlation,
			}
			tickerEnv.Correlation.Metrics = map[string]data.Metric[float64]{
				"alpha": {
					Label: "alpha",
					Raw:   7.5,
				},
			}

			state.Update(tickerEnv)

			cut := state.Cut("BTC/USD")
			So(cut, ShouldNotBeNil)
			So(cut.Correlation, ShouldNotBeNil)
			So(cut.Correlation, ShouldNotResemble, tickerEnv.Correlation)
			So(cut.Correlation.Metrics["alpha"].Raw, ShouldEqual, tickerEnv.Correlation.Metrics["alpha"].Raw)

			cvd := data.NewMeasurement[float64]("cvd", map[string]data.Metric[float64]{})
			cvd.Label, cvd.At, cvd.From = "BTC/USD", at, at
			tradeEnv := &Envelope{
				Key:       "BTC/USD",
				TypeID:    EnvelopeTrade,
				TradeData: kraken.TradeData{Symbol: "BTC/USD"},
				CVD:       cvd,
			}
			tradeEnv.CVD.Metrics = map[string]data.Metric[float64]{
				"cvd": {
					Label: "cvd",
					Raw:   3.0,
				},
			}
			state.Update(tradeEnv)

			cut = state.Cut("BTC/USD")
			So(cut.Correlation, ShouldNotResemble, tickerEnv.Correlation)
			So(cut.CVD, ShouldNotResemble, tradeEnv.CVD)
			So(cut.CVD.Metrics["cvd"].Raw, ShouldEqual, tradeEnv.CVD.Metrics["cvd"].Raw)
			tickerEnv.Correlation.Metrics["alpha"] = data.Metric[float64]{Label: "alpha"}
			So(cut.Correlation.Metrics["alpha"].Raw, ShouldEqual, 7.5)
			tradeEnv.CVD.Metrics["cvd"] = data.Metric[float64]{Label: "cvd"}
			So(cut.CVD.Metrics["cvd"].Raw, ShouldEqual, 3.0)

			Convey("Hydrating does not alias signal measurement ownership", func() {
				partial := &Envelope{
					Key:    "BTC/USD",
					TypeID: EnvelopeLevel3,
				}
				state.Hydrate(partial)

				So(partial.Correlation, ShouldNotResemble, tickerEnv.Correlation)
				So(partial.CVD, ShouldNotResemble, tradeEnv.CVD)
				So(partial.Correlation.Metrics["alpha"].Raw, ShouldEqual, 7.5)
				So(partial.CVD.Metrics["cvd"].Raw, ShouldEqual, 3.0)
				cut.Correlation.Metrics["alpha"] = data.Metric[float64]{Label: "alpha", Raw: 9}
				So(partial.Correlation.Metrics["alpha"].Raw, ShouldEqual, 7.5)
				cut.CVD.Metrics["cvd"] = data.Metric[float64]{Label: "cvd", Raw: 9}
				So(partial.CVD.Metrics["cvd"].Raw, ShouldEqual, 3.0)
			})

			Convey("Updating manifold state retains physics observables", func() {
				manifold := &ManifoldState{
					At: at,
					Reading: sensorium.Reading{
						Divergence: 1.5,
						KuramotoR:  0.92,
					},
				}
				state.UpdateManifold("BTC/USD", manifold)

				cut := state.Cut("BTC/USD")
				So(cut.Manifold, ShouldEqual, manifold)

				Convey("Hydrating a partial envelope completes missing contemporaneous fields", func() {
					partial := &Envelope{
						Key:    "BTC/USD",
						TypeID: EnvelopeLevel3,
					}
					state.Hydrate(partial)

					So(partial.Correlation, ShouldNotResemble, tickerEnv.Correlation)
					So(partial.CVD, ShouldNotResemble, tradeEnv.CVD)
					So(partial.Manifold, ShouldEqual, manifold)
				})
			})
		})
	})
}
