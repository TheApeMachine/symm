package leadlag

import (
	"context"
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
LeadLag is the Anchor perspective, measuring temporal correlation between the
cross-section leader and each follower. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	ctx     context.Context
	cancel  context.CancelFunc
	ticker  *Ticker
	section *Section
}

func NewSignal(ctx context.Context, api *websocket.API) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:     ctx,
		cancel:  cancel,
		ticker:  NewTicker(ctx, api),
		section: NewSection(),
	}
}

func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	rows := signal.ticker.cache
	out := datura.Map[datura.Map[*decimal.Decimal]]{}

	thesis.CrossSection.Observe(rows)
	view := thesis.CrossSection.Snapshot()
	anchor := view.Leader()

	if anchor != "" {
		signal.section.SetAnchor(anchor)

		for _, row := range rows {
			if row.Timestamp.IsZero() {
				continue
			}

			if row.Last == nil {
				continue
			}

			lastPrice := row.Last.Float64()

			if lastPrice <= 0 {
				continue
			}

			signal.section.ObservePrice(row.Symbol, lastPrice, row.Timestamp)
		}

		for _, row := range rows {
			features := signal.section.Features(row.Symbol)

			if features.Price <= 0 {
				continue
			}

			lagFraction := 0.0
			lagCorrelation := 0.0
			contempCorrelation := 0.0
			signedLagCorrelation := 0.0
			signedContempCorrelation := 0.0

			if features.LagOK && features.SampleCount > 0 {
				dynamicMax := signal.section.maxLagBars(features.SampleCount)

				if dynamicMax > 0 {
					lagFraction = math.Abs(float64(features.LagBars)) / float64(dynamicMax)
				}

				signedLagCorrelation = features.LagCorr
				lagCorrelation = math.Abs(features.LagCorr)
			}

			if features.ContempOK {
				signedContempCorrelation = features.ContempCorr
				contempCorrelation = math.Abs(features.ContempCorr)
			}

			correlation := min(math.Max(contempCorrelation, lagCorrelation), 1)

			lagDominates := max(0, min(1, (lagCorrelation-contempCorrelation)*1e9))
			signedCorrelation := min(max(
				signedContempCorrelation+lagDominates*(signedLagCorrelation-signedContempCorrelation),
				-1,
			), 1)

			sampleSupport := 0.0

			if features.SampleCount > 0 {
				shortWindow, _, err := statistic.ResolveWindows(
					make([]float64, features.SampleCount),
					0,
					0,
				)

				if err == nil && shortWindow > 0 {
					sampleSupport = float64(features.SampleCount) / float64(shortWindow)
				}
			}

			anchorActive := 0.1

			if features.MoveMoved ||
				(features.StallMargin > 0 && lagFraction > 0) ||
				features.ContempOK ||
				features.LagOK {
				anchorActive = 1
			}

			stallDamp := 1.0

			if features.MoveMoved {
				stallDamp = 0
			}

			stallMargin := math.Min(1, math.Max(0, features.StallMargin))
			noLag := 1 - lagFraction
			uncorrelated := 1 - correlation
			lagEvidence := lagCorrelation * lagFraction
			syncEvidence := contempCorrelation * noLag
			decoupledEvidence := uncorrelated * (1 - stallMargin)
			stallEvidence := stallMargin * uncorrelated * noLag * stallDamp

			inefficient := sampleSupport * anchorActive * lagEvidence * (1 - stallMargin)
			syncScore := sampleSupport * anchorActive * syncEvidence * (1 - stallMargin)
			decoupled := sampleSupport * anchorActive * decoupledEvidence
			stall := sampleSupport * anchorActive * stallEvidence
			strength := max(max(inefficient, syncScore), max(decoupled, stall))

			if strength <= 0 {
				strength = 0.01
			}

			out[row.Symbol] = datura.Map[*decimal.Decimal]{
				"correlation":              decimal.NewFromFloat64(correlation),
				"signedCorrelation":        decimal.NewFromFloat64(signedCorrelation),
				"signedContempCorrelation": decimal.NewFromFloat64(signedContempCorrelation),
				"signedLagCorrelation":     decimal.NewFromFloat64(signedLagCorrelation),
				"lagFraction":              decimal.NewFromFloat64(lagFraction),
				"sampleSupport":            decimal.NewFromFloat64(sampleSupport),
				"inefficient":              decimal.NewFromFloat64(inefficient),
				"sync":                     decimal.NewFromFloat64(syncScore),
				"decoupled":                decimal.NewFromFloat64(decoupled),
				"stall":                    decimal.NewFromFloat64(stall),
				"strength":                 decimal.NewFromFloat64(strength),
			}
		}
	}

	signal.ticker.cache = signal.ticker.cache[:0]

	thesis.Signals.Store("tickers", rows)
	thesis.Measurements.Store("leadlag", out)

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
