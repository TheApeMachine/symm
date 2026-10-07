package strategy

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestAuthorized(t *testing.T) {
	Convey("Given a Training component with triad gating", t, func() {
		at := time.Now().UTC()

		Convey("When resonance surprise is positive and manifold impedance is clear", func() {
			resonanceM := data.NewMeasurement(1, "BTC/USD", "resonance", 1, 1)
			resonanceM.At = at
			resonanceM.From = at
			resonanceM.Write(data.NewMetric("surprise", 1.5, data.UnitRatio, data.TimescaleInstantaneous))

			manifoldM := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			manifoldM.At = at
			manifoldM.From = at
			manifoldM.Write(
				data.NewMetric("kuramoto_r", 0.4, data.UnitRatio, data.TimescaleInstantaneous),
				data.NewMetric("pressure_grad_norm", 0.1, data.UnitRatio, data.TimescaleInstantaneous),
			)

			So(authorized(resonanceM, manifoldM), ShouldBeTrue)
		})

		Convey("When resonance surprise is zero (equilibrium churn), entry is vetoed", func() {
			resonanceM := data.NewMeasurement(1, "BTC/USD", "resonance", 1, 1)
			resonanceM.At = at
			resonanceM.From = at
			resonanceM.Write(data.NewMetric("surprise", 0.0, data.UnitRatio, data.TimescaleInstantaneous))

			manifoldM := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			manifoldM.At = at
			manifoldM.From = at
			manifoldM.Write(data.NewMetric("kuramoto_r", 0.4, data.UnitRatio, data.TimescaleInstantaneous))

			So(authorized(resonanceM, manifoldM), ShouldBeFalse)
		})

		Convey("When manifold has complete locked synchronization and opposing pressure, entry is vetoed", func() {
			resonanceM := data.NewMeasurement(1, "BTC/USD", "resonance", 1, 1)
			resonanceM.At = at
			resonanceM.From = at
			resonanceM.Write(data.NewMetric("surprise", 2.0, data.UnitRatio, data.TimescaleInstantaneous))

			manifoldM := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			manifoldM.At = at
			manifoldM.From = at
			manifoldM.Write(
				data.NewMetric("kuramoto_r", 1.0, data.UnitRatio, data.TimescaleInstantaneous),
				data.NewMetric("pressure_grad_norm", 5.0, data.UnitRatio, data.TimescaleInstantaneous),
			)

			So(authorized(resonanceM, manifoldM), ShouldBeFalse)
		})
	})
}

/*
fillingTransport fills every market order at a fixed unit price with a fixed
fee, echoing the client order ID back through Desk.Apply like Paper does.
*/
type fillingTransport struct {
	desk  *broker.Desk
	unit  map[string]float64
	fee   float64
	write chan struct{}
}

func (transport *fillingTransport) Write(buf []byte) error {
	message := kraken.AddOrderMessage{}
	if err := sonic.Unmarshal(buf, &message); err != nil {
		return err
	}
	quantity, err := decimal.NewFromString(message.Params.Volume)
	if err != nil {
		return err
	}
	unit := transport.unit[message.Params.Type]
	transport.desk.Apply(&kraken.Execution{Data: []kraken.ExecutionData{{
		ClientOrderID: message.Params.ClOrdId,
		Symbol:        message.Params.Pair,
		Side:          message.Params.Type,
		LastQty:       quantity,
		Cost:          decimal.NewFromFloat64(quantity.Float64() * unit),
		FeeUsdEquiv:   decimal.NewFromFloat64(transport.fee),
	}}})
	transport.write <- struct{}{}
	return nil
}

func TestPaper_SettleRefines(t *testing.T) {
	Convey("Given a READY training session that paper trades a round trip", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {
					WSName: "BTC/USD", Base: "BTC", Quote: "USD",
					LotDecimals: 8, LotMultiplier: 1,
				},
			},
		})

		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.SetReferenceCash(decimal.NewFromFloat64(200))
		price.SetQuote("BTC/USD", decimal.NewFromFloat64(59990), decimal.NewFromFloat64(60000))

		transport := &fillingTransport{
			unit:  map[string]float64{"buy": 60000, "sell": 63000},
			fee:   0.1,
			write: make(chan struct{}, 2),
		}
		desk := broker.NewDesk(ctx, transport, price)
		transport.desk = desk

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)

		training := NewTraining(
			ctx, price, desk, catalog, storeTee, 1000,
		)
		training.Transition(runtime.READY)

		training.paper.episode("BTC/USD").update(func(state *episodeState) {
			state.entry = []byte("R0_R1/R1_R2")
			state.exit = []byte("R2_R3/R3_R0")
		})

		before := census(training, "records")

		So(desk.Enter("BTC/USD"), ShouldBeNil)
		select {
		case <-transport.write:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for entry fill")
		}

		So(desk.Exit("BTC/USD"), ShouldBeNil)
		select {
		case <-transport.write:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for exit fill")
		}

		deadline := time.After(2 * time.Second)
		for training.paper.resolved.Load() == 0 {
			select {
			case <-deadline:
				t.Fatal("timed out waiting for settle")
			case <-time.After(10 * time.Millisecond):
			}
		}

		So(training.paper.resolved.Load(), ShouldEqual, 1)
		So(census(training, "records"), ShouldBeGreaterThan, before)
		So(census(training, "enter"), ShouldBeGreaterThan, 0)
		So(census(training, "exit"), ShouldBeGreaterThan, 0)
		So(math.Float64frombits(training.paper.returnsBits.Load()), ShouldNotEqual, 0)
	})
}

func TestPaper_WaitNeverEnters(t *testing.T) {
	Convey("Given a READY training session whose trie answers a live context", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {
					WSName: "BTC/USD", Base: "BTC", Quote: "USD",
					LotDecimals: 8, LotMultiplier: 1,
				},
			},
		})

		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.SetReferenceCash(decimal.NewFromFloat64(200))
		price.SetQuote("BTC/USD", decimal.NewFromFloat64(59990), decimal.NewFromFloat64(60000))

		transport := &fillingTransport{
			unit:  map[string]float64{"buy": 60000, "sell": 60000},
			fee:   0.1,
			write: make(chan struct{}, 2),
		}
		desk := broker.NewDesk(ctx, transport, price)
		transport.desk = desk

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)

		training := NewTraining(
			ctx, price, desk, tablestest.New(t), storeTee, 1000,
		)

		history := store.NewStream()

		for tick := int64(1); tick <= 16; tick++ {
			training.impulse.grid.Update(tick, history.Deform(map[string]float64{
				store.CellKey("cvd_value"):    1.5 + float64(tick*tick)*0.1,
				store.CellKey("hawkes_value"): 3.0 + float64(tick*tick)*0.2,
			}))
		}

		training.impulse.grid.Settle()
		training.Transition(runtime.READY)

		var priorLive *data.Measurement

		live := func(value float64) *data.Measurement {
			measurement := data.NewMeasurement(1000, "BTC/USD", "cvd", 1, 1)

			if priorLive != nil {
				measurement = priorLive.Next("cvd")
			}

			measurement.At = time.Now().UTC()
			measurement.From = measurement.At
			priorLive = measurement.Write(
				data.NewMetric("cvd_value", value, data.UnitCount, data.TimescaleTick),
			)
			return priorLive
		}

		// The live stream needs one observation before it moves; the
		// second step, far above cvd's noise floor, lights cvd's region and
		// is the one the trie answers.
		tok := []byte{training.impulse.grid.Cells[store.CellKey("cvd_value")].Region}

		primed := training.Step(live(2.0))
		So(metricRaw(primed, "action"), ShouldEqual, 0)

		teach := func(action string) {
			So(training.Model.Teach(string(tok), action, 0.01), ShouldBeNil)
		}

		Convey("When the trie abstains (wait stance, no enter leaf)", func() {
			// No Teach: empty Winner is precursor wait stance, not a wait leaf.
			out := training.Step(live(4.0))

			Convey("No order is submitted and the desk stays flat", func() {
				So(out, ShouldNotBeNil)

				select {
				case <-transport.write:
					t.Fatal("abstention submitted an order")
				case <-time.After(200 * time.Millisecond):
				}

				So(desk.State("BTC/USD"), ShouldEqual, broker.FLAT)
				So(metricRaw(out, "action"), ShouldEqual, 0)
			})
		})

		Convey("When the trie answers enter", func() {
			teach(actionEnter)
			out := training.Step(live(4.0))

			Convey("The entry is submitted to the desk", func() {
				select {
				case <-transport.write:
				case <-time.After(time.Second):
					t.Fatal("enter submitted no order")
				}

				So(metricRaw(out, "action"), ShouldEqual, 1)
			})
		})
	})
}

func TestPaper_LossDampensEnter(t *testing.T) {
	Convey("Given a READY training session that paper trades a losing round trip", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {
					WSName: "BTC/USD", Base: "BTC", Quote: "USD",
					LotDecimals: 8, LotMultiplier: 1,
				},
			},
		})

		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.SetReferenceCash(decimal.NewFromFloat64(200))
		price.SetQuote("BTC/USD", decimal.NewFromFloat64(59990), decimal.NewFromFloat64(60000))

		transport := &fillingTransport{
			unit:  map[string]float64{"buy": 60000, "sell": 59000},
			fee:   0.1,
			write: make(chan struct{}, 2),
		}
		desk := broker.NewDesk(ctx, transport, price)
		transport.desk = desk

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)

		training := NewTraining(
			ctx, price, desk, tablestest.New(t), storeTee, 1000,
		)
		training.Transition(runtime.READY)

		training.paper.episode("BTC/USD").update(func(state *episodeState) {
			state.entry = []byte("R0_R1/R1_R2")
			state.exit = []byte("R2_R3/R3_R0")
		})

		So(desk.Enter("BTC/USD"), ShouldBeNil)
		select {
		case <-transport.write:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for entry fill")
		}

		So(desk.Exit("BTC/USD"), ShouldBeNil)
		select {
		case <-transport.write:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for exit fill")
		}

		deadline := time.After(2 * time.Second)
		for training.paper.resolved.Load() == 0 {
			select {
			case <-deadline:
				t.Fatal("timed out waiting for settle")
			case <-time.After(10 * time.Millisecond):
			}
		}

		Convey("The losing entry context dampens enter, never wait or exit alone", func() {
			So(math.Float64frombits(training.paper.returnsBits.Load()), ShouldBeLessThan, 0)
			So(census(training, actionWait), ShouldEqual, 0)
			So(census(training, actionEnter), ShouldBeGreaterThan, 0)
			So(census(training, actionExit), ShouldEqual, 0)

			call, err := training.Model.Recall("R0_R1/R1_R2", "")
			So(err, ShouldBeNil)
			// Penalized enter may still win until pruned; it must not be wait.
			So(call.Winner, ShouldNotEqual, actionWait)
		})
	})
}

func TestPaper_LossDoesNotTeachExitWithoutEnter(t *testing.T) {
	Convey("Given a READY training session that paper trades a losing round trip", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {
					WSName: "BTC/USD", Base: "BTC", Quote: "USD",
					LotDecimals: 8, LotMultiplier: 1,
				},
			},
		})

		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.SetReferenceCash(decimal.NewFromFloat64(200))
		price.SetQuote("BTC/USD", decimal.NewFromFloat64(59990), decimal.NewFromFloat64(60000))

		transport := &fillingTransport{
			unit:  map[string]float64{"buy": 60000, "sell": 59000},
			fee:   0.1,
			write: make(chan struct{}, 2),
		}
		desk := broker.NewDesk(ctx, transport, price)
		transport.desk = desk

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)

		training := NewTraining(
			ctx, price, desk, tablestest.New(t), storeTee, 1000,
		)
		training.Transition(runtime.READY)

		training.paper.episode("BTC/USD").update(func(state *episodeState) {
			state.entry = []byte("R0_R1/R1_R2")
			state.exit = []byte("R2_R3/R3_R0")
		})

		So(desk.Enter("BTC/USD"), ShouldBeNil)
		select {
		case <-transport.write:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for entry fill")
		}

		So(desk.Exit("BTC/USD"), ShouldBeNil)
		select {
		case <-transport.write:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for exit fill")
		}

		deadline := time.After(2 * time.Second)
		for training.paper.resolved.Load() == 0 {
			select {
			case <-deadline:
				t.Fatal("timed out waiting for settle")
			case <-time.After(10 * time.Millisecond):
			}
		}

		Convey("A loss dampens enter, never wait or exit without enter", func() {
			So(math.Float64frombits(training.paper.returnsBits.Load()), ShouldBeLessThan, 0)
			So(census(training, actionWait), ShouldEqual, 0)
			So(census(training, actionEnter), ShouldBeGreaterThan, 0)
			So(census(training, actionExit), ShouldEqual, 0)

			call, err := training.Model.Recall("R0_R1/R1_R2", "")
			So(err, ShouldBeNil)
			So(call.Winner, ShouldNotEqual, actionWait)
		})
	})
}
