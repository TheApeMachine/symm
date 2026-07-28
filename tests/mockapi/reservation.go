package mockapi

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	balancesfixture "github.com/theapemachine/symm/tests/fixtures/balances"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
)

/*
available returns spendable ledger quantity after all resting reservations.
*/
func (paper *Paper) available(asset string) float64 {
	return paper.balances[asset] - paper.reserved[asset]
}

/*
fee selects the fee side implied by immediate or resting execution.
*/
func (paper *Paper) fee(order paperOrder) float64 {
	if order.maker {
		return paper.options.MakerFee
	}

	return paper.options.TakerFee
}

/*
reserve locks the exact asset required to fund one non-crossing limit order.
*/
func (paper *Paper) reserve(order paperOrder) paperOrder {
	base, quote, _ := strings.Cut(order.symbol, "/")
	order.reserveAsset = base
	order.reserve = order.quantity

	if order.side == "buy" {
		order.reserveAsset = quote
		order.reserve = order.quantity * order.limit * (1 + paper.options.MakerFee)
	}

	paper.reserved[order.reserveAsset] += order.reserve
	return order
}

/*
release unlocks the exact reservation before applying a resting fill.
*/
func (paper *Paper) release(order paperOrder) {
	paper.reserved[order.reserveAsset] -= order.reserve
}

/*
balanceFrame renders total, available, and reserved balances with one monotonic
private sequence shared by snapshots and updates.
*/
func (paper *Paper) balanceFrame(typ balancesfixture.FixtureType) []byte {
	paper.nextBalance++
	return balancesfixture.Frame(
		paper.balances,
		paper.reserved,
		typ,
		paper.nextBalance,
	)
}

/*
BalanceSnapshot returns the paper ledger's current authoritative wallet state.
*/
func (paper *Paper) BalanceSnapshot() []byte {
	paper.mu.Lock()
	defer paper.mu.Unlock()
	return paper.balanceFrame(balancesfixture.SNAPSHOT)
}

/*
ExecutionSnapshot returns the paper venue's current open-order projections plus
executed trade history so private resubscription matches Kraken's
snap_orders+snap_trades surface more closely.
*/
func (paper *Paper) ExecutionSnapshot() []byte {
	paper.mu.Lock()
	defer paper.mu.Unlock()
	options := make([]executionfixture.Options, 0, len(paper.order)+len(paper.history))
	orderIDs := append([]string(nil), paper.order...)
	sort.Strings(orderIDs)

	for _, orderID := range orderIDs {
		order, exists := paper.open[orderID]

		if !exists {
			continue
		}

		options = append(options, executionfixture.Options{
			OrderID:     order.id,
			ExecID:      fmt.Sprintf("OPEN-%s", order.id),
			Symbol:      order.symbol,
			Side:        order.side,
			LastQty:     "0",
			LastPrice:   "0",
			Cost:        "0",
			OrderStatus: "open",
			OrderType:   order.typ,
			ExecType:    "new",
			CumQty:      "0",
			CumCost:     "0",
			AvgPrice:    "0",
			Timestamp:   paper.options.Now().Format(time.RFC3339Nano),
		})
	}

	historyIDs := make([]string, 0, len(paper.history))

	for execID := range paper.history {
		historyIDs = append(historyIDs, execID)
	}

	sort.Slice(historyIDs, func(left, right int) bool {
		leftTrade := paper.history[historyIDs[left]]
		rightTrade := paper.history[historyIDs[right]]

		if leftTrade.Time == nil && rightTrade.Time == nil {
			return historyIDs[left] < historyIDs[right]
		}

		if leftTrade.Time == nil {
			return true
		}

		if rightTrade.Time == nil {
			return false
		}

		if diff := leftTrade.Time.Cmp(rightTrade.Time); diff != 0 {
			return diff < 0
		}

		return historyIDs[left] < historyIDs[right]
	})

	for _, execID := range historyIDs {
		trade := paper.history[execID]

		if trade.Price == nil || trade.Volume == nil || trade.Cost == nil {
			continue
		}

		fee := "0"

		if trade.Fee != nil {
			fee = trade.Fee.String()
		}

		timestamp := paper.options.Now().Format(time.RFC3339Nano)

		if trade.Time != nil {
			seconds, _ := strconv.ParseFloat(trade.Time.String(), 64)
			timestamp = time.Unix(0, int64(seconds*float64(time.Second))).UTC().Format(time.RFC3339Nano)
		}

		options = append(options, executionfixture.Options{
			OrderID:     trade.OrderID,
			ExecID:      execID,
			Symbol:      trade.Pair,
			Side:        strings.ToLower(trade.Type),
			LastQty:     trade.Volume.String(),
			LastPrice:   trade.Price.String(),
			Cost:        trade.Cost.String(),
			OrderStatus: "filled",
			OrderType:   trade.OrderType,
			ExecType:    "trade",
			CumQty:      trade.Volume.String(),
			CumCost:     trade.Cost.String(),
			AvgPrice:    trade.Price.String(),
			FeeUsdEquiv: fee,
			Timestamp:   timestamp,
		})
	}

	return executionfixture.Snapshot(options)
}

/*
snapshot converts one resting instruction into Kraken reconciliation data.
*/
func (order paperOrder) snapshot() spot.Order {
	quantity := decimal.NewFromFloat64(order.quantity)
	price := decimal.NewFromFloat64(order.limit)
	zero := decimal.NewFromInt64(0)

	return spot.Order{
		Status: "open",
		Description: &spot.OrderDescription{
			Pair: order.symbol, Type: order.side, OrderType: order.typ, Price: price,
		},
		Volume: quantity, VolumeExecuted: zero, Price: price, LimitPrice: price,
	}
}
