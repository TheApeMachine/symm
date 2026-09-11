package strategy

import (
	"bytes"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
TradeOutcome records one completed simulated trade leg with its duration
and return in basis points.
*/
type TradeOutcome struct {
	Symbol     string
	EntryPrice *decimal.Decimal
	ExitPrice  *decimal.Decimal
	Quantity   *decimal.Decimal
	Profit     *decimal.Decimal
	ReturnBp   float64
	EntryAt    time.Time
	ExitAt     time.Time
}

/*
ActionDecision represents the single shared model's learned evaluation for
a given context: the chosen action and its measured evidence strength.
*/
const maxConcurrentPositions = 5

type ActionDecision struct {
	Action     Action
	Context    []byte
	Confidence float64
	Contrast   float64
	Support    uint64
}

/*
MainAgent executes simulated trading (forward testing) based on the precursor
model developed by the parallel learning agents.

It owns its wallet, cash, equity, fees, positions, and forward evaluation.
Once robustly net-positive in simulated trading, it switches to paper or real
trading according to configuration.
*/
type MainAgent struct {
	mu            sync.RWMutex
	id            int
	status        string // "simulated" or "trading"
	targetAccount string // "paper" or "real"
	instrument    *broker.Instrument
	price         *broker.Price
	engine        *cognition.Engine

	symbolPhase    map[string]string
	symbolMaturity map[string]int
	entryContexts  map[string][]byte

	initial    *decimal.Decimal
	cash       *decimal.Decimal
	equity     *decimal.Decimal
	fees       *decimal.Decimal
	profit     *decimal.Decimal
	realized   *decimal.Decimal
	unrealized *decimal.Decimal
	wealth     float64

	positions     map[string]*types.Holding
	posQuantities map[string]*decimal.Decimal
	posCosts      map[string]*decimal.Decimal
	lastPrice     map[string]*decimal.Decimal

	decisions uint64
	fills     uint64
	wins      uint64
	losses    uint64

	outcomes   []TradeOutcome
	meanReturn float64
	m2Return   float64
	variance   float64

	lastDecision *telemetry.LearningDecisionT
	alternatives []*telemetry.LearningActionT
}

func NewMainAgent(initialCash *decimal.Decimal, targetAccount string, deps ...any) *MainAgent {
	if initialCash == nil || initialCash.Sign() <= 0 {
		balance := int64(200)

		if system.Cfg != nil && system.Cfg.Market != nil && system.Cfg.Market.Balance > 0 {
			balance = int64(system.Cfg.Market.Balance)
		}

		initialCash = decimal.NewFromInt64(balance)
	}

	if targetAccount == "" {
		targetAccount = viper.GetString("trading.model")

		if targetAccount == "" {
			targetAccount = "paper"
		}
	}

	var inst *broker.Instrument
	var prc *broker.Price
	var eng *cognition.Engine

	for _, dep := range deps {
		switch v := dep.(type) {
		case *broker.Instrument:
			inst = v
		case *broker.Price:
			prc = v
		case *cognition.Engine:
			eng = v
		}
	}

	zero := decimal.NewFromInt64(0)

	return &MainAgent{
		id:             0,
		status:         "simulated",
		targetAccount:  targetAccount,
		instrument:     inst,
		price:          prc,
		engine:         eng,
		initial:        initialCash,
		cash:           initialCash,
		equity:         initialCash,
		fees:           zero,
		profit:         zero,
		realized:       zero,
		unrealized:     zero,
		positions:      make(map[string]*types.Holding),
		posQuantities:  make(map[string]*decimal.Decimal),
		posCosts:       make(map[string]*decimal.Decimal),
		lastPrice:      make(map[string]*decimal.Decimal),
		symbolPhase:    make(map[string]string),
		symbolMaturity: make(map[string]int),
		entryContexts:  make(map[string][]byte),
	}
}

func (agent *MainAgent) SetInstrument(instrument *broker.Instrument) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.instrument = instrument
}

func (agent *MainAgent) SetPrice(price *broker.Price) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.price = price
}

func (agent *MainAgent) SetEngine(engine *cognition.Engine) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.engine = engine
}

func (agent *MainAgent) feeRate(symbol string) *decimal.Decimal {
	if agent.price != nil {
		if fee := agent.price.FeeIfAvailable(symbol); fee != nil && fee.Fee != nil {
			percentMultiplier := decimal.NewFromFloat64(0.01)
			return fee.Fee.Mul(percentMultiplier)
		}
	}

	return nil
}

func (agent *MainAgent) ID() int {
	return agent.id
}

func (agent *MainAgent) Status() string {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	samples := agent.wins + agent.losses

	if agent.status == "simulated" && samples >= 20 {
		stdErr := math.Sqrt(agent.variance / float64(samples))

		if agent.meanReturn+2*stdErr < 0 {
			return "learning"
		}
	}

	return agent.status
}

func (agent *MainAgent) TargetAccount() string {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	return agent.targetAccount
}

func (agent *MainAgent) Graded() uint64 {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	return agent.wins + agent.losses
}

func (agent *MainAgent) canEnter(decision ActionDecision) bool {
	if decision.Action != ActionEnter {
		return false
	}

	if decision.Contrast <= 0 || decision.Support <= 1 {
		return false
	}
	samples := agent.wins + agent.losses

	if samples >= 20 {
		stdErr := math.Sqrt(agent.variance / float64(samples))

		if agent.meanReturn+2*stdErr < 0 {
			return false
		}
	}

	return true
}

/*
IsHolding reports whether a position is currently held for the symbol.
*/
func (agent *MainAgent) IsHolding(symbol string) bool {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	holding := agent.positions[symbol]

	return holding != nil && holding.Qty != nil && holding.Qty.Sign() > 0
}

/*
Step evaluates the learned action decision, manages simulated positions, and
executes forward-testing trades.
*/
func (agent *MainAgent) Step(envelope *types.Envelope, decision ActionDecision) {
	if envelope == nil {
		return
	}

	agent.mu.Lock()
	defer agent.mu.Unlock()

	symbol := envelope.Symbol()

	if symbol == "" {
		for _, measurement := range envelope.Measurements() {
			if measurement != nil && measurement.Label != "" {
				symbol = measurement.Label
				break
			}
		}
	}

	if symbol == "" {
		return
	}

	currentPrice := agent.extractPrice(envelope, symbol)
	now := time.Now().UTC()
	holding := agent.positions[symbol]

	if currentPrice != nil && currentPrice.Sign() > 0 {
		agent.lastPrice[symbol] = currentPrice

		// 1. Mark existing open positions to market
		agent.markPositions()

		// 2. Score candidate actions against learned decision
		agent.buildCandidates(symbol, decision)

		// 3. Evaluate trading action
		if holding == nil || holding.Qty == nil || holding.Qty.Sign() <= 0 {
			if decision.Action == ActionEnter {
				agent.symbolMaturity[symbol]++
			}
			if decision.Action != ActionEnter {
				agent.symbolMaturity[symbol] = 0
			}
			agent.symbolPhase[symbol] = string(decision.Action)

			// No position open: enter only when precursor model has developed positive skill/edge
			if len(agent.positions) < maxConcurrentPositions && agent.canEnter(decision) {
				agent.enterLong(envelope, symbol, currentPrice, decision, now)
			}
		}

		if holding != nil && holding.Qty != nil && holding.Qty.Sign() > 0 {
			shouldExit := false

			// Check venue status: exit if pair goes offline
			if agent.instrument != nil && agent.instrument.Has(symbol) {
				pair := agent.instrument.Pair(symbol)

				if pair.Status != "" && pair.Status != "online" {
					shouldExit = true
				}
			}

			// Exit when learner's policy chooses EXIT
			if !shouldExit && decision.Action == ActionExit {
				shouldExit = true
			}

			if shouldExit {
				agent.exitLong(envelope, symbol, currentPrice, now)
			}
		}

		// 4. Update overall portfolio valuation
		agent.updateValuation()

		// 5. Evaluate robustness threshold for paper/live promotion
		agent.evaluateRobustness()
	}

	// 6. Ensure continuous decision round telemetry on every evaluation step
	if envelope.StrategyRound == nil {
		agent.attachContinuousDecision(envelope, symbol, currentPrice, decision, holding, now)
	}
}

func (agent *MainAgent) attachContinuousDecision(
	envelope *types.Envelope,
	symbol string,
	price *decimal.Decimal,
	decision ActionDecision,
	holding *types.Holding,
	now time.Time,
) {
	action := types.ActionHold
	outcome := "hold"
	reason := "holding open position"

	if holding == nil || holding.Qty == nil || holding.Qty.Sign() <= 0 {
		action = types.ActionNothing
		outcome = "wait"
		reason = string(decision.Action)

		if reason == "" {
			reason = "scanning for precursor opportunity"
		}
	}

	if decision.Action == ActionEnter {
		action = types.ActionEnter
		outcome = "enter"
		reason = "precursor opportunity qualified"
	}

	if decision.Action == ActionExit && holding != nil && holding.Qty != nil && holding.Qty.Sign() > 0 {
		action = types.ActionExit
		outcome = "exit"
		reason = "learned exit trigger"
	}

	decisionRecord := &types.Decision{
		ID:               uuid.NewString(),
		Action:           action,
		Symbol:           symbol,
		At:               now,
		Direction:        0.0,
		ReferencePrice:   price,
		Confidence:       decision.Confidence,
		AvailableCapital: agent.cash,
		OpenPositions:    len(agent.positions),
		Cause:            "learned_policy",
		Reason:           reason,
	}

	envelope.StrategyRound = &types.StrategyRound{
		Symbol:    symbol,
		Evaluated: true,
		Outcome:   outcome,
		Decisions: []*types.Decision{decisionRecord},
	}
}

func (agent *MainAgent) extractPrice(envelope *types.Envelope, symbol string) *decimal.Decimal {
	if envelope.TickerData.Last != nil && envelope.TickerData.Last.Sign() > 0 {
		return envelope.TickerData.Last
	}

	if envelope.TickerData.Bid != nil && envelope.TickerData.Bid.Sign() > 0 {
		return envelope.TickerData.Bid
	}

	if cached, ok := agent.lastPrice[symbol]; ok && cached != nil && cached.Sign() > 0 {
		return cached
	}

	return nil
}

func (agent *MainAgent) markPositions() {
	unrealized := decimal.NewFromInt64(0)

	for symbol, holding := range agent.positions {
		qty := agent.posQuantities[symbol]
		cost := agent.posCosts[symbol]
		currentPrice := agent.lastPrice[symbol]

		if qty == nil || qty.Sign() <= 0 || currentPrice == nil || cost == nil {
			continue
		}
		marketValue := currentPrice.Mul(qty)
		pnl := marketValue.Sub(cost)
		holding.PnL = pnl

		if cost.Sign() > 0 {
			retPct := pnl.SetScale(decimal.DefaultScale).Div(cost).Mul(decimal.NewFromInt64(100))
			holding.ReturnPct = retPct.Float64()
		}
		unrealized = unrealized.Add(pnl)
	}
	agent.unrealized = unrealized
}

func (agent *MainAgent) enterLong(
	envelope *types.Envelope,
	symbol string,
	price *decimal.Decimal,
	decision ActionDecision,
	now time.Time,
) {
	// Allocate available cash across max concurrent position slots
	notional := agent.cash.Div(decimal.NewFromInt64(maxConcurrentPositions))

	// Validate position viability against venue facts via Instrument
	if agent.instrument != nil && agent.instrument.Has(symbol) {
		pair := agent.instrument.Pair(symbol)

		// Venue status check: must be online to trade
		if pair.Status != "" && pair.Status != "online" {
			return
		}

		// Venue minimum order cost (CostMin)
		if pair.CostMin != nil && pair.CostMin.Sign() > 0 && notional.Cmp(pair.CostMin) < 0 {
			notional = pair.CostMin
		}

		// Venue minimum order quantity (QtyMin)
		if pair.QtyMin != nil && pair.QtyMin.Sign() > 0 {
			minQtyNotional := pair.QtyMin.Mul(price)

			if notional.Cmp(minQtyNotional) < 0 {
				notional = minQtyNotional
			}
		}
	}

	if notional.Cmp(decimal.NewFromInt64(10)) < 0 {
		notional = decimal.NewFromInt64(10)
	}
	feeRate := agent.feeRate(symbol)

	if feeRate == nil {
		return
	}
	fee := notional.Mul(feeRate)
	totalCost := notional.Add(fee)

	if agent.cash.Cmp(totalCost) < 0 {
		return
	}
	quantity := notional.SetScale(decimal.DefaultScale).Div(price)

	if agent.instrument != nil && agent.instrument.Has(symbol) {
		pair := agent.instrument.Pair(symbol)

		if pair.QtyPrecision > 0 {
			quantity = quantity.SetScale(int64(pair.QtyPrecision))
		}

		if pair.QtyMin != nil && quantity.Cmp(pair.QtyMin) < 0 {
			return
		}
	}

	if quantity.Sign() <= 0 {
		return
	}

	agent.cash = agent.cash.Sub(totalCost)
	agent.fees = agent.fees.Add(fee)
	agent.fills++
	agent.decisions++

	holding := &types.Holding{
		Symbol:     symbol,
		Qty:        quantity,
		EntryPrice: price,
		EntryAt:    &now,
		EntryFee:   fee,
		PnL:        decimal.NewFromInt64(0),
	}
	agent.positions[symbol] = holding
	agent.posQuantities[symbol] = quantity
	agent.posCosts[symbol] = notional

	if len(decision.Context) > 0 {
		agent.entryContexts[symbol] = bytes.Clone(decision.Context)
	}

	if envelope != nil {
		decisionRecord := &types.Decision{
			ID:               uuid.NewString(),
			Action:           types.ActionEnter,
			Symbol:           symbol,
			At:               now,
			Direction:        1.0,
			ProposedNotional: notional,
			ProposedQuantity: quantity,
			ReferencePrice:   price,
			Confidence:       decision.Confidence,
			AvailableCapital: agent.cash,
			OpenPositions:    len(agent.positions),
			Cause:            "learned_policy",
			Reason:           string(decision.Action),
		}
		envelope.StrategyRound = &types.StrategyRound{
			Symbol:    symbol,
			Evaluated: true,
			Outcome:   "buy",
			Decisions: []*types.Decision{decisionRecord},
		}
	}

	agent.lastDecision = &telemetry.LearningDecisionT{
		Id:       agent.decisions,
		Agent:    0,
		Symbol:   symbol,
		AtNs:     now.UnixNano(),
		Quantity: quantity.String(),
		Action: &telemetry.LearningActionT{
			Kind:  "buy",
			Power: 1,
			Prior: &telemetry.LearningPriorT{
				Defined:           true,
				Mean:              decision.Confidence,
				Support:           float64(decision.Support),
				Authority:         decision.Confidence,
				EvidenceAuthority: decision.Contrast,
			},
		},
	}
}

func (agent *MainAgent) exitLong(
	envelope *types.Envelope,
	symbol string,
	price *decimal.Decimal,
	now time.Time,
) {
	qty := agent.posQuantities[symbol]
	cost := agent.posCosts[symbol]
	holding := agent.positions[symbol]

	if qty == nil || qty.Sign() <= 0 || cost == nil || cost.Sign() <= 0 {
		return
	}
	proceeds := price.Mul(qty)
	feeRate := agent.feeRate(symbol)
	exitFee := proceeds.Mul(feeRate)
	entryFee := holding.EntryFee

	if entryFee == nil {
		entryFee = decimal.NewFromInt64(0)
	}
	netProfit := proceeds.Sub(cost).Sub(exitFee).Sub(entryFee)

	agent.cash = agent.cash.Add(proceeds).Sub(exitFee)
	agent.fees = agent.fees.Add(exitFee)
	agent.realized = agent.realized.Add(netProfit)
	agent.fills++
	agent.decisions++

	returnBp := netProfit.SetScale(decimal.DefaultScale).Div(cost).Mul(decimal.NewFromInt64(10000)).Float64()

	if netProfit.Sign() > 0 {
		agent.wins++
	}

	if netProfit.Sign() <= 0 {
		agent.losses++
	}

	entryAt := now

	if holding.EntryAt != nil {
		entryAt = *holding.EntryAt
	}

	entryPrice := holding.EntryPrice
	if entryPrice == nil || entryPrice.Sign() <= 0 {
		entryPrice = agent.lastPrice[symbol]
	}

	agent.recordOutcome(TradeOutcome{
		Symbol:     symbol,
		EntryPrice: entryPrice,
		ExitPrice:  price,
		Quantity:   qty,
		Profit:     netProfit,
		ReturnBp:   returnBp,
		EntryAt:    entryAt,
		ExitAt:     now,
	}, returnBp)

	// Forward-testing feedback into the cognitive attractor trie:
	if entryContext, hasContext := agent.entryContexts[symbol]; hasContext && len(entryContext) > 0 && agent.engine != nil {
		fee := agent.feeRate(symbol)
		feedback := -1.0

		if fee != nil {
			roundTripBp := fee.Mul(decimal.NewFromInt64(20000)).Float64()

			if returnBp > roundTripBp {
				feedback = math.Min(1.0, (returnBp-roundTripBp)/roundTripBp)
			}
		}

		agent.engine.Observe(entryContext, []byte(ActionEnter), feedback)
	}

	delete(agent.entryContexts, symbol)
	delete(agent.positions, symbol)
	delete(agent.posQuantities, symbol)
	delete(agent.posCosts, symbol)

	if envelope != nil {
		decision := &types.Decision{
			ID:               uuid.NewString(),
			Action:           types.ActionExit,
			Symbol:           symbol,
			At:               now,
			Direction:        -1.0,
			ProposedNotional: cost,
			ProposedQuantity: qty,
			ReferencePrice:   price,
			AvailableCapital: agent.cash,
			OpenPositions:    len(agent.positions),
			Cause:            "position_exit",
			EntryPrice:       entryPrice,
			ExitPrice:        price,
			PnL:              netProfit,
		}
		envelope.StrategyRound = &types.StrategyRound{
			Symbol:    symbol,
			Evaluated: true,
			Outcome:   "sell",
			Decisions: []*types.Decision{decision},
		}
	}

	agent.lastDecision = &telemetry.LearningDecisionT{
		Id:       agent.decisions,
		Agent:    0,
		Symbol:   symbol,
		AtNs:     now.UnixNano(),
		Quantity: qty.String(),
		Action: &telemetry.LearningActionT{
			Kind:   "sell",
			Power:  1,
			Reduce: true,
			Prior: &telemetry.LearningPriorT{
				Defined: true,
				Mean:    returnBp / 10000.0,
				Support: float64(agent.wins + agent.losses),
			},
		},
	}
}

func (agent *MainAgent) recordOutcome(outcome TradeOutcome, returnBp float64) {
	agent.outcomes = append(agent.outcomes, outcome)
	count := float64(len(agent.outcomes))

	// Welford's algorithm for numerically stable running mean and variance
	delta := returnBp - agent.meanReturn
	agent.meanReturn += delta / count
	delta2 := returnBp - agent.meanReturn
	agent.m2Return += delta * delta2

	if count > 1 {
		agent.variance = agent.m2Return / (count - 1)
	}
}

func (agent *MainAgent) buildCandidates(symbol string, decision ActionDecision) {
	samples := uint64(len(agent.outcomes))
	meanFraction := agent.meanReturn / 10000.0
	varFraction := agent.variance / 100000000.0

	priorUp := &telemetry.LearningPriorT{
		Defined:           decision.Support > 0,
		Mean:              meanFraction,
		Variance:          varFraction,
		VarianceDefined:   samples > 1,
		Samples:           samples,
		Support:           float64(decision.Support),
		Authority:         decision.Confidence,
		EvidenceAuthority: decision.Contrast,
	}

	priorDown := &telemetry.LearningPriorT{
		Defined:           decision.Support > 0,
		Mean:              -meanFraction,
		Variance:          varFraction,
		VarianceDefined:   samples > 1,
		Samples:           samples,
		Support:           float64(decision.Support),
		Authority:         1.0 - decision.Confidence,
		EvidenceAuthority: decision.Contrast,
	}

	priorWait := &telemetry.LearningPriorT{
		Defined:         true,
		Mean:            0,
		Variance:        0,
		VarianceDefined: true,
		Samples:         samples,
		Support:         float64(decision.Support),
		Authority:       0.5,
	}

	agent.alternatives = []*telemetry.LearningActionT{
		{Kind: "enter", Power: 1, Reduce: false, Prior: priorUp},
		{Kind: "exit", Power: 1, Reduce: true, Prior: priorDown},
		{Kind: "wait", Power: 0, Reduce: false, Prior: priorWait},
	}
}

func (agent *MainAgent) updateValuation() {
	positionValue := decimal.NewFromInt64(0)

	for symbol, qty := range agent.posQuantities {
		currentPrice := agent.lastPrice[symbol]

		if qty != nil && currentPrice != nil {
			positionValue = positionValue.Add(currentPrice.Mul(qty))
		}
	}

	agent.equity = agent.cash.Add(positionValue)
	agent.profit = agent.equity.Sub(agent.initial)

	if agent.initial.Sign() > 0 {
		agent.wealth = agent.profit.Div(agent.initial).Float64()
	}
}

/*
evaluateRobustness verifies if the main agent has developed a statistically
defensible edge (sample maturity, net-positive return after fees, positive profit).
When robustly positive, promotes the agent from "simulated" to "trading".
*/
func (agent *MainAgent) evaluateRobustness() {
	samples := agent.wins + agent.losses

	if samples < 10 {
		return
	}

	if agent.realized.Sign() <= 0 || agent.meanReturn <= 0 {
		return
	}
	standardError := math.Sqrt(agent.variance / float64(samples))

	// Positive lower confidence bound
	if (agent.meanReturn - standardError) > 0 {
		if agent.status != "trading" {
			agent.status = "trading"
			errnie.Info("main agent promoted to trading: robustly net-positive edge verified")
		}
	}
}

/*
AgentTelemetry projects the MainAgent state into the FlatBuffers LearningAgentT shape.
*/
func (agent *MainAgent) AgentTelemetry() *telemetry.LearningAgentT {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	positions := make([]*telemetry.PositionT, 0, len(agent.positions))

	for _, holding := range agent.positions {
		entryAt := int64(0)

		if holding.EntryAt != nil {
			entryAt = holding.EntryAt.UnixNano()
		}

		positions = append(positions, &telemetry.PositionT{
			Status: "open",
			Holding: &telemetry.HoldingT{
				Symbol:     holding.Symbol,
				Qty:        holding.Qty.String(),
				EntryPrice: holding.EntryPrice.String(),
				EntryAt:    entryAt,
				EntryFee:   holding.EntryFee.String(),
				Pnl:        holding.PnL.String(),
				ReturnPct:  holding.ReturnPct,
			},
		})
	}

	samples := uint64(len(agent.outcomes))
	meanFraction := agent.meanReturn / 10000.0
	varFraction := agent.variance / 100000000.0

	reading := &telemetry.LearningPriorT{
		Defined:         samples > 0,
		Mean:            meanFraction,
		Variance:        varFraction,
		VarianceDefined: samples > 1,
		Samples:         samples,
		Support:         float64(samples),
		Maturity:        float64(samples) / 100.0,
	}

	status := agent.status

	if status == "simulated" && samples >= 3 && (agent.meanReturn <= 0 || agent.realized.Sign() < 0) {
		status = "learning"
	}

	return &telemetry.LearningAgentT{
		Id:           0,
		Initial:      agent.initial.String(),
		Cash:         agent.cash.String(),
		Equity:       agent.equity.String(),
		Fees:         agent.fees.String(),
		Profit:       agent.profit.String(),
		Wealth:       agent.wealth,
		Realized:     agent.realized.String(),
		Unrealized:   agent.unrealized.String(),
		Decisions:    agent.decisions,
		Fills:        agent.fills,
		Pending:      uint64(len(agent.positions)),
		Wins:         agent.wins,
		Losses:       agent.losses,
		Status:       status,
		Positions:    positions,
		Last:         agent.lastDecision,
		Alternatives: agent.alternatives,
		Reading:      reading,
		Open:         int32(len(agent.positions)),
	}
}
