package pumpdump

import (
	"container/ring"

	"fmt"

	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
)

var pumpDumpCategories = []logic.CategoryType{
	logic.CategoryVerticalIgnition,
	logic.CategoryCoiledCompression,
	logic.CategoryOrganicTrend,
	logic.CategoryFadedExhaustion,
}

/*
Signal identifies pre-pump microstructure by looking for sudden "verticality" in volume and price.

Volume Lift (RVOL) : Measures fast and medium-term volume spikes against a median hourly baseline.
Precursor Move     : Uses a $PositiveMove$ dynamic to score how much the price has already begun to detach from its recent anchor.
Spread Compression : Scores how much the bid/ask spread has tightened versus its own baseline.
Move Classifier    : A state-free primitive that maps these metrics into an explicit "Pump" or "Dump" class.

The "Ignition" Story      : It identifies the exact moment a move stops being random walk and becomes a vertical event driven by abnormal volume "lift".
The "Coiled Spring" Story : By tracking spread compression and book-side strength, it identifies when a market is "tightly wound" and ready to snap.

| Category           | Volume Lift | Price Precursor | Market "Feel"        |
|--------------------|-------------|-----------------|----------------------|
| Vertical Ignition  | High Spike  | High            | Launching / Breakout |
| Coiled Compression | Moderate    | Low             | Pre-Pump / Loaded    |
| Organic Trend      | Low/Steady  | Moderate        | Healthy Momentum     |
| Faded Exhaustion   | Falling     | Flat            | Leg is Dead          |
*/
type Signal struct {
	symbol          string
	entity          *logic.Entity
	measurements    *ring.Ring
	warmupRemaining int
	transition      *numeric.TransitionMatrix
	rvol            *numeric.FastSlowRatio
	compression     *numeric.FastSlowRatio
	weights         numeric.ClassifierWeights
	tuner           *numeric.FeedbackTuner
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
) *Signal {
	capacity := viper.GetInt("signals.pumpdump.measurements_capacity")

	return &Signal{
		symbol:          symbol,
		entity:          entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		transition:      numeric.NewTransitionMatrix(5, viper.GetFloat64("signals.pumpdump.surprise.matrix.alpha")),
		rvol:            numeric.NewFastSlowRatio(viper.GetInt("signals.pumpdump.fast_window"), viper.GetFloat64("signals.pumpdump.volume.epsilon")),
		compression:     numeric.NewInvertedFastSlowRatio(viper.GetInt("signals.pumpdump.fast_window"), viper.GetFloat64("signals.pumpdump.volume.epsilon")),
		weights:         numeric.DefaultClassifierWeights(viper.GetFloat64("signals.pumpdump.surprise.weights.threshold")),
		tuner:           numeric.NewFeedbackTuner(),
	}
}

func (signal *Signal) Symbol() string {
	return signal.symbol
}

func (signal *Signal) Measure(feedback *market.Feedback, at time.Time) (logic.Measurement, error) {
	if feedback != nil {
		_, err := signal.tuner.Apply(
			signal.symbol,
			feedback.Symbol,
			feedback.Samples,
			feedback.MSE,
			feedback.Scale,
			feedback.Bias,
			&signal.weights,
		)

		if err != nil {
			return logic.Measurement{}, errnie.Error(err)
		}
	}

	switch signal.entity.Type {
	case logic.EntityTrade:
		return signal.measureTrade(at)
	case logic.EntityTick:
		return signal.measureTick(at)
	case logic.EntityBook:
		return signal.measureBook(at)
	default:
		return logic.Measurement{}, errnie.Error(
			fmt.Errorf("pumpdump: unsupported entity %d", signal.entity.Type),
		)
	}
}

func (signal *Signal) measureTrade(at time.Time) (logic.Measurement, error) {
	var (
		prices  []float64
		volumes []float64
		err     error
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		trade, ok := item.(*krakenmarket.TradeUpdate)

		if !ok {
			err = fmt.Errorf("pumpdump: expected trade update")
			return
		}

		if trade.Price <= 0 || trade.Qty <= 0 {
			return
		}

		prices = append(prices, trade.Price)
		volumes = append(volumes, trade.Qty)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	return signal.fromSeries(prices, volumes, nil, at)
}

func (signal *Signal) measureTick(at time.Time) (logic.Measurement, error) {
	var (
		prices  []float64
		volumes []float64
		spreads []float64
		err     error
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		tick, ok := item.(*krakenmarket.TickerUpdate)

		if !ok {
			err = fmt.Errorf("pumpdump: expected ticker update")
			return
		}

		if tick.Bid <= 0 || tick.Ask <= tick.Bid {
			return
		}

		prices = append(prices, tick.Ask+tick.Bid)
		volumes = append(volumes, tick.AskQty+tick.BidQty)
		spreads = append(spreads, tick.Ask-tick.Bid)
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	return signal.fromSeries(prices, volumes, spreads, at)
}

func (signal *Signal) measureBook(at time.Time) (logic.Measurement, error) {
	var (
		prices  []float64
		volumes []float64
		spreads []float64
		err     error
	)

	signal.measurements.Do(func(item any) {
		if item == nil {
			return
		}

		frame, ok := item.(*krakenmarket.BookUpdate)

		if !ok {
			err = fmt.Errorf("pumpdump: expected book update")
			return
		}

		if len(frame.Bids) == 0 || len(frame.Asks) == 0 {
			return
		}

		touchSpread := frame.Asks[0].Price - frame.Bids[0].Price

		if touchSpread <= 0 {
			return
		}

		spreads = append(spreads, touchSpread)

		for _, bid := range frame.Bids {
			if bid.Qty > 0 {
				prices = append(prices, bid.Price)
				volumes = append(volumes, bid.Qty)
			}
		}

		for _, ask := range frame.Asks {
			if ask.Qty > 0 {
				prices = append(prices, ask.Price)
				volumes = append(volumes, ask.Qty)
			}
		}
	})

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	return signal.fromSeries(prices, volumes, spreads, at)
}

func (signal *Signal) fromSeries(
	prices []float64,
	volumes []float64,
	spreads []float64,
	at time.Time,
) (logic.Measurement, error) {
	if len(prices) == 0 {
		return logic.Measurement{Symbol: signal.symbol, ObservedAt: at}, nil
	}

	price := prices[len(prices)-1]
	volume := numeric.Sum(volumes)
	spread := 0.0

	if len(spreads) > 0 {
		spread = spreads[len(spreads)-1]
	}

	rvol, err := signal.rvol.Next(0, volumes...)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	move, precursor := numeric.AnchorChange(prices[0], price)
	compression := 0.0

	if len(spreads) > 0 {
		compression, err = signal.compression.Next(0, spreads...)

		if err != nil {
			return logic.Measurement{}, errnie.Error(err)
		}
	}

	probabilities, err := numeric.SoftmaxScores(signal.weights.Scores(rvol, precursor, compression))

	if err != nil {
		return logic.Measurement{Symbol: signal.symbol, ObservedAt: at}, err
	}

	category := pumpDumpCategories[numeric.ArgmaxIndex(probabilities)]

	categoryIndex := 0

	switch category {
	case logic.CategoryVerticalIgnition:
		categoryIndex = 1
	case logic.CategoryCoiledCompression:
		categoryIndex = 2
	case logic.CategoryOrganicTrend:
		categoryIndex = 3
	case logic.CategoryFadedExhaustion:
		categoryIndex = 4
	}

	surpriseVector := signal.transition.PadObserved(probabilities, 1e-6)
	surprise, err := signal.transition.Surprise(surpriseVector)

	if err != nil {
		return logic.Measurement{Symbol: signal.symbol, ObservedAt: at}, err
	}

	signal.transition.Update(categoryIndex)

	confidence := 0.0

	if categoryIndex > 0 && categoryIndex-1 < len(probabilities) {
		confidence = probabilities[categoryIndex-1]
	}

	position := logic.PositionTypeNone

	if move > 0 {
		position = logic.PositionTypeLong
	}

	if move < 0 {
		position = logic.PositionTypeShort
	}

	return logic.Measurement{
		Source:     logic.SourcePumpDump,
		Symbol:     signal.symbol,
		Price:      price,
		Strength:   signal.weights.Strength(rvol, precursor),
		Volume:     volume,
		Spread:     spread,
		Elapsed:    0,
		Category:   category,
		Regime:     logic.RegimeTypeNone,
		Position:   position,
		Confidence: confidence,
		Surprise:   surprise,
		ObservedAt: at,
	}, nil
}

func (signal *Signal) Record(raw any) bool {
	warmed := false

	if signal.warmupRemaining > 0 {
		signal.warmupRemaining--
		warmed = true
	}

	signal.measurements.Value = raw
	signal.measurements = signal.measurements.Next()

	return warmed
}

func (signal *Signal) WarmupFilled() int {
	return signal.measurements.Len() - signal.warmupRemaining
}
