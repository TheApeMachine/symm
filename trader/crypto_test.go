package trader

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
testPlanner wires a Planner with nil broker and audit deps for Crypto
construction tests that only exercise runtime assembly.
*/
func testPlanner(
	ctx context.Context,
	uiHub chan<- []byte,
	analyzer *logic.Analyzer,
) *strategy.Planner {
	return strategy.NewPlanner(
		ctx,
		uiHub,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		analyzer,
		nil,
		nil,
	)
}

func TestNewCrypto(t *testing.T) {
	previousDepth := viper.Get("market.l3_depth")
	previousInterval := viper.Get("signals.fluid.integration_interval")
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	t.Cleanup(func() { viper.Set("market.l3_depth", previousDepth) })
	t.Cleanup(func() { viper.Set("signals.fluid.integration_interval", previousInterval) })
	t.Cleanup(func() { viper.Set("signals.feed_timeline_capacity", previousTimeline) })
	t.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("market.l3_depth", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)

	Convey("Given NewCrypto wiring", t, func() {
		ctx := context.Background()
		booter := system.NewBooter(ctx, nil)
		analyzer, err := logic.NewAnalyzer(ctx, booter, nil, nil, nil, nil, nil)
		So(err, ShouldBeNil)
		planner := testPlanner(ctx, nil, analyzer)
		tree := dmt.NewTree(t.TempDir())
		t.Cleanup(func() {
			if err := tree.Close(); err != nil {
				t.Error(err)
			}
		})
		thesis := types.NewThesis(nil, nil)
		thesis.Tick = 46

		Convey("When the runtime is constructed", func() {
			crypto, err := NewCrypto(
				ctx,
				booter,
				nil,
				nil,
				nil,
				nil,
				nil,
				analyzer,
				planner,
				tree,
				thesis,
				nil,
				nil,
			)

			Convey("Then it is ready to start", func() {
				So(err, ShouldBeNil)
				So(crypto, ShouldNotBeNil)
				So(crypto.tick.Load(), ShouldEqual, 46)
			})
		})
	})
}

func TestCryptoRun(t *testing.T) {
	previousDepth := viper.Get("market.l3_depth")
	previousInterval := viper.Get("signals.fluid.integration_interval")
	previousDataPath := viper.Get("system.data_path")
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	t.Cleanup(func() { viper.Set("market.l3_depth", previousDepth) })
	t.Cleanup(func() { viper.Set("signals.fluid.integration_interval", previousInterval) })
	t.Cleanup(func() { viper.Set("system.data_path", previousDataPath) })
	t.Cleanup(func() { viper.Set("signals.feed_timeline_capacity", previousTimeline) })
	t.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("market.l3_depth", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)
	dataPath := t.TempDir()
	viper.Set("system.data_path", dataPath)

	Convey("Given a started crypto runtime", t, func() {
		ctx := context.Background()
		channel := make(chan []byte, 128)
		booter := system.NewBooter(ctx, channel)
		analyzer, err := logic.NewAnalyzer(ctx, booter, nil, nil, nil, nil, nil)
		So(err, ShouldBeNil)
		planner := testPlanner(ctx, channel, analyzer)
		tree := dmt.NewTree(t.TempDir())
		t.Cleanup(func() {
			if err := tree.Close(); err != nil {
				t.Error(err)
			}
		})
		thesis := types.NewThesis(channel, nil)
		previousQuote := viper.GetString("market.quote_currency")
		viper.Set("market.quote_currency", "USD")
		t.Cleanup(func() { viper.Set("market.quote_currency", previousQuote) })
		balance := broker.NewBalance(nil, nil, make(chan []byte, 8))
		balance.BalanceAck([]byte(
			`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
				`"asset":"USD","balance":"1000","available":"1000","reserved":"0"` +
				`}]}`,
		))
		desk := broker.NewDesk(nil, nil, nil, balance)
		hub, err := ui.NewHub(ctx, nil, balance, channel)
		So(err, ShouldBeNil)
		t.Cleanup(func() {
			if err := hub.Close(); err != nil {
				t.Error(err)
			}
		})

		crypto, err := NewCrypto(
			ctx,
			booter,
			nil,
			nil,
			balance,
			desk,
			nil,
			analyzer,
			planner,
			tree,
			thesis,
			hub,
			nil,
		)

		So(err, ShouldBeNil)

		t.Cleanup(func() {
			if err := crypto.Close(); err != nil {
				t.Error(err)
			}
		})

		booter.AddStages(
			system.NewStage(system.StagePreflight),
			system.NewStage(system.StageWarmup, crypto),
		)

		So(booter.Start(), ShouldBeNil)
		So(crypto.Run(), ShouldBeNil)

		Convey("When no market frame has arrived", func() {
			Convey("Then the runtime should wait without advancing the tick", func() {
				So(crypto.Status(), ShouldEqual, types.READY)
				So(crypto.tick.Load(), ShouldEqual, 0)
			})
		})

		Convey("When one market frame arrives", func() {
			crypto.market.OnTicker([]byte(`{
				"channel":"ticker",
				"type":"update",
				"data":[{
					"symbol":"BTC/USD",
					"bid":"100",
					"ask":"101",
					"last":"100.5",
					"volume":10,
					"timestamp":"2026-07-16T20:00:00Z"
				}]
			}`))
			deadline := time.Now().Add(time.Second)

			for crypto.tick.Load() == 0 && time.Now().Before(deadline) {
				runtime.Gosched()
			}

			Convey("Then the runtime reports ready and advances the tick", func() {
				So(crypto.Status(), ShouldEqual, types.READY)
				So(crypto.tick.Load(), ShouldEqual, 1)

				published := int64(0)
				deadline = time.Now().Add(time.Second)

				for published == 0 && time.Now().Before(deadline) {
					select {
					case raw := <-channel:
						frame := struct {
							Tick *struct {
								Count int64 `json:"count"`
							} `json:"tick"`
						}{}

						So(sonic.Unmarshal(raw, &frame), ShouldBeNil)

						if frame.Tick != nil {
							published = frame.Tick.Count
						}
					default:
						runtime.Gosched()
					}
				}

				So(published, ShouldEqual, 1)
			})
		})
	})
}

/*
TestCryptoTradePointerHoldings proves enter submission reads the pointer shape
admit/planner store on Thesis.Holdings — a value assertion panics at runtime.
*/
func TestCryptoTradePointerHoldings(t *testing.T) {
	previousDepth := viper.Get("market.l3_depth")
	previousInterval := viper.Get("signals.fluid.integration_interval")
	previousTrack := viper.Get("signals.feed_track_capacity")
	t.Cleanup(func() { viper.Set("market.l3_depth", previousDepth) })
	t.Cleanup(func() { viper.Set("signals.fluid.integration_interval", previousInterval) })
	t.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("market.l3_depth", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("signals.feed_track_capacity", 128)

	Convey("Given an enter decision backed by a pointer holding", t, func() {
		ctx := context.Background()
		booter := system.NewBooter(ctx, nil)
		analyzer, err := logic.NewAnalyzer(ctx, booter, nil, nil, nil, nil, nil)
		So(err, ShouldBeNil)

		crypto := &Crypto{
			desk:    broker.NewDesk(nil, nil, nil, nil),
			planner: testPlanner(ctx, nil, analyzer),
		}
		thesis := types.NewThesis(nil, nil)
		holding := types.NewHolding(
			ctx,
			"BTC/USD",
			decimal.NewFromFloat64(0.01),
		)
		thesis.Holdings.Store("BTC/USD", holding)
		thesis.Decisions = []types.Decision{{
			Symbol: "BTC/USD",
			Action: "enter",
		}}

		Convey("When trade submits the enter", func() {
			So(func() { crypto.planner.Update(thesis, nil, 0) }, ShouldNotPanic)
		})
	})
}
