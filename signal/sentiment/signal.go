package sentiment

import (
	"context"
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Sentiment measures global market conviction from breadth and leadership
performance. Categories belong in logic; this signal emits numerical scores only.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	ticker *Ticker
}

func NewSignal(ctx context.Context, api *websocket.API) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(ctx, api),
	}
}

func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	tickers := signal.ticker.cache
	out := datura.Map[datura.Map[*decimal.Decimal]]{}

	thesis.CrossSection.Observe(tickers)
	view := thesis.CrossSection.Snapshot()
	breadth := view.Breadth()

	for _, row := range tickers {
		if out[row.Symbol] == nil {
			out[row.Symbol] = datura.Map[*decimal.Decimal]{
				"change":         decimal.NewFromFloat64(0),
				"breadth":        decimal.NewFromFloat64(0),
				"leaderStrength": decimal.NewFromFloat64(0),
				"leaderEvidence": decimal.NewFromFloat64(0),
				"relativeLead":   decimal.NewFromFloat64(0),
				"surgeScore":     decimal.NewFromFloat64(0),
				"divergentScore": decimal.NewFromFloat64(0),
				"slumpScore":     decimal.NewFromFloat64(0),
				"strength":       decimal.NewFromFloat64(0),
			}
		}

		change := row.ChangePct / 100
		leaderStrength := 0.0
		leaderEvidence := 0.0
		relativeLead := 0.0

		if view.IsLeader(row.Symbol, change) {
			leaderStrength = math.Abs(change)
			threshold := view.LeadershipThreshold()
			leaderEvidence = leaderStrength / threshold
			relativeLead = 1
		}

		leaderMass := leaderEvidence / (1 + leaderEvidence)
		surgeScore := breadth * leaderEvidence * math.Max(relativeLead, 1/(1+leaderEvidence))
		divergentScore := (1 - breadth) * relativeLead * leaderEvidence
		slumpScore := (1 - breadth) * (1 - relativeLead) / (1 + leaderMass)
		strength := math.Max(surgeScore, math.Max(divergentScore, slumpScore))

		out[row.Symbol]["change"] = decimal.NewFromFloat64(change)
		out[row.Symbol]["breadth"] = decimal.NewFromFloat64(breadth)
		out[row.Symbol]["leaderStrength"] = decimal.NewFromFloat64(leaderStrength)
		out[row.Symbol]["leaderEvidence"] = decimal.NewFromFloat64(leaderEvidence)
		out[row.Symbol]["relativeLead"] = decimal.NewFromFloat64(relativeLead)
		out[row.Symbol]["surgeScore"] = decimal.NewFromFloat64(surgeScore)
		out[row.Symbol]["divergentScore"] = decimal.NewFromFloat64(divergentScore)
		out[row.Symbol]["slumpScore"] = decimal.NewFromFloat64(slumpScore)
		out[row.Symbol]["strength"] = decimal.NewFromFloat64(strength)
	}

	signal.ticker.cache = signal.ticker.cache[:0]

	thesis.Signals.Store("tickers", tickers)
	thesis.Measurements.Store("sentiment", out)

	return thesis
}

func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
