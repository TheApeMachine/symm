package strategy

import (
	"context"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/learning"
)

/*
Rehearsal presents retained numerical inputs and subsequent captured books to
LocalLearning. It uses the live wallets, action selector, fills and reward
accounting. It has no execution desk. Retrospective marks never enter Input or
Step; they select tape and evaluate its results outside this owner.
*/
type Rehearsal struct {
	Agent  *Agent
	inputs map[string]uint64
	at     time.Time
}

/* NewRehearsal uses supplied venue economics and a frozen, previously measured horizon. */
func NewRehearsal(ctx context.Context, books LearningBook, pair func(string) kraken.InstrumentPair,
	fee func(string) *kraken.TradeVolumeFee, initial *decimal.Decimal, horizons map[string]time.Duration,
	record func(hindsight.LearningEvent) error,
) (*Rehearsal, error) {
	agent, err := NewAgent(ctx, learning.NewGrid(), books, pair, fee, initial, record)
	if err != nil {
		return nil, err
	}
	for symbol, horizon := range horizons {
		if horizon <= 0 {
			return nil, errnie.Err(errnie.Validation, "rehearsal: positive measured horizon required for "+symbol, nil)
		}
		agent.Horizons[symbol] = horizon
	}
	return &Rehearsal{Agent: agent, inputs: make(map[string]uint64)}, nil
}

/* Input admits only facts available before the next book is presented. */
func (rehearsal *Rehearsal) Input(input hindsight.RehearsalInput) error {
	if input.At.IsZero() || input.At.Before(rehearsal.at) || input.Symbol == "" || input.GridVersion == 0 ||
		len(input.Context) != len(input.Quantities) || input.Authority < 0 || input.Authority > 1 {
		return errnie.Err(errnie.Validation, "rehearsal: ordered identified numerical input required", nil)
	}
	local := rehearsal.Agent.LocalLearning
	market := local.markets[input.Symbol]
	if market == nil {
		market = &learningMarket{symbol: input.Symbol, opportunityHorizon: local.Horizons[input.Symbol]}
		local.markets[input.Symbol] = market
	}
	market.sequence = market.sequence[:0]
	market.conditions = market.conditions[:0]
	for index, quantity := range input.Quantities {
		if quantity[0] == "" || quantity[1] == "" {
			return errnie.Err(errnie.Validation, "rehearsal: quantity identity missing", nil)
		}
		token := uint64(local.Grid.Column(quantity[0], quantity[1]) + 1)
		market.sequence = append(market.sequence, token)
		market.conditions = append(market.conditions, learning.RemapCondition(input.Context[index], token))
	}
	market.gridVersion, market.authority = input.GridVersion, input.Authority
	market.capture = input.Capture
	rehearsal.at = input.At
	return nil
}

/* Step observes one later executable book; no sleeps substitute for captured time. */
func (rehearsal *Rehearsal) Step(symbol string, at time.Time, capture hindsight.CaptureIdentity) error {
	if at.IsZero() || at.Before(rehearsal.at) {
		return errnie.Err(errnie.Validation, "rehearsal: book time moved backwards", nil)
	}
	rehearsal.at = at
	local := rehearsal.Agent.LocalLearning
	market := local.markets[symbol]
	if market == nil {
		return nil
	}
	market.at, market.seq = at, capture.Sequence
	local.now = func() time.Time { return at }
	market.events = market.events[:0]
	var err error
	local.books.Book(symbol, func(book *spotbook.Book) {
		if book == nil || book.Bids.High == nil || book.Asks.Low == nil || book.Bids.High.Price.Cmp(book.Asks.Low.Price) >= 0 {
			return
		}
		if len(market.lanes) == 0 {
			err = local.initialize(market)
		}
		if err != nil {
			return
		}
		changed := rehearsal.inputs[symbol] != market.gridVersion
		rehearsal.inputs[symbol] = market.gridVersion
		err = local.transition(market, book, at, changed)
	})
	if err != nil {
		return err
	}
	for _, event := range market.events {
		event.HorizonSource = "hindsight_completed_excursion_mean"
		if err := local.Record(event); err != nil {
			return err
		}
	}
	return local.flush()
}
