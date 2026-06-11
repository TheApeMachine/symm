package depthflow

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/rawbus"
)

func setDepthflowSystemTestConfig() {
	viper.Set("system.queue.ttl", time.Second)
	viper.Set("system.queue.buffer", 16)
	viper.Set("telemetry.gauge.readings_capacity", 16)
	viper.Set("signals.depthflow.measurements_capacity", 4)
	viper.Set("signals.trade_match_window", time.Minute)
	viper.Set("signals.cross_section.return_capacity", 64)
	viper.Set("signals.cross_section.min_bars", 8)
	viper.Set("signals.cross_section.breadth_history_capacity", 64)
}

func TestSystemSymbolAnnounceThenBook(t *testing.T) {
	setDepthflowSystemTestConfig()

	Convey("Given a fresh depthflow system and an announced symbol", t, func() {
		initCrossSection(&market.CrossSectionConfig{
			MatchWindow: time.Minute,
			ReturnCap:   64,
			MinBars:     8,
			BreadthHist: 64,
		})

		ctx, cancel := context.WithCancel(context.Background())
		pool := qpool.NewQ[any](ctx, 2, 16, nil)

		system := NewSystem(ctx, pool)

		t.Cleanup(func() {
			cancel()
			_ = system.Close()
			pool.Close()
		})

		So(system, ShouldNotBeNil)

		tickErr := make(chan error, 1)

		go func() {
			tickErr <- system.Tick()
		}()

		So(rawbus.Send(system.base.Bus(), rawbus.TypeSymbols, []string{"ZBCN/USD"}), ShouldBeNil)

		eventAt := time.Now()
		updates := krakenmarket.BookUpdates{
			{
				Symbol:    "ZBCN/USD",
				Timestamp: eventAt,
				Type:      "snapshot",
				Bids: []krakenmarket.BookLevel{
					{Price: 99, Qty: 10},
					{Price: 98, Qty: 20},
				},
				Asks: []krakenmarket.BookLevel{
					{Price: 101, Qty: 1},
					{Price: 102, Qty: 1},
				},
			},
		}

		So(rawbus.Send(system.base.Bus(), rawbus.TypeBook, &updates), ShouldBeNil)

		Convey("Tick should keep running after the first book update", func() {
			time.Sleep(300 * time.Millisecond)

			select {
			case err := <-tickErr:
				So(err, ShouldBeNil)
			default:
			}
		})
	})
}
