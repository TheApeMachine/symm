package trader

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"

	. "github.com/smartystreets/goconvey/convey"
)

type cryptoTestSocket struct {
	channels map[string]chan []byte
}

func (socket *cryptoTestSocket) Observe(channel string) chan []byte {
	if socket.channels == nil {
		socket.channels = map[string]chan []byte{}
	}

	if socket.channels[channel] == nil {
		socket.channels[channel] = make(chan []byte, 4)
	}

	return socket.channels[channel]
}

type cryptoTestPrivate struct {
	cryptoTestSocket
	orders []*kraken.Order
}

func (private *cryptoTestPrivate) Submit(order *kraken.Order) error {
	private.orders = append(private.orders, order)
	return nil
}

func (private *cryptoTestPrivate) Close() {
}

func TestCryptoExecute(testingTB *testing.T) {
	Convey("Given a crypto trader execution boundary", testingTB, func() {
		restore := configureCryptoTestPortfolio()
		defer restore()

		public := &cryptoTestSocket{}
		private := &cryptoTestPrivate{}
		desk, err := broker.NewDesk(context.Background(), public, private, make(chan []byte))
		So(err, ShouldBeNil)

		portfolio, err := NewPortfolio(nil)
		So(err, ShouldBeNil)

		crypto := &Crypto{
			desk:      desk,
			portfolio: portfolio,
		}
		actions := []*logic.Action{{
			Symbol:   "BTC/USD",
			Side:     "buy",
			Score:    1,
			Fraction: 0.05,
			Price:    *decimal.NewFromFloat64(100000),
		}}

		Convey("When buy actions arrive before the balance snapshot", func() {
			ready := crypto.execute(actions)

			Convey("Then execution should wait without mutating portfolio state", func() {
				So(ready, ShouldBeFalse)
				So(private.orders, ShouldHaveLength, 0)
				So(portfolio.active(), ShouldEqual, 0)
			})
		})

		Convey("When buy actions arrive after the balance snapshot", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			readyDesk, err := broker.NewDesk(ctx, public, private, make(chan []byte))
			So(err, ShouldBeNil)
			readyDesk.UIForward = make(chan []byte, 4)
			crypto.desk = readyDesk

			done := make(chan error, 1)
			go func() {
				done <- readyDesk.Run()
			}()

			private.channels["balances"] <- []byte(`[{
					"asset": "USD",
					"balance": 200,
					"available": 200
				}]`)
			private.channels["executions"] <- []byte(`[]`)
			private.channels["orders"] <- []byte(`[]`)

			deadline := time.After(time.Second)
			for !readyDesk.Ready() {
				select {
				case <-readyDesk.UIForward:
				case <-deadline:
					testingTB.Fatal("desk did not publish account state")
				}
			}

			ready := crypto.execute(actions)
			cancel()

			Convey("Then it should submit the order through the desk", func() {
				So(<-done, ShouldBeNil)
				So(ready, ShouldBeTrue)
				So(private.orders, ShouldHaveLength, 1)
				So(portfolio.active(), ShouldEqual, 1)
			})
		})
	})
}

func BenchmarkCryptoExecute(benchmarkTB *testing.B) {
	restore := configureCryptoTestPortfolio()
	defer restore()

	public := &cryptoTestSocket{}
	private := &cryptoTestPrivate{}
	desk, err := broker.NewDesk(context.Background(), public, private, make(chan []byte))
	if err != nil {
		benchmarkTB.Fatal(err)
	}

	portfolio, err := NewPortfolio(nil)
	if err != nil {
		benchmarkTB.Fatal(err)
	}

	crypto := &Crypto{
		desk:      desk,
		portfolio: portfolio,
	}
	actions := []*logic.Action{{
		Symbol:   "BTC/USD",
		Side:     "buy",
		Score:    1,
		Fraction: 0.05,
		Price:    *decimal.NewFromFloat64(100000),
	}}

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		_ = crypto.execute(actions)
	}
}

func configureCryptoTestPortfolio() func() {
	previousNormalSlots := viper.GetInt("trading.slots.normal")
	previousOpportunitySlots := viper.GetInt("trading.entry.opportunity_slot_count")
	previousMaxPositions := viper.GetInt("trading.max_concurrent_positions")
	previousTrailingOffset := viper.GetFloat64("trading.stop.trailing_offset_bps")
	previousMinOffset := viper.GetFloat64("trading.stop.min_offset_bps")
	previousMaxOffset := viper.GetFloat64("trading.stop.max_offset_bps")
	previousTakerFee := viper.GetFloat64("trading.paper.taker_fee_bps")
	previousSlippage := viper.GetFloat64("trading.paper.slippage_bps")

	viper.Set("trading.slots.normal", 1)
	viper.Set("trading.entry.opportunity_slot_count", 0)
	viper.Set("trading.max_concurrent_positions", 1)
	viper.Set("trading.stop.trailing_offset_bps", 100)
	viper.Set("trading.stop.min_offset_bps", 20)
	viper.Set("trading.stop.max_offset_bps", 500)
	viper.Set("trading.paper.taker_fee_bps", 40)
	viper.Set("trading.paper.slippage_bps", 2)

	return func() {
		viper.Set("trading.slots.normal", previousNormalSlots)
		viper.Set("trading.entry.opportunity_slot_count", previousOpportunitySlots)
		viper.Set("trading.max_concurrent_positions", previousMaxPositions)
		viper.Set("trading.stop.trailing_offset_bps", previousTrailingOffset)
		viper.Set("trading.stop.min_offset_bps", previousMinOffset)
		viper.Set("trading.stop.max_offset_bps", previousMaxOffset)
		viper.Set("trading.paper.taker_fee_bps", previousTakerFee)
		viper.Set("trading.paper.slippage_bps", previousSlippage)
	}
}
