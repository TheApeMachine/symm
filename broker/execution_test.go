package broker

import (
	"context"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type orderProbe struct {
	writes int
}

func (o *orderProbe) Write(buf []byte) error {
	o.writes++
	return nil
}

func TestEnterOnPendingBeforeTransportWrite(t *testing.T) {
	Convey("onPending runs before EnterWithRegulator (and any Write)", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		viper.Set("trading.allocation.max_fraction", 0.1)
		viper.Set("market.model", "paper")

		probe := &orderProbe{}
		price := NewPrice(ctx, nil, nil, nil, nil)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.1)})

		balance := NewBalance(ctx, nil)
		balance.Quote = "USD"
		balance.UpdateWallet(&kraken.Balance{
			Data: []kraken.BalanceData{{
				Asset:   "USD",
				Balance: decimal.NewFromFloat64(1000),
			}},
		})

		price.Transition(runtime.READY)
		exec := NewExecution(ctx, probe, price, balance)
		exec.Transition(runtime.READY)

		order := make([]string, 0, 2)
		reg, err := exec.Enter("BTC/USD", func(r *position.Regulator) {
			order = append(order, "pending:"+r.Symbol)
			So(probe.writes, ShouldEqual, 0)
		})

		// No book → Quantity fails after onPending, before Write.
		So(order, ShouldResemble, []string{"pending:BTC/USD"})
		So(probe.writes, ShouldEqual, 0)
		So(reg, ShouldNotBeNil)
		So(err, ShouldNotBeNil)
	})
}

func TestEnterInsufficientCashSoftFailKeepsReady(t *testing.T) {
	Convey("insufficient available cash soft-fails without Execution ERROR", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		prevFrac := viper.GetFloat64("trading.allocation.max_fraction")
		viper.Set("trading.allocation.max_fraction", 0.1)
		defer viper.Set("trading.allocation.max_fraction", prevFrac)

		probe := &orderProbe{}
		price := NewPrice(ctx, nil, nil, nil, nil)
		price.SetFee("ETH/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.Transition(runtime.READY)

		balance := NewBalance(ctx, nil)
		balance.Quote = "USD"
		balance.UpdateWallet(&kraken.Balance{
			Data: []kraken.BalanceData{{
				Asset:     "USD",
				Balance:   decimal.NewFromFloat64(1000),
				Available: decimal.NewFromFloat64(0),
			}},
		})

		exec := NewExecution(ctx, probe, price, balance)
		exec.Transition(runtime.READY)

		reg, err := exec.Enter("ETH/USD")
		So(err, ShouldNotBeNil)
		So(errnie.IsValidation(err), ShouldBeTrue)
		So(IsEnterSoftFail(err), ShouldBeTrue)
		So(exec.Status(), ShouldEqual, runtime.READY)
		So(probe.writes, ShouldEqual, 0)
		So(reg, ShouldBeNil)
	})
}

func TestQuantityBelowMinAbstains(t *testing.T) {
	Convey("cash too small for CostMin/QtyMin abstains with validation", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"ETH": {AltName: "ETH"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"ETH/USD": {
					WSName:        "ETH/USD",
					Base:          "ETH",
					Quote:         "USD",
					LotDecimals:   8,
					LotMultiplier: 1,
				},
			},
		})

		inst := &Instrument{cache: &sync.Map{}, quote: "USD"}
		inst.Cache([]kraken.InstrumentPair{{
			Symbol:  "ETH/USD",
			Base:    "ETH",
			Quote:   "USD",
			Status:  "online",
			QtyMin:  decimal.NewFromFloat64(0.01),
			CostMin: decimal.NewFromFloat64(5),
		}})

		price := NewPrice(ctx, nil, nil, inst, normalizer)
		fee := decimal.NewFromFloat64(0.26)
		price.SetFee("ETH/USD", kraken.TradeVolumeFee{Fee: fee, Minfee: fee, Maxfee: fee})
		price.Update(&kraken.TickerData{
			Symbol: "ETH/USD",
			Ask:    decimal.NewFromFloat64(2500),
			Bid:    decimal.NewFromFloat64(2499),
		})

		qty, err := price.Quantity("ETH/USD", decimal.NewFromFloat64(0.193))
		So(qty, ShouldBeNil)
		So(err, ShouldNotBeNil)
		So(errnie.IsValidation(err), ShouldBeTrue)
		So(IsEnterSoftFail(err), ShouldBeTrue)
	})
}

func TestSizeToAvailableCashNotTotal(t *testing.T) {
	Convey("Enter spends a fraction of Available, never of reserved Total", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		prevFrac := viper.GetFloat64("trading.allocation.max_fraction")
		viper.Set("trading.allocation.max_fraction", 1.0)
		defer viper.Set("trading.allocation.max_fraction", prevFrac)

		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"ETH": {AltName: "ETH"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"ETH/USD": {
					WSName:        "ETH/USD",
					Base:          "ETH",
					Quote:         "USD",
					LotDecimals:   8,
					LotMultiplier: 1,
				},
			},
		})

		inst := &Instrument{cache: &sync.Map{}, quote: "USD"}
		inst.Cache([]kraken.InstrumentPair{{
			Symbol:  "ETH/USD",
			Base:    "ETH",
			Quote:   "USD",
			Status:  "online",
			QtyMin:  decimal.NewFromFloat64(0.0001),
			CostMin: decimal.NewFromFloat64(0.5),
		}})

		price := NewPrice(ctx, nil, nil, inst, normalizer)
		fee := decimal.NewFromFloat64(0.26)
		price.SetFee("ETH/USD", kraken.TradeVolumeFee{Fee: fee, Minfee: fee, Maxfee: fee})
		price.Update(&kraken.TickerData{
			Symbol: "ETH/USD",
			Ask:    decimal.NewFromFloat64(2500),
			Bid:    decimal.NewFromFloat64(2499),
		})
		price.Transition(runtime.READY)

		balance := NewBalance(ctx, nil)
		balance.Quote = "USD"
		// Total ~1000 reserved; only ~50 Available — must size to ~50 not ~1000.
		balance.UpdateWallet(&kraken.Balance{
			Data: []kraken.BalanceData{{
				Asset:     "USD",
				Balance:   decimal.NewFromFloat64(1000),
				Available: decimal.NewFromFloat64(50),
				Reserved:  decimal.NewFromFloat64(950),
			}},
		})

		probe := &orderProbe{}
		exec := NewExecution(ctx, probe, price, balance)
		exec.Transition(runtime.READY)

		reg, err := exec.Enter("ETH/USD")
		So(err, ShouldBeNil)
		So(reg, ShouldNotBeNil)
		So(probe.writes, ShouldEqual, 1)
		So(exec.Status(), ShouldEqual, runtime.READY)

		// Volume notional (pre-fee ballpark) must fit in Available, not Total.
		So(reg.Pending, ShouldNotBeNil)
		vol, perr := decimal.NewFromString(reg.Pending.Volume)
		So(perr, ShouldBeNil)
		notional := vol.Mul(decimal.NewFromFloat64(2500))
		So(notional.Cmp(decimal.NewFromFloat64(55)), ShouldEqual, -1)
		So(notional.Cmp(decimal.NewFromFloat64(100)), ShouldEqual, -1)
	})
}
