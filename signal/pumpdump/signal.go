package pumpdump

import (
	"container/ring"

	"fmt"

	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/statistic"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
	signalsupport "github.com/theapemachine/symm/signal"
)

var pumpDumpCategories = []logic.CategoryType{
	logic.CategoryVerticalIgnition,
	logic.CategoryCoiledCompression,
	logic.CategoryOrganicTrend,
	logic.CategoryFadedExhaustion,
}

/*
Signal identifies pre-pump microstructure by looking for sudden "verticality" in volume and price.

Volume Lift (RVOL) : Current volume against a time-decayed baseline (halflife from config window).
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
	transition      *probability.TransitionMatrix
	rvolTracker     *adaptive.TimeElasticMemory
	compTracker     *adaptive.TimeElasticMemory
	lastRvol        float64
	lastCompression float64
	observeErr      error
	weights         learning.ClassifierWeights
	tuner           *learning.FeedbackTuner
}

func NewSignal(
	symbol string,
	entity *logic.Entity,
) *Signal {
	capacity := viper.GetInt("signals.pumpdump.measurements_capacity")
	halflife := viper.GetDuration("signals.pumpdump.window")

	if halflife <= 0 {
		halflife = time.Minute
	}

	epsilon := viper.GetFloat64("signals.pumpdump.volume.epsilon")

	return &Signal{
		symbol:          symbol,
		entity:          entity,
		measurements:    ring.New(capacity),
		warmupRemaining: capacity,
		transition:      probability.NewTransitionMatrix(5, viper.GetFloat64("signals.pumpdump.surprise.matrix.alpha")),
		rvolTracker:     adaptive.NewTimeElasticMemory(halflife, epsilon),
		compTracker:     adaptive.NewTimeElasticMemory(halflife, epsilon),
		weights:         learning.DefaultClassifierWeights(viper.GetFloat64("signals.pumpdump.surprise.weights.threshold")),
		tuner:           learning.NewFeedbackTuner(),
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
		return logic.Measurement{}, fmt.Errorf(
			"pumpdump: unsupported entity type %q for signal %q",
			signal.entity.Type,
			signal.symbol,
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
	if len(prices) < 2 {
		return logic.Measurement{}, nil
	}

	price := prices[len(prices)-1]
	volume := float64(statistic.NewSum().Observe(nomagique.Numbers(volumes...)...))
	spread := 0.0

	if len(spreads) > 0 {
		spread = spreads[len(spreads)-1]
	}

	var err error

	if spread <= 0 {
		spread, err = signalsupport.TouchSpread(prices)

		if err != nil {
			return logic.Measurement{}, nil
		}
	}

	elapsed, err := signalsupport.ObservationElapsed(
		signal.measurements, at,
	)

	if err != nil {
		return logic.Measurement{}, nil
	}

	_, _, ok := signalsupport.ResolvedChange(prices)

	if !ok {
		return logic.Measurement{}, nil
	}

	row, err := krakenmarket.SymbolRowFromPrices(
		signal.symbol, prices, volume, 1, at,
	)

	if err != nil {
		return logic.Measurement{}, nil
	}

	if errnie.Error(signal.observeErr) != nil {
		return logic.Measurement{}, errnie.Error(signal.observeErr)
	}

	if !signal.rvolTracker.Initialized() {
		return logic.Measurement{}, nil
	}

	rvol := signal.lastRvol
	compression := signal.lastCompression

	move, precursor := numeric.AnchorChange(prices[0], price)

	probabilities, err := probability.SoftmaxScores(
		signal.weights.Scores(rvol, precursor, compression),
	)

	if err != nil {
		return logic.Measurement{}, err
	}

	category := pumpDumpCategories[probability.ArgmaxIndex(
		probabilities,
	)]

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

	surpriseVector := signal.transition.PadObserved(
		probabilities, 1e-6,
	)

	surprise, err := signal.transition.Surprise(
		surpriseVector,
	)

	if err != nil {
		return logic.Measurement{}, nil
	}

	signal.transition.Update(categoryIndex)

	confidence, err := probability.CategoryConfidence(
		probabilities, categoryIndex,
	)

	if err != nil {
		return logic.Measurement{}, nil
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
		Elapsed:    elapsed,
		Category:   category,
		Regime:     logic.RegimeTypeNone,
		Position:   position,
		Confidence: confidence,
		Surprise:   surprise,
		ObservedAt: at,
		Market:     *row,
	}, nil
}

func (signal *Signal) Record(raw any) bool {
	warmed := false

	if signal.warmupRemaining > 0 {
		signal.warmupRemaining--
		warmed = true
	}

	if observeErr := signal.trackObservation(raw); observeErr != nil {
		signal.observeErr = errnie.Error(observeErr)
	}

	signal.measurements.Value = raw
	signal.measurements = signal.measurements.Next()

	return warmed
}

func (signal *Signal) trackObservation(raw any) error {
	at, volume, spread, ok := observationFromEvent(raw)

	if !ok {
		return nil
	}

	if at.IsZero() {
		return fmt.Errorf("pumpdump: event timestamp is required")
	}

	if volume > 0 {
		relative, err := signal.rvolTracker.Update(at, volume)

		if err != nil {
			return err
		}

		signal.lastRvol = relative
	}

	if spread > 0 {
		spreadRatio, err := signal.compTracker.Update(at, spread)

		if err != nil {
			return err
		}

		if spreadRatio > 0 {
			signal.lastCompression = 1.0 / spreadRatio
		}

		return nil
	}

	signal.lastCompression = 0

	return nil
}

func observationFromEvent(raw any) (at time.Time, volume float64, spread float64, ok bool) {
	switch event := raw.(type) {
	case *krakenmarket.TradeUpdate:
		if event == nil || event.Price <= 0 || event.Qty <= 0 {
			return time.Time{}, 0, 0, false
		}

		return event.Timestamp, event.Qty, 0, true
	case *krakenmarket.TickerUpdate:
		if event == nil || event.Bid <= 0 || event.Ask <= event.Bid {
			return time.Time{}, 0, 0, false
		}

		return event.Timestamp, event.AskQty + event.BidQty, event.Ask - event.Bid, true
	case *krakenmarket.BookUpdate:
		if event == nil || len(event.Bids) == 0 || len(event.Asks) == 0 {
			return time.Time{}, 0, 0, false
		}

		touchSpread := event.Asks[0].Price - event.Bids[0].Price

		if touchSpread <= 0 {
			return time.Time{}, 0, 0, false
		}

		touchVolume := event.Bids[0].Qty + event.Asks[0].Qty

		return event.Timestamp, touchVolume, touchSpread, true
	default:
		return time.Time{}, 0, 0, false
	}
}

func (signal *Signal) WarmupFilled() int {
	return signal.measurements.Len() - signal.warmupRemaining
}
