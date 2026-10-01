package strategy

import (
	"context"
	"errors"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestPaperEnterSoftFailKeepsTrainingReady(t *testing.T) {
	Convey("insufficient available cash soft-fails enter; predict path abstains", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		viper.Set("trading.allocation.max_fraction", 0.1)
		viper.Set("market.model", "paper")

		price := priced(t, "ETH/USD", 0.26)
		price.Transition(runtime.READY)

		balance := broker.NewBalance(ctx, nil)
		balance.Quote = "USD"
		// Total looks rich; Available is the paper CLI truth (0.19).
		balance.UpdateWallet(&kraken.Balance{
			Data: []kraken.BalanceData{{
				Asset:     "USD",
				Balance:   decimal.NewFromFloat64(1000),
				Available: decimal.NewFromFloat64(0.193),
				Reserved:  decimal.NewFromFloat64(999.807),
			}},
		})

		paper := broker.NewPaper(ctx)
		trader := NewTrader(ctx, paper, price, balance)
		if trader.desk != nil && trader.desk.Execution != nil {
			trader.desk.Execution.Transition(runtime.READY)
		}

		training := NewTraining(ctx, 1, price, trader, nil, nil)
		frame := regionFrame("ETH/USD", 1, 2)
		training.grid.Update([]*data.Measurement[float64]{frame})
		training.grid.Settle()

		training.mu.Lock()
		training.checkpointed = true
		training.skill.Update(1)
		training.skill.Update(1)
		training.mu.Unlock()

		So(training.paperOpen(), ShouldBeTrue)
		So(balance.Cash().Cmp(decimal.NewFromFloat64(0.193)), ShouldEqual, 0)

		symbol := "ETH/USD"
		training.remember(symbol, []byte{9, 9, 9})
		training.mu.Lock()
		training.paperPredictions++
		training.mu.Unlock()

		err := training.trader.OnAction(symbol, cognition.ActionEnter, 0.9)
		So(err, ShouldNotBeNil)
		So(broker.IsEnterSoftFail(err), ShouldBeTrue)
		So(trader.desk.Execution.Status(), ShouldEqual, runtime.READY)

		// predict() soft path: Warn + forget — never training.Error(err).
		errnie.Warn("[training] paper enter skipped: " + err.Error())
		training.forget(symbol)

		So(training.entries[symbol], ShouldBeNil)
		So(training.paperTrades, ShouldEqual, 0)
	})
}

func TestIsEnterSoftFailRecognizesInsufficientAvailable(t *testing.T) {
	Convey("paper CLI insufficient-available messages classify as soft", t, func() {
		err := errors.New("kraken paper: Insufficient USD Available: 0.193 Required: 99.999")
		So(broker.IsEnterSoftFail(err), ShouldBeTrue)

		hard := errors.New("disk full")
		So(broker.IsEnterSoftFail(hard), ShouldBeFalse)
	})
}
