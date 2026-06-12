package broker

import (
	"errors"
	"fmt"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

const basisPointsPerUnit = 10000.0

type PreTradeRiskGate interface {
	Validate(*logic.Action, QuoteSnapshot, time.Time) error
}

/*
QuoteSnapshot captures the ticker state observed by the desk.
*/
type QuoteSnapshot struct {
	Symbol     string
	Bid        float64
	Ask        float64
	Last       float64
	ObservedAt time.Time
}

/*
TickerPreTradeRiskGate enforces the final order-boundary ticker checks.
*/
type TickerPreTradeRiskGate struct {
	maxQuoteAge    time.Duration
	maxSpreadBps   float64
	maxSlippageBps float64
}

func NewPreTradeRiskGate(
	tradingConfig config.TradingConfig,
) (*TickerPreTradeRiskGate, error) {
	if tradingConfig.MaxQuoteAge <= 0 {
		return nil, errors.New("broker risk: max quote age must be positive")
	}

	if tradingConfig.MaxSpreadBps <= 0 {
		return nil, errors.New("broker risk: max spread bps must be positive")
	}

	if tradingConfig.MaxSlippageBps <= 0 {
		return nil, errors.New("broker risk: max slippage bps must be positive")
	}

	return &TickerPreTradeRiskGate{
		maxQuoteAge:    tradingConfig.MaxQuoteAge,
		maxSpreadBps:   tradingConfig.MaxSpreadBps,
		maxSlippageBps: tradingConfig.MaxSlippageBps,
	}, nil
}

func NewQuoteSnapshot(ticker *market.TickerUpdate, observedAt time.Time) QuoteSnapshot {
	if ticker == nil {
		return QuoteSnapshot{}
	}

	return QuoteSnapshot{
		Symbol:     ticker.Symbol,
		Bid:        ticker.Bid,
		Ask:        ticker.Ask,
		Last:       ticker.Last,
		ObservedAt: observedAt,
	}
}

func (riskGate *TickerPreTradeRiskGate) Validate(
	action *logic.Action,
	quote QuoteSnapshot,
	now time.Time,
) error {
	if riskGate == nil {
		return errors.New("broker risk: pre-trade risk gate is not configured")
	}

	if action == nil {
		return errors.New("broker risk: action is required")
	}

	if action.Type.IsExit() {
		return nil
	}

	if action.Symbol == "" {
		return errors.New("broker risk: action symbol is required")
	}

	if quote.Symbol == "" {
		return fmt.Errorf("broker risk: quote for %q is required", action.Symbol)
	}

	if quote.Symbol != action.Symbol {
		return fmt.Errorf(
			"broker risk: quote symbol %q does not match action symbol %q",
			quote.Symbol,
			action.Symbol,
		)
	}

	if quote.ObservedAt.IsZero() {
		return fmt.Errorf("broker risk: quote for %q has no observation time", action.Symbol)
	}

	quoteAge := now.Sub(quote.ObservedAt)

	if quoteAge < 0 {
		return fmt.Errorf("broker risk: quote for %q is from the future", action.Symbol)
	}

	if quoteAge > riskGate.maxQuoteAge {
		return fmt.Errorf(
			"broker risk: quote for %q is stale: %s > %s",
			action.Symbol,
			quoteAge,
			riskGate.maxQuoteAge,
		)
	}

	spreadBps, spreadErr := quote.SpreadBps()

	if spreadErr != nil {
		return spreadErr
	}

	if spreadBps > riskGate.maxSpreadBps {
		return fmt.Errorf(
			"broker risk: spread for %q is %.4f bps > %.4f bps",
			action.Symbol,
			spreadBps,
			riskGate.maxSpreadBps,
		)
	}

	slippageBps, slippageErr := quote.ProjectedSlippageBps(action)

	if slippageErr != nil {
		return slippageErr
	}

	if slippageBps > riskGate.maxSlippageBps {
		return fmt.Errorf(
			"broker risk: projected slippage for %q is %.4f bps > %.4f bps",
			action.Symbol,
			slippageBps,
			riskGate.maxSlippageBps,
		)
	}

	return nil
}

func (quote QuoteSnapshot) SpreadBps() (float64, error) {
	midpoint, midErr := quote.midpoint()

	if midErr != nil {
		return 0, midErr
	}

	if quote.Ask < quote.Bid {
		return 0, fmt.Errorf("broker risk: crossed quote for %q", quote.Symbol)
	}

	return ((quote.Ask - quote.Bid) / midpoint) * basisPointsPerUnit, nil
}

func (quote QuoteSnapshot) ProjectedSlippageBps(
	action *logic.Action,
) (float64, error) {
	if action == nil {
		return 0, errors.New("broker risk: action is required")
	}

	referencePrice, referenceErr := quote.referencePrice(action)

	if referenceErr != nil {
		return 0, referenceErr
	}

	touchPrice, touchErr := quote.touchPrice(action.Side)

	if touchErr != nil {
		return 0, touchErr
	}

	slippage := directionalSlippage(action.Side, referencePrice, touchPrice)

	if slippage <= 0 {
		return 0, nil
	}

	return (slippage / referencePrice) * basisPointsPerUnit, nil
}

func (quote QuoteSnapshot) midpoint() (float64, error) {
	if quote.Bid <= 0 || quote.Ask <= 0 {
		return 0, fmt.Errorf("broker risk: bid and ask are required for %q", quote.Symbol)
	}

	midpoint := (quote.Bid + quote.Ask) / 2

	if midpoint <= 0 {
		return 0, fmt.Errorf("broker risk: midpoint is invalid for %q", quote.Symbol)
	}

	return midpoint, nil
}

func (quote QuoteSnapshot) referencePrice(action *logic.Action) (float64, error) {
	if action.Price > 0 {
		return action.Price, nil
	}

	if quote.Last > 0 {
		return quote.Last, nil
	}

	return quote.midpoint()
}

func (quote QuoteSnapshot) touchPrice(side trading.Side) (float64, error) {
	switch side {
	case trading.Buy:
		if quote.Ask <= 0 {
			return 0, fmt.Errorf("broker risk: ask is required for %q", quote.Symbol)
		}

		return quote.Ask, nil
	case trading.Sell:
		if quote.Bid <= 0 {
			return 0, fmt.Errorf("broker risk: bid is required for %q", quote.Symbol)
		}

		return quote.Bid, nil
	default:
		return 0, fmt.Errorf("broker risk: unsupported side %q", side)
	}
}

func directionalSlippage(
	side trading.Side,
	referencePrice float64,
	touchPrice float64,
) float64 {
	switch side {
	case trading.Buy:
		return touchPrice - referencePrice
	case trading.Sell:
		return referencePrice - touchPrice
	default:
		return 0
	}
}

func (desk *Desk) storeQuote(ticker *market.TickerUpdate) {
	if desk == nil || ticker == nil || ticker.Symbol == "" {
		return
	}

	desk.recordTickerAge(ticker)
	desk.quotes.Store(ticker.Symbol, NewQuoteSnapshot(ticker, time.Now().UTC()))
}

func (desk *Desk) validatePreTrade(action *logic.Action) error {
	if desk == nil {
		return errors.New("broker risk: desk is required")
	}

	if desk.isRiskReducingExit(action) {
		return nil
	}

	if desk.riskGate == nil {
		return errors.New("broker risk: pre-trade risk gate is not configured")
	}

	if action == nil {
		return errors.New("broker risk: action is required")
	}

	rawQuote, ok := desk.quotes.Load(action.Symbol)

	if !ok {
		return fmt.Errorf("broker risk: quote for %q is required", action.Symbol)
	}

	quote, quoteOK := rawQuote.(QuoteSnapshot)

	if !quoteOK {
		return fmt.Errorf("broker risk: quote for %q is invalid", action.Symbol)
	}

	return desk.riskGate.Validate(action, quote, time.Now().UTC())
}

func (desk *Desk) isRiskReducingExit(action *logic.Action) bool {
	if action == nil {
		return false
	}

	if action.Type.IsExit() {
		return true
	}

	if action.Side != trading.Sell {
		return false
	}

	if _, ok := desk.stops.Load(action.Symbol); ok {
		return true
	}

	frame := desk.positions.Snapshot()

	for _, position := range frame.Positions {
		if position.Symbol == action.Symbol && position.Quantity > 0 {
			return true
		}
	}

	return false
}
