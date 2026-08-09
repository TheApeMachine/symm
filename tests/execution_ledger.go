package tests

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/theapemachine/symm/tests/fixtures/balances"
	testtypes "github.com/theapemachine/symm/tests/types"
)

type balanceProjection struct {
	balances map[string]float64
	webAt    time.Time
	restAt   time.Time
	webDone  bool
	restDone bool
}

type inventoryBasis struct {
	quantity float64
	cost     float64
}

/*
executionLedger owns exchange balances, delayed account projections, fees,
cost basis, realized PnL, and drawdown.
*/
type executionLedger struct {
	config       testtypes.ExecutionConfig
	private      *Conn
	balances     map[string]float64
	projections  []balanceProjection
	basis        map[string]inventoryBasis
	economics    EconomicsReport
	realizedPeak float64
	sequence     int64
}

func newExecutionLedger(
	config testtypes.ExecutionConfig,
	private *Conn,
) *executionLedger {
	return &executionLedger{
		config:   config,
		private:  private,
		balances: private.transport.accountBalances(),
		basis:    map[string]inventoryBasis{},
	}
}

/*
Ordered records accepted quantity as the denominator of the fill ratio.
*/
func (ledger *executionLedger) Ordered(quantity float64) {
	ledger.economics.OrderedQuantity += quantity
}

/*
Affordable enforces exchange-side inventory before a fill changes state.
*/
func (ledger *executionLedger) Affordable(
	order *executionOrder,
	quantity float64,
	cost float64,
	fee float64,
) bool {
	if !ledger.config.EnforceBalances {
		return true
	}

	base, quote, known := splitPair(order.order.Request.Pair)

	if !known {
		return false
	}

	if order.order.Request.Type == "buy" {
		return ledger.balances[quote] >= cost+fee
	}

	return ledger.balances[base] >= quantity
}

/*
ApplyFill reconciles one execution against balances and cost basis.
*/
func (ledger *executionLedger) ApplyFill(
	order *executionOrder,
	sample testtypes.Sample,
	quantity float64,
	cost float64,
	fee float64,
) {
	base, quote, _ := splitPair(order.order.Request.Pair)
	basis := ledger.basis[order.order.Request.Pair]
	ledger.economics.ExecutedQuantity += quantity
	ledger.economics.Fees += fee
	topPrice := sample.Ask

	if order.order.Request.Type == "sell" {
		topPrice = sample.Bid
	}

	ledger.economics.Slippage += math.Abs(cost - quantity*topPrice)

	if order.order.Request.Type == "buy" {
		ledger.balances[quote] -= cost + fee
		ledger.balances[base] += quantity
		basis.quantity += quantity
		basis.cost += cost
		ledger.basis[order.order.Request.Pair] = basis
		ledger.project(sample.Timestamp)
		ledger.reconcile()
		return
	}

	ledger.balances[base] -= quantity
	ledger.balances[quote] += cost - fee
	entryCost := 0.0

	if basis.quantity > 0 {
		entryCost = basis.cost * quantity / basis.quantity
		basis.quantity -= quantity
		basis.cost -= entryCost
		ledger.basis[order.order.Request.Pair] = basis
	}

	ledger.economics.GrossPnL += cost - entryCost
	ledger.project(sample.Timestamp)
	ledger.reconcile()
}

func (ledger *executionLedger) reconcile() {
	ledger.economics.NetPnL = ledger.economics.GrossPnL - ledger.economics.Fees
	ledger.realizedPeak = max(ledger.realizedPeak, ledger.economics.NetPnL)
	drawdown := ledger.realizedPeak - ledger.economics.NetPnL
	ledger.economics.MaximumDrawdown = max(
		ledger.economics.MaximumDrawdown,
		drawdown,
	)
}

func (ledger *executionLedger) project(at time.Time) {
	ledger.projections = append(ledger.projections, balanceProjection{
		balances: copyBalances(ledger.balances),
		webAt:    at.Add(ledger.config.BalanceDelay),
		restAt:   at.Add(ledger.config.RESTBalanceDelay),
	})
}

/*
ApplyBalances advances websocket and REST account truth on independent clocks.
*/
func (ledger *executionLedger) ApplyBalances(at time.Time) {
	pending := ledger.projections[:0]

	for index := range ledger.projections {
		projection := &ledger.projections[index]

		if !projection.restDone && !at.Before(projection.restAt) {
			ledger.private.transport.setBalances(projection.balances)
			projection.restDone = true
		}

		if !projection.webDone && !at.Before(projection.webAt) {
			ledger.sequence++
			ledger.private.Publish("balances", balances.Frame(
				projection.balances,
				map[string]float64{},
				balances.UPDATE,
				ledger.sequence,
			))
			projection.webDone = true
		}

		if !projection.webDone || !projection.restDone {
			pending = append(pending, *projection)
		}
	}

	ledger.projections = pending
}

/*
Report returns detached economics with its current fill ratio.
*/
func (ledger *executionLedger) Report() EconomicsReport {
	report := ledger.economics

	if report.OrderedQuantity > 0 {
		report.FillRatio = report.ExecutedQuantity / report.OrderedQuantity
	}

	return report
}

/*
Validate rejects any exchange-side negative balance.
*/
func (ledger *executionLedger) Validate() error {
	for asset, balance := range ledger.balances {
		if balance < 0 {
			return fmt.Errorf("simulator: %s balance is negative", asset)
		}
	}

	return nil
}

func splitPair(pair string) (string, string, bool) {
	assets := strings.Split(pair, "/")

	if len(assets) != 2 || assets[0] == "" || assets[1] == "" {
		return "", "", false
	}

	return assets[0], assets[1], true
}

func copyBalances(source map[string]float64) map[string]float64 {
	copy := make(map[string]float64, len(source))

	for asset, balance := range source {
		copy[asset] = balance
	}

	return copy
}
