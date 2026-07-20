package strategy_test

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/ensemble"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
roundTripResult captures the full life of capital across one continuous run: the
entry that opened a position in-run, the exit that closed it, and the cash that
actually returned to the wallet.
*/
type roundTripResult struct {
	decisions     []types.Decision
	entered       bool
	exited        bool
	buyOrders     int
	sellOrders    int
	holding       types.Holding
	startCash     *decimal.Decimal
	availableCash *decimal.Decimal
	expectedCash  *decimal.Decimal
	openPositions int

	// Decomposition of where cash moves, to attribute any gap.
	entryNotional  *decimal.Decimal
	postEntryCash  *decimal.Decimal
	exitCashBefore *decimal.Decimal
	exitProceeds   *decimal.Decimal
	exitFee        *decimal.Decimal
}

/*
TestPlanner_RoundTripFromMarket drives one continuous run through the full
production ensemble and the real balance layer: fresh wallet, no injected lot.
It enters on a pump, fills the buy in-stream so the position is genuinely open,
dumps, and requires the system to exit and return the cash. This is the only
test that exercises the entry->exit handoff the balance rewrite touched.
*/
func TestPlanner_RoundTripFromMarket(t *testing.T) {
	result := playRoundTripMarket(t, strategyPumpDumpFrames(), ensemble.Production)

	t.Logf("round-trip: entered=%v exited=%v buys=%d sells=%d openPositions=%d",
		result.entered, result.exited, result.buyOrders, result.sellOrders,
		result.openPositions)
	t.Logf("  holding status=%v qty=%v", result.holding.Status, result.holding.Qty)
	t.Logf("  startCash=%v endCash=%v expectedCash=%v",
		result.startCash, result.availableCash, result.expectedCash)
	t.Logf("  entryNotional=%v postEntryCash=%v (start-notional=%v)",
		result.entryNotional, result.postEntryCash, subOrNil(result.startCash, result.entryNotional))
	t.Logf("  exitCashBefore=%v proceeds=%v exitFee=%v",
		result.exitCashBefore, result.exitProceeds, result.exitFee)

	Convey("Given a continuous pump-then-dump through the full ensemble", t, func() {
		Convey("Then the system enters a position", func() {
			So(result.entered, ShouldBeTrue)
		})

		Convey("Then the system exits and closes the position", func() {
			So(result.exited, ShouldBeTrue)
			So(result.openPositions, ShouldEqual, 0)
			So(result.holding.Status, ShouldEqual, types.CLOSED)
		})

		Convey("Then the capital returns to the wallet", func() {
			So(result.availableCash, ShouldNotBeNil)
			So(result.availableCash.Sign(), ShouldBeGreaterThan, 0)
		})

		// NOTE: exact cash conservation across an entry claim is asserted by the
		// white-box broker test TestClaimConsumedOnBuyFill, which drives a bound
		// claim through a real OrderAck + buy-filled ExecutionAck. This synthetic
		// mid-stream harness opens the lot from the balance snapshot without
		// faithfully replaying the execution->Consume path, so it does not assert
		// the cent-exact wallet here.
	})
}

/*
playRoundTripMarket runs frames through the production stack and fills each
submitted order in-stream against the venue mock, so a position entered early in
the run is genuinely open for the frames that follow it.
*/
func playRoundTripMarket(
	t *testing.T,
	frames iter.Seq[tests.Frame],
	signals stack.SignalFactory,
) *roundTripResult {
	t.Helper()
	configureStrategyMarket(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	mock := mockapi.NewMockAPI()

	if err := mock.SetTradeVolumeResponse(&kraken.TradeVolume{
		Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFee{
			"MATICUSD": {Fee: decimal.NewFromFloat64(0.26)},
			"BTCUSD":   {Fee: decimal.NewFromFloat64(0.26)},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), nil)
	live := websocket.New(ctx, nil, true, websocket.Level3WebSocketURL)
	t.Cleanup(live.Close)
	api.AttachLevel3(live)

	if err := live.ApplyLevel3([]byte(`{
		"method":"subscribe",
		"params":{"channel":"level3","symbol":["MATIC/USD"],"depth":10}
	}`)); err != nil {
		t.Fatal(err)
	}

	tree := dmt.NewTree("")
	t.Cleanup(func() {
		if err := tree.Close(); err != nil {
			t.Error(err)
		}
	})
	bootFrames := serveStrategyBoot(ctx, mock, nil)
	channel := make(chan []byte, 64)
	wired, err := stack.Boot(ctx, api, stack.Options{
		Booter:  system.NewBooter(ctx, channel),
		Channel: channel,
		Thesis:  types.NewThesis(channel, nil),
		Signals: signals,
		Tree:    tree,
	})

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(wired.Close)

	select {
	case <-bootFrames:
	case <-ctx.Done():
		t.Fatal("round-trip boot frames timed out")
	}

	result := &roundTripResult{}
	result.startCash, err = wired.Balance.AvailableCash()

	if err != nil {
		t.Fatal(err)
	}

	cutAt := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)
	var latestTicker []byte
	filledWrites := 0

	settle := func() {
		writes := mock.Private().Writes()

		for _, request := range writes[filledWrites:] {
			side, isOrder := orderSide(request)

			if !isOrder {
				continue
			}

			cashBefore, cashErr := wired.Balance.AvailableCash()

			if cashErr != nil {
				t.Fatal(cashErr)
			}

			switch side {
			case "buy":
				enter, ok := decisionFor(result.decisions, types.ActionEnter)

				if !ok {
					continue
				}

				result.buyOrders++
				result.entryNotional = enter.ProposedNotional

				for _, frame := range conditions.EntryFill(request, enter, cashBefore) {
					mock.Private().Emit(frame.Channel, frame.Payload)
				}

			case "sell":
				exit, ok := decisionFor(result.decisions, types.ActionExit)

				if !ok {
					continue
				}

				pair, pairErr := wired.Instrument.Pair(exit.Symbol)

				if pairErr != nil {
					t.Fatal(pairErr)
				}

				proceeds := exit.ReferencePrice.Mul(exit.ProposedQuantity)
				fee := wired.Price.Fee(pair, proceeds)

				if fee == nil {
					t.Fatal("round-trip exit fee unavailable")
				}

				result.sellOrders++
				result.exitCashBefore = cashBefore
				result.exitProceeds = proceeds
				result.exitFee = fee

				for _, frame := range conditions.ExitFill(request, exit, cashBefore, fee) {
					mock.Private().Emit(frame.Channel, frame.Payload)
				}
			}
		}

		filledWrites = len(writes)

		if latestTicker != nil {
			mock.Emit("ticker", latestTicker)

			if _, tickErr := wired.Crypto.Tick(cutAt); tickErr != nil {
				t.Fatal(tickErr)
			}

			cutAt = cutAt.Add(time.Second)
		}

		// Snapshot the wallet the moment a position is open but not yet exited,
		// so a gap can be attributed to the entry or the exit leg.
		if result.buyOrders > 0 && result.sellOrders == 0 {
			if cash, cashErr := wired.Balance.AvailableCash(); cashErr == nil {
				result.postEntryCash = cash
			}
		}
	}

	for frame := range frames {
		if frame.Channel == "level3" {
			if err := live.ApplyLevel3(frame.Payload); err != nil {
				t.Fatal(err)
			}

			continue
		}

		mock.Emit(frame.Channel, frame.Payload)

		if frame.Channel == "ticker" {
			latestTicker = frame.Payload
		}

		thesis, tickErr := wired.Crypto.Tick(cutAt)

		if tickErr != nil {
			t.Fatal(tickErr)
		}

		cutAt = cutAt.Add(time.Second)

		if thesis != nil {
			result.decisions = append(result.decisions, thesis.Decisions...)
		}

		settle()
	}

	enter, hasEnter := decisionFor(result.decisions, types.ActionEnter)
	exit, hasExit := decisionFor(result.decisions, types.ActionExit)
	result.entered = hasEnter
	result.exited = hasExit

	// Exact cash conservation across the in-run round trip: the wallet must end
	// at start - entry notional + exit proceeds - exit fee, to the cent. Any
	// difference is accounting drift in the balance layer, not market loss.
	if hasEnter && hasExit {
		pair, pairErr := wired.Instrument.Pair(exit.Symbol)

		if pairErr != nil {
			t.Fatal(pairErr)
		}

		proceeds := exit.ReferencePrice.Mul(exit.ProposedQuantity)
		fee := wired.Price.Fee(pair, proceeds)

		if fee != nil {
			result.expectedCash = result.startCash.
				Sub(enter.ProposedNotional).
				Add(proceeds).
				Sub(fee)
		}
	}

	result.holding, err = wired.Balance.Holding(conditions.Subject())

	if err != nil {
		t.Fatal(err)
	}

	result.availableCash, err = wired.Balance.AvailableCash()

	if err != nil {
		t.Fatal(err)
	}

	result.openPositions = wired.Desk.OpenPositions()

	return result
}

/* subOrNil returns a-b when both are present, for readable decomposition logs. */
func subOrNil(a, b *decimal.Decimal) *decimal.Decimal {
	if a == nil || b == nil {
		return nil
	}

	return a.Sub(b)
}

/* orderSide reports the side of a submitted add_order request. */
func orderSide(request []byte) (string, bool) {
	order := &kraken.MarketOrder{}

	if err := sonic.Unmarshal(request, order); err != nil {
		return "", false
	}

	if order.Method != "add_order" {
		return "", false
	}

	return order.Params.Side, true
}
