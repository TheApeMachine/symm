package pumpdump

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

/*
PumpDump is the Ignition perspective, identifying pre-pump microstructure by
looking for sudden "verticality" in volume and price.

1. What it measures exactly (in isolation)

Volume Lift (RVOL): Measures positive volume delta spikes against a
median-scaled baseline whose depth is derived from the pair's tick cadence.

Precursor Move: Scores upward price detachment from its recent anchor
(positive-only log return, scaled by its own recent median).

Spread Compression: Scores how much the bid/ask spread has tightened versus
its own median-scaled baseline.

Ignition Classifier: Maps rvol, precursor, compression, and rvol-decline into
four ignition states (not a symmetric pump/dump direction classifier).

---

2. Semantically, what story does it tell?

The PumpDump signal tells the story of explosive ignition and coiled energy.

The "Ignition" Story: It identifies the exact moment a move stops being random
walk and becomes a vertical event driven by abnormal volume "lift".

The "Coiled Spring" Story: By tracking spread compression with moderate volume
lift and low precursor, it identifies when a market is "tightly wound" and
ready to snap.

1. Vertical Ignition

Volume and price are detaching together in a vertical event.
Indicators: High volume lift spike with high price precursor.
Semantic Meaning: Launching/Breakout — the move has ignited.

2. Coiled Compression

Energy is building before the vertical move.
Indicators: Moderate volume lift with low price precursor.
Semantic Meaning: Pre-Pump/Loaded — tightly wound and ready to snap.

3. Organic Trend

Steady momentum without abnormal verticality.
Indicators: Low/steady volume lift with moderate price precursor.
Semantic Meaning: Healthy momentum — supported but not explosive.

4. Faded Exhaustion

The vertical leg has lost its lift.
Indicators: Falling volume lift with flat price precursor.
Semantic Meaning: Leg is dead — the ignition has faded.

# Summary of PumpDump Categories

| Category           | Volume Lift | Price Precursor | Market "Feel"            |
|:-------------------|:------------|:----------------|:-------------------------|
| Vertical Ignition  | High Spike  | High            | Launching / Breakout     |
| Coiled Compression | Moderate    | Low             | Pre-Pump / Loaded        |
| Organic Trend      | Low/Steady  | Moderate        | Healthy Momentum         |
| Faded Exhaustion   | Falling     | Flat            | Leg is Dead              |
*/

/*
Signal composes the role objects that own raw ticker, book, and trade artifact
history. Signal routes market artifacts and supplies role context to Ticker.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	ticker *Ticker
	book   *Book
	trade  *Trade
}

/*
NewSignal constructs the verticality signal.
*/
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	book := NewBook()
	trade := NewTrade()

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(),
		book:   book,
		trade:  trade,
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker", "book", "trade"}
}

/*
Measure scores ticker rows against tree-backed measurements and enriched book,
trade, and cross-section context.
*/
func (signal *Signal) Measure(
	input market.Input,
	crossSection *market.CrossSection,
) ([]*logic.Measurement, error) {
	if input.Role == "ticker" {
		return signal.measureTickers(input)
	}

	if input.Role == "book" {
		return signal.measureBooks(input)
	}

	if input.Role == "trade" {
		return signal.measureTrades(input)
	}

	return nil, nil
}

func (signal *Signal) measureTickers(
	input market.Input,
) ([]*logic.Measurement, error) {
	measurements := make([]*logic.Measurement, 0, len(input.Ticker))

	for _, row := range input.Ticker {
		if row.Last <= 0 || math.IsNaN(row.Last) || math.IsInf(row.Last, 0) {
			continue
		}

		measurement, err := signal.ticker.Measure(row)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent, err.Error(), err,
			))
		}

		if measurement == nil {
			continue
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
}

func (signal *Signal) measureBooks(
	input market.Input,
) ([]*logic.Measurement, error) {
	measurements := make([]*logic.Measurement, 0, len(input.Book))

	for _, row := range input.Book {
		measurement, err := signal.book.Measure(row)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent, err.Error(), err,
			))
		}

		if measurement == nil {
			continue
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
}

func (signal *Signal) measureTrades(
	input market.Input,
) ([]*logic.Measurement, error) {
	measurements := make([]*logic.Measurement, 0, len(input.Trade))

	for _, row := range input.Trade {
		measurement, err := signal.trade.Measure(row)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent, err.Error(), err,
			))
		}

		if measurement == nil {
			continue
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}
