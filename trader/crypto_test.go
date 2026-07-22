package trader

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

type emitSignal struct {
	tickerIn chan []kraken.TickerData
	bookIn   chan []kraken.BookData
	tradeIn  chan []kraken.TradeData
	ctx      context.Context
	cancel   context.CancelFunc
}

func newEmitSignal(ctx context.Context) *emitSignal {
	ctx, cancel := context.WithCancel(ctx)
	signal := &emitSignal{
		tickerIn: make(chan []kraken.TickerData, 64),
		bookIn:   make(chan []kraken.BookData, 64),
		tradeIn:  make(chan []kraken.TradeData, 64),
		ctx:      ctx,
		cancel:   cancel,
	}

	return signal
}

func (signal *emitSignal) Tickers() chan []kraken.TickerData { return signal.tickerIn }
func (signal *emitSignal) Books() chan []kraken.BookData     { return signal.bookIn }
func (signal *emitSignal) Trades() chan []kraken.TradeData   { return signal.tradeIn }

func (signal *emitSignal) Measure() chan []*types.Measurement {
	out := make(chan []*types.Measurement, 64)

	go func() {
		defer close(out)

		for {
			select {
			case <-signal.ctx.Done():
				return
			case rows := <-signal.tickerIn:
				if len(rows) == 0 {
					continue
				}

				out <- []*types.Measurement{{
					Source: types.SourcePumpDump,
					Stream: types.PumpDump,
					Metric: types.MetricIgnition,
					Symbol: rows[0].Symbol,
					At:     time.Now().UTC(),
					Unit:   types.UnitDimensionless,
					Raw:    1,
				}}
			case <-signal.bookIn:
			case <-signal.tradeIn:
			}
		}
	}()

	return out
}

func TestCryptoTick(t *testing.T) {
	Convey("Given a crypto runtime with signal ingress and a measuring signal", t, func() {
		viper.Set("signals.feed_timeline_capacity", 16)
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.slots.normal", 2)
		viper.Set("trading.slots.reserved", 0)
		viper.Set("trading.allocation.max_fraction", 0.2)

		ctx := context.Background()
		messages := make(chan []byte, 8)
		hub := ui.NewHub(ctx, nil, nil, messages)
		defer hub.Close()

		balance := broker.NewBalance(nil, nil, messages)
		balance.BalanceAck([]byte(
			`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
				`"asset":"USD","balance":"1000","available":"1000"}]}`,
		))
		desk := broker.NewDesk(nil, nil, nil, balance)
		signal := newEmitSignal(ctx)
		defer signal.cancel()
		planner := strategy.NewPlanner(
			ctx, messages, nil, desk, nil, nil, balance, nil, nil, nil,
		)
		defer planner.Close()

		crypto := &Crypto{
			ctx:     ctx,
			status:  types.READY,
			desk:    desk,
			balance: balance,
			planner: planner,
			uiHub:   hub,
			signals: []types.Signal{signal},
			outs:    []chan []*types.Measurement{signal.Measure()},
			tick:    &atomic.Int64{},
		}

		crypto.OnTicker([]byte(
			`{"channel":"ticker","type":"update","data":[{` +
				`"symbol":"BTC/USD","bid":"100","ask":"101","last":"100.5"}]}`,
		))

		Convey("When Tick drains signal measurements and Decides", func() {
			thesis, tickErr := crypto.Tick()

			Convey("It completes a durable tick", func() {
				So(tickErr, ShouldBeNil)
				So(thesis, ShouldNotBeNil)
				So(thesis.Tick, ShouldEqual, 1)
				So(len(thesis.Measurements), ShouldBeGreaterThan, 0)
				So(crypto.lastThesis.Load(), ShouldEqual, thesis)
			})
		})
	})
}
