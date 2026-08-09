package tests

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"

	"github.com/theapemachine/symm/tests/fixtures/execution"
	testtypes "github.com/theapemachine/symm/tests/types"
)

type executionOrder struct {
	order                                                           simulatedOrder
	submittedAt, acknowledgeAt, executeAt, expireAt, nextFragmentAt time.Time
	outcome                                                         testtypes.OrderOutcome
	acknowledged, triggered, terminal                               bool
	terminalState                                                   string
	cumulativeQuantity, cumulativeCost, cumulativeFee               float64
	fragmentsRemaining                                              int
	executionIDs                                                    []string
}

/* executionModel owns simulated liquidity, lifecycles, and reconciliation. */
type executionModel struct {
	config       testtypes.ExecutionConfig
	profiles     map[testtypes.MarketState]testtypes.RegimeProfile
	private      *Conn
	book         *executionBook
	ledger       *executionLedger
	rng          *rand.Rand
	active       map[string][]*executionOrder
	orders       []*executionOrder
	outcomeIndex int
	executionSeq int64
	mechanics    MechanicsReport
}

func newExecutionModel(
	config testtypes.ExecutionConfig,
	profiles map[testtypes.MarketState]testtypes.RegimeProfile,
	symbols []*testtypes.Symbol,
	private *Conn,
	seed int64,
) *executionModel {
	return &executionModel{
		config:   config,
		profiles: testtypes.CloneProfiles(profiles),
		private:  private,
		book:     newExecutionBook(config, symbols),
		ledger:   newExecutionLedger(config, private),
		rng:      rand.New(rand.NewSource(seed)),
		active:   make(map[string][]*executionOrder, len(symbols)),
	}
}

/* Process advances one symbol's orders and delayed balance projections. */
func (model *executionModel) Process(
	sample testtypes.Sample,
	state testtypes.MarketState,
) {
	model.ledger.ApplyBalances(sample.Timestamp)
	model.accept(sample, state)
	orders := model.active[sample.Symbol]
	remaining := orders[:0]

	for _, order := range orders {
		model.process(order, sample)

		awaitingAcknowledgement := model.config.EmitAcknowledgements &&
			model.config.ExecutionBeforeAcknowledgment && !order.acknowledged

		if !order.terminal || awaitingAcknowledgement {
			remaining = append(remaining, order)
		}
	}

	model.active[sample.Symbol] = remaining
	model.ledger.ApplyBalances(sample.Timestamp)
}

func (model *executionModel) accept(
	sample testtypes.Sample,
	state testtypes.MarketState,
) {
	for _, accepted := range model.private.transport.takeOrders(sample.Symbol) {
		order := &executionOrder{
			order:              accepted,
			submittedAt:        sample.Timestamp,
			acknowledgeAt:      sample.Timestamp.Add(model.config.AcknowledgementDelay),
			executeAt:          sample.Timestamp.Add(model.config.ExecutionDelay),
			nextFragmentAt:     sample.Timestamp.Add(model.config.ExecutionDelay),
			outcome:            model.outcome(),
			fragmentsRemaining: 1,
		}

		if model.config.MaximumOrderQuantity > 0 &&
			accepted.Quantity > model.config.MaximumOrderQuantity {
			order.outcome = testtypes.OrderReject
		}

		if order.outcome == testtypes.OrderFill &&
			model.rng.Float64() < model.config.PartialFillProb {
			order.fragmentsRemaining = model.fragmentCount()
		}

		if model.config.ExpireAfter > 0 {
			order.expireAt = sample.Timestamp.Add(model.config.ExpireAfter)
		}

		model.mechanics.Submitted++
		model.ledger.Ordered(accepted.Quantity)
		model.orders = append(model.orders, order)
		model.active[sample.Symbol] = append(model.active[sample.Symbol], order)
		profile, known := model.profiles[state]

		if !known {
			panic("simulator: execution received an unknown market state")
		}

		if accepted.Request.Type == "buy" && !profile.AdmitsLong {
			model.mechanics.FalsePositiveEntries++
		}
	}
}

func (model *executionModel) outcome() testtypes.OrderOutcome {
	if model.outcomeIndex < len(model.config.Outcomes) {
		outcome := model.config.Outcomes[model.outcomeIndex]
		model.outcomeIndex++

		return outcome
	}

	draw := model.rng.Float64()

	if draw < model.config.RejectionProb {
		return testtypes.OrderReject
	}

	if draw < model.config.RejectionProb+model.config.CancellationProb {
		return testtypes.OrderCancel
	}

	if draw < model.config.RejectionProb+model.config.CancellationProb+
		model.config.NoFillProb {
		return testtypes.OrderNoFill
	}

	return testtypes.OrderFill
}

func (model *executionModel) fragmentCount() int {
	limit := math.Exp(-model.config.MeanFragmentCount)
	product := 1.0
	fragments := 0

	for product > limit {
		fragments++
		product *= model.rng.Float64()
	}

	return max(2, fragments-1)
}

func (model *executionModel) process(
	order *executionOrder,
	sample testtypes.Sample,
) {
	if order.terminal {
		if model.config.EmitAcknowledgements && !order.acknowledged &&
			!sample.Timestamp.Before(order.acknowledgeAt) {
			model.acknowledge(order, sample)
		}

		return
	}

	if order.outcome == testtypes.OrderReject &&
		!sample.Timestamp.Before(order.executeAt) {
		model.terminal(order, sample, "rejected", "rejected")
		return
	}

	if model.config.EmitAcknowledgements && !order.acknowledged &&
		!sample.Timestamp.Before(order.acknowledgeAt) {
		model.acknowledge(order, sample)
	}

	if order.outcome == testtypes.OrderCancel &&
		!sample.Timestamp.Before(order.executeAt) {
		model.terminal(order, sample, "canceled", "canceled")
		return
	}

	if order.outcome == testtypes.OrderExpire &&
		!sample.Timestamp.Before(order.executeAt) {
		model.terminal(order, sample, "expired", "expired")
		return
	}

	if !order.expireAt.IsZero() && !sample.Timestamp.Before(order.expireAt) {
		model.terminal(order, sample, "expired", "expired")
		return
	}

	if order.outcome == testtypes.OrderNoFill ||
		sample.Timestamp.Before(order.executeAt) ||
		sample.Timestamp.Before(order.nextFragmentAt) {
		return
	}

	if !model.config.ExecutionBeforeAcknowledgment &&
		model.config.EmitAcknowledgements && !order.acknowledged {
		return
	}

	if !model.executable(order, sample) {
		return
	}

	model.fill(order, sample)
}

func (model *executionModel) acknowledge(
	order *executionOrder,
	sample testtypes.Sample,
) {
	order.acknowledged = true
	model.mechanics.Acknowledged++
	model.publish(order, sample, 0, 0, "open", "new")
}

func (model *executionModel) executable(
	order *executionOrder,
	sample testtypes.Sample,
) bool {
	switch order.order.Request.OrderType {
	case "market":
		return true
	case "limit":
		if order.order.Request.Type == "buy" {
			return sample.Ask <= order.order.Price &&
				model.rng.Float64() < model.config.LimitFillProb
		}

		return sample.Bid >= order.order.Price &&
			model.rng.Float64() < model.config.LimitFillProb
	case "stop-loss", "stop-loss-limit":
		if !order.triggered && order.order.Request.Type == "sell" {
			order.triggered = sample.Last <= order.order.Price
		}

		if !order.triggered && order.order.Request.Type == "buy" {
			order.triggered = sample.Last >= order.order.Price
		}

		if !order.triggered || order.order.Request.OrderType == "stop-loss" {
			return order.triggered
		}

		if order.order.Request.Type == "buy" {
			return sample.Ask <= order.order.Price2 &&
				model.rng.Float64() < model.config.LimitFillProb
		}

		return sample.Bid >= order.order.Price2 &&
			model.rng.Float64() < model.config.LimitFillProb
	}

	return false
}

func (model *executionModel) fill(
	order *executionOrder,
	sample testtypes.Sample,
) {
	remaining := order.order.Quantity - order.cumulativeQuantity
	fragmentLimit := remaining

	if order.fragmentsRemaining > 1 {
		fragmentLimit = remaining / float64(order.fragmentsRemaining)
	}

	quantity, cost := model.book.Consume(order, sample, fragmentLimit)

	if quantity <= 0 {
		return
	}

	fee, err := model.private.transport.orderFee(
		order.order.Request.Pair,
		cost,
		order.order.Request.OrderType != "market" &&
			order.order.Request.OrderType != "stop-loss",
	)

	if err != nil {
		model.mechanics.InvariantViolations = append(
			model.mechanics.InvariantViolations,
			err.Error(),
		)
		model.terminal(order, sample, "rejected", "rejected")
		return
	}

	if !model.ledger.Affordable(order, quantity, cost, fee) {
		model.terminal(order, sample, "rejected", "rejected")
		return
	}

	order.cumulativeQuantity += quantity
	order.cumulativeCost += cost
	order.cumulativeFee += fee
	order.fragmentsRemaining = max(1, order.fragmentsRemaining-1)
	order.nextFragmentAt = sample.Timestamp.Add(model.config.FragmentDelay)
	model.ledger.ApplyFill(order, sample, quantity, cost, fee)
	status := "partially_filled"

	if order.cumulativeQuantity >= order.order.Quantity {
		status = "filled"
		order.terminal = true
		order.terminalState = status
		model.mechanics.Filled++
	} else {
		model.mechanics.PartiallyFilled++
	}

	model.publish(order, sample, quantity, cost, status, "trade")
}

func (model *executionModel) terminal(
	order *executionOrder,
	sample testtypes.Sample,
	status string,
	execType string,
) {
	order.terminal = true
	order.terminalState = status

	switch status {
	case "canceled":
		model.mechanics.Canceled++
	case "rejected":
		model.mechanics.Rejected++
	case "expired":
		model.mechanics.Expired++
	}

	model.publish(order, sample, 0, 0, status, execType)
}

func (model *executionModel) publish(
	order *executionOrder,
	sample testtypes.Sample,
	lastQuantity float64,
	lastCost float64,
	status string,
	execType string,
) {
	model.executionSeq++
	executionID := fmt.Sprintf("%s-exec-%06d", order.order.ID, model.executionSeq)
	order.executionIDs = append(order.executionIDs, executionID)
	lastPrice := 0.0

	if lastQuantity > 0 {
		lastPrice = lastCost / lastQuantity
	}

	averagePrice := 0.0

	if order.cumulativeQuantity > 0 {
		averagePrice = order.cumulativeCost / order.cumulativeQuantity
	}

	model.private.Publish("executions", execution.Frame(execution.Options{
		OrderID:       order.order.ID,
		ClientOrderID: order.order.Request.ClOrdId,
		ExecID:        executionID,
		Symbol:        order.order.Request.Pair,
		Side:          order.order.Request.Type,
		LastQty:       format(lastQuantity),
		LastPrice:     format(lastPrice),
		Cost:          format(lastCost),
		OrderStatus:   status,
		OrderType:     order.order.Request.OrderType,
		ExecType:      execType,
		CumQty:        format(order.cumulativeQuantity),
		CumCost:       format(order.cumulativeCost),
		AvgPrice:      format(averagePrice),
		FeeUsdEquiv:   format(order.cumulativeFee),
		Timestamp:     sample.Timestamp.Format(time.RFC3339Nano),
		Sequence:      model.executionSeq,
	}))
}

func format(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
