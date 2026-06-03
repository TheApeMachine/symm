package market

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/public"
)

func TestInstrumentTick(t *testing.T) {
	Convey("Given an instrument snapshot on the shared raw bus as *SocketMessage", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		defer pool.Close()

		raw := bus.Group(pool, "raw", 10*time.Millisecond)
		publicOut := bus.Group(pool, "kraken:public", 10*time.Millisecond)
		publicSubscriber := publicOut.Subscribe("test:instrument", 16)

		instrument := NewInstrument(ctx, pool)

		tickDone := make(chan struct{})

		go func() {
			_ = instrument.Tick()
			close(tickDone)
		}()

		payload, err := json.Marshal(InstrumentUpdate{
			Pairs: []InstrumentPair{{
				Symbol:         "BTC/EUR",
				Base:           "BTC",
				Quote:          "EUR",
				Status:         "online",
				QtyPrecision:   8,
				QtyIncrement:   0.00000001,
				PricePrecision: 1,
				CostPrecision:  5,
				PriceIncrement: 0.1,
				QtyMin:         0.00005,
			}},
		})

		So(err, ShouldBeNil)

		raw.Send(&qpool.QValue[any]{
			Value: &public.SocketMessage{
				Channel: "instrument",
				Type:    "snapshot",
				Data:    payload,
			},
		})

		viper.Set("market.max_scan_symbols", 8)
		viper.Set("market.book_depth_levels", 10)
		viper.Set("market.default_symbols", []string{"BTC/EUR"})
		viper.Set("market.anchor_symbol", "BTC/EUR")

		Convey("It should emit book subscribe on kraken:public", func() {
			deadline := time.After(2 * time.Second)
			var bookSubscribe bool

			for !bookSubscribe {
				select {
				case message := <-publicSubscriber.Incoming:
					frame, ok := message.Value.(map[string]any)

					if !ok {
						continue
					}

					params, _ := frame["params"].(map[string]any)

					if params["channel"] == "book" {
						bookSubscribe = true
					}
				case <-deadline:
					So("timeout waiting for book subscribe", ShouldBeEmpty)
					return
				}
			}

			So(bookSubscribe, ShouldBeTrue)
		})

		cancel()

		select {
		case <-tickDone:
		case <-time.After(2 * time.Second):
		}
	})
}
