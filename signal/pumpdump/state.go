package pumpdump

import (
	"errors"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

var (
	errBaselineUnobserved = errors.New("pumpdump: baseline unobserved")
	errLiftUnobserved     = errors.New("pumpdump: lift unobserved")
)

func isWarmup(err error) bool {
	return errors.Is(err, errBaselineUnobserved) || errors.Is(err, errLiftUnobserved)
}

/*
pumpReading is one symbol's folded ignition row before publish.
*/
type pumpReading struct {
	anchor       float64
	grossVolume  float64
	signedVolume float64
	rvol         float64
	precursor    float64
	skew         float64
	lift         float64
	code         float64
	observation  float64
	confidence   float64
	standout     float64
}

/*
pumpState is one symbol's ignition state. Gross and signed windows carry executed
size; EMAs self-scale RVOL and precursor; pipe bands ignition lift.
*/
type pumpState struct {
	gross    *adaptive.Window
	signed   *adaptive.Window
	volBase  *adaptive.EMA
	moveBase *adaptive.EMA
	pipe     *numeric.Classed
	last     float64
}

func newPumpState(classifier *adaptive.Classifier, window time.Duration) *pumpState {
	return &pumpState{
		gross:    adaptive.NewWindow(window),
		signed:   adaptive.NewWindow(window),
		volBase:  adaptive.NewEMA(0),
		moveBase: adaptive.NewEMA(0),
		pipe:     numeric.NewClassed(classifier),
	}
}

func (state *pumpState) ratioScale(value float64, base *adaptive.EMA) (float64, error) {
	magnitude := math.Abs(value)

	if !base.Observed() {
		if _, err := base.Next(0, magnitude); err != nil {
			return 0, err
		}

		return 0, errnie.Error(errBaselineUnobserved)
	}

	norm := base.Value()

	if norm <= 0 {
		if _, err := base.Next(0, magnitude); err != nil {
			return 0, err
		}

		return 0, errnie.Error(errors.New("pumpdump: baseline is zero"))
	}

	scaled := value / norm

	if _, err := base.Next(0, magnitude); err != nil {
		return 0, err
	}

	return scaled, nil
}

func (state *pumpState) fold(trade market.TradeUpdate) (pumpReading, error) {
	nanos := float64(trade.Timestamp.UnixNano())

	if _, err := state.gross.Next(0, nanos, trade.Qty, trade.Price); err != nil {
		return pumpReading{}, errnie.Error(err)
	}

	signedQty := trade.Qty

	if trade.Side != "buy" {
		signedQty = -trade.Qty
	}

	if _, err := state.signed.Next(0, nanos, signedQty, trade.Price); err != nil {
		return pumpReading{}, errnie.Error(err)
	}

	state.last = trade.Price

	anchor := state.gross.Anchor()

	if anchor <= 0 {
		return pumpReading{}, errnie.Error(errors.New("pumpdump: anchor is required"))
	}

	grossVolume := state.gross.Sum()
	signedVolume := state.signed.Sum()

	if grossVolume <= 0 {
		return pumpReading{}, errnie.Error(errors.New("pumpdump: gross volume is required"))
	}

	rvol, err := state.ratioScale(grossVolume, state.volBase)

	if err != nil {
		return pumpReading{}, err
	}

	precursorMove := (state.last - anchor) / anchor
	precursor, err := state.ratioScale(precursorMove, state.moveBase)

	if err != nil {
		return pumpReading{}, err
	}

	skew := signedVolume / grossVolume
	lift := (rvol - 1) * (1 + precursor) * (1 + skew)

	code, err := state.pipe.Push(lift)

	if err != nil {
		return pumpReading{}, errnie.Error(err)
	}

	return pumpReading{
		anchor:       anchor,
		grossVolume:  grossVolume,
		signedVolume: signedVolume,
		rvol:         rvol,
		precursor:    precursor,
		skew:         skew,
		lift:         lift,
		code:         code,
		observation:  state.pipe.Observation(),
		confidence:   state.pipe.Confidence(),
		standout:     state.pipe.Standout(),
	}, nil
}
