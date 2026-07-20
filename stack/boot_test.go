package stack_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

func TestBoot(t *testing.T) {
	previousModel := viper.Get("trading.model")
	previousQuote := viper.Get("market.quote_currency")
	previousBatch := viper.Get("market.subscribe_batch")
	previousPace := viper.Get("market.subscribe_pace")
	previousLevel3 := viper.Get("market.l3_enabled")
	previousInterval := viper.Get("signals.fluid.integration_interval")
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	previousDataPath := viper.Get("system.data_path")
	t.Cleanup(func() {
		viper.Set("trading.model", previousModel)
		viper.Set("market.quote_currency", previousQuote)
		viper.Set("market.subscribe_batch", previousBatch)
		viper.Set("market.subscribe_pace", previousPace)
		viper.Set("market.l3_enabled", previousLevel3)
		viper.Set("signals.fluid.integration_interval", previousInterval)
		viper.Set("signals.feed_timeline_capacity", previousTimeline)
		viper.Set("signals.feed_track_capacity", previousTrack)
		viper.Set("system.data_path", previousDataPath)
	})
	viper.Set("trading.model", "live")
	viper.Set("market.quote_currency", "USD")
	viper.Set("market.subscribe_batch", 200)
	viper.Set("market.subscribe_pace", time.Duration(0))
	viper.Set("market.l3_enabled", false)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)
	viper.Set("system.data_path", t.TempDir())

	Convey("Given synthetic public and private Kraken connections", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		mock := mockapi.NewMockAPI()
		So(mock.SetTradeVolumeResponse(&kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFee{
				"MATICUSD": {Fee: decimal.NewFromFloat64(0.26)},
			}},
		}), ShouldBeNil)
		api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), nil)
		tree := dmt.NewTree(t.TempDir())
		t.Cleanup(func() {
			if closeErr := tree.Close(); closeErr != nil {
				t.Error(closeErr)
			}
		})

		go func() {
			instrumentSent := false
			// ponytail: this mock Conn exposes no write notification, so the test
			// intentionally polls; an event-driven write notification is the upgrade.
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}

				if !instrumentSent {
					for _, request := range mock.Public().Writes() {
						if !bytes.Contains(request, []byte(`"channel":"instrument"`)) {
							continue
						}

						mock.Emit("instrument", []byte(`{
							"channel":"instrument","type":"snapshot","data":{"pairs":[{
								"symbol":"MATIC/USD","base":"MATIC","quote":"USD","status":"online",
								"qty_precision":8,"qty_increment":0.00000001,"price_precision":4,
								"cost_precision":6,"cost_min":0.43,"tick_size":0.0001,
								"price_increment":0.0001,"qty_min":4
							}]}}`))
						instrumentSent = true
						break
					}
				}

				for _, request := range mock.Private().Writes() {
					if !bytes.Contains(request, []byte(`"channel":"balances"`)) {
						continue
					}

					mock.Private().Emit("balances", []byte(`{
						"channel":"balances","type":"snapshot","sequence":1,"data":[{
							"asset":"USD","balance":"1000","available":"1000","reserved":"0"
						}]}`))
					return
				}
			}
		}()

		channel := make(chan []byte, 64)
		wired, err := stack.Boot(ctx, api, stack.Options{
			Booter:  system.NewBooter(ctx, channel),
			Channel: channel,
			Thesis:  types.NewThesis(channel, nil),
			Signals: func(
				ctx context.Context,
				api *websocket.API,
				_ *broker.Instrument,
				channel chan []byte,
			) []types.Signal {
				return []types.Signal{pumpdump.NewSignal(
					ctx,
					api,
					channel,
					viper.GetInt("signals.feed_track_capacity"),
				)}
			},
			Tree: tree,
		})
		So(err, ShouldBeNil)
		So(wired, ShouldNotBeNil)
		defer wired.Close()

		Convey("When a pump tape crosses the same connections", func() {
			cutAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			var thesis *types.Thesis
			peakObservations := 0

			for frame := range conditions.TapePumpDump().Frames() {
				mock.Emit(frame.Channel, frame.Payload)
				thesis, err = wired.Crypto.Tick(cutAt)
				So(err, ShouldBeNil)
				cutAt = cutAt.Add(time.Second)

				if thesis == nil {
					continue
				}

				peakObservations = max(
					peakObservations,
					types.ObservationCount(thesis.Measurements),
				)
			}

			Convey("Then the production graph completes a measured thesis", func() {
				So(thesis, ShouldNotBeNil)
				So(peakObservations, ShouldBeGreaterThan, 0)
			})
		})
	})
}
