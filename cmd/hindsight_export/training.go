package main

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/* trainingObservationRecord is one exact retained Advisor observation. */
type trainingObservationRecord struct {
	Kind       string             `json:"kind"`
	Run        string             `json:"run"`
	Sequence   uint64             `json:"sequence"`
	Ordinal    uint64             `json:"ordinal"`
	Tick       int64              `json:"tick"`
	Symbol     string             `json:"symbol"`
	ObservedAt int64              `json:"observedAt"`
	Clock      string             `json:"clock"`
	Coordinate uint64             `json:"coordinate"`
	Metrics    map[string]float64 `json:"metrics"`
	Bid        float64            `json:"bid,omitempty"`
	Ask        float64            `json:"ask,omitempty"`
	HasQuote   bool               `json:"hasQuote"`
}

type trainingQuote struct {
	bid float64
	ask float64
}

/* trainingObservationStream reconstructs Solver's retained metric map. */
type trainingObservationStream struct {
	clock       string
	metrics     map[string]map[string]float64
	coordinates map[string]uint64
	quotes      map[string]trainingQuote
}

func newTrainingObservationStream(clock string) *trainingObservationStream {
	return &trainingObservationStream{
		clock:       clock,
		metrics:     make(map[string]map[string]float64),
		coordinates: make(map[string]uint64),
		quotes:      make(map[string]trainingQuote),
	}
}

/* Observe emits the retained state exactly when the declared clock advances. */
func (stream *trainingObservationStream) Observe(
	runID string,
	sequence uint64,
	ordinal uint64,
	state *telemetry.EnvelopeState,
	current map[string]float64,
) (trainingObservationRecord, bool, error) {
	symbol, observedAt := stream.retain(state, current)

	if symbol == "" {
		return trainingObservationRecord{}, false, nil
	}

	clock, clockFound := current[stream.clock]

	if !clockFound {
		return trainingObservationRecord{}, false, nil
	}

	if clock < 0 || math.Trunc(clock) != clock {
		return trainingObservationRecord{}, false, fmt.Errorf(
			"advisor market clock must be a non-negative ordinal",
		)
	}

	coordinate := uint64(clock)
	previous, hasPrevious := stream.coordinates[symbol]

	if hasPrevious && coordinate < previous {
		return trainingObservationRecord{}, false, fmt.Errorf(
			"advisor market clock moved backwards for %s",
			symbol,
		)
	}

	stream.coordinates[symbol] = coordinate

	if coordinate == 0 || hasPrevious && coordinate == previous {
		return trainingObservationRecord{}, false, nil
	}

	record := trainingObservationRecord{
		Kind:       "observation",
		Run:        runID,
		Sequence:   sequence,
		Ordinal:    ordinal,
		Tick:       state.Tick(),
		Symbol:     symbol,
		ObservedAt: observedAt,
		Clock:      stream.clock,
		Coordinate: coordinate,
		Metrics:    stream.metrics[symbol],
	}

	if quote, found := stream.quotes[symbol]; found {
		record.Bid = quote.bid
		record.Ask = quote.ask
		record.HasQuote = true
	}

	return record, true, nil
}

func (stream *trainingObservationStream) retain(
	state *telemetry.EnvelopeState,
	current map[string]float64,
) (string, int64) {
	symbol, observedAt := stateMarketIdentity(state)

	if symbol == "" {
		return "", 0
	}

	if state.TypeId() == byte(types.EnvelopeTicker) {
		ticker := state.TickerData(nil)

		if ticker != nil && ticker.HasBid() && ticker.HasAsk() &&
			ticker.Bid() > 0 && ticker.Ask() >= ticker.Bid() {
			stream.quotes[symbol] = trainingQuote{
				bid: ticker.Bid(),
				ask: ticker.Ask(),
			}
		}
	}

	retained, found := stream.metrics[symbol]

	if !found {
		retained = make(map[string]float64)
		stream.metrics[symbol] = retained
	}

	for key, value := range current {
		retained[key] = value
	}

	return symbol, observedAt
}

func stateMarketIdentity(state *telemetry.EnvelopeState) (string, int64) {
	if state == nil {
		return "", 0
	}

	switch types.TypeID(state.TypeId()) {
	case types.EnvelopeTicker:
		market := state.TickerData(nil)

		if market != nil {
			return string(market.Symbol()), market.TimestampNs()
		}
	case types.EnvelopeTrade:
		market := state.TradeData(nil)

		if market != nil {
			return string(market.Symbol()), market.TimestampNs()
		}
	case types.EnvelopeLevel3:
		market := state.Level3Data(nil)

		if market != nil {
			return string(market.Symbol()), market.TimestampNs()
		}
	case types.EnvelopeFuturesTicker:
		market := state.FuturesTickerData(nil)

		if market != nil {
			return spotSymbol(string(market.Symbol())), market.TimestampNs()
		}
	case types.EnvelopeFuturesTrade:
		market := state.FuturesTradeData(nil)

		if market != nil {
			return spotSymbol(string(market.Symbol())), market.TimestampNs()
		}
	}

	return "", 0
}

func spotSymbol(product string) string {
	symbol, found := kraken.SpotSymbol(product)

	if found {
		return symbol
	}

	return product
}
