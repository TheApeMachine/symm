package audit

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
)

/*
ExcursionRun evaluates real order book execution profitability for a market excursion.
To enter a long excursion, we cross the book by buying at the Best Ask (EntryAsk)
the tick recorded at ignition B. To exit, we sell at the Best Bid (ExitBid) the
tick recorded at exhaustion C. The excursion is only profitable if the move
clears both the bid-ask spread and the round-trip taker fees.
*/
type ExcursionRun struct {
	Symbol   string
	EntryAsk float64
	ExitBid  float64
	Ticks    int64
}

/*
NetReturn computes the realized return of crossing the order book on entry and exit:
NetReturn = (ExitBid - EntryAsk) / EntryAsk - 2 * feeRate
*/
func (run *ExcursionRun) NetReturn(takerFee float64) float64 {
	return (run.ExitBid-run.EntryAsk)/run.EntryAsk - 2.0*takerFee
}

/*
EpisodeData records empirical coordinates of an excursion episode on the tape.
*/
type EpisodeData struct {
	Class     string  `json:"class"`
	Symbol    string  `json:"symbol"`
	StartTick int64   `json:"start_tick"`
	BTick     int64   `json:"b_tick"`
	CTick     int64   `json:"c_tick"`
	EndTick   int64   `json:"end_tick"`
	BPrice    float64 `json:"b_price"`
	CPrice    float64 `json:"c_price"`
}

/*
PrecursorStage audits event-centred market excursions and precursor token separation.
It measures whether a symbol's regional tokens leading into ignition (Mark B)
statistically diverge from that symbol's background (outside [A, C]), and
whether up excursions clear friction at the recorded touch.
*/
type PrecursorStage struct {
	*runtime.System
	ch            chan *data.Value
	violations    []*Violation
	grid          *nmstore.Grid
	tickTokens    map[string]map[int64]string
	excursions    []*ExcursionRun
	episodes      []*EpisodeData
	clearanceRate float64
	meanNetReturn float64
	jsd           float64
	nullJSDMean   float64
	nullJSDP95    float64
	pValue        float64
	err           error
}

func NewPrecursorStage(ctx context.Context) *PrecursorStage {
	return &PrecursorStage{
		System:     runtime.NewSystem(ctx, "audit:precursor"),
		ch:         make(chan *data.Value, 64),
		violations: make([]*Violation, 0),
		grid:       nmstore.NewGrid(ctx),
		tickTokens: make(map[string]map[int64]string),
		excursions: make([]*ExcursionRun, 0),
		episodes:   make([]*EpisodeData, 0),
	}
}

func (stage *PrecursorStage) Channel() chan *data.Value {
	return stage.ch
}

func (stage *PrecursorStage) Run() error {
	errnie.Info(fmt.Sprintf("%s: Running audit...", stage.Name()))

	for measurement := range stage.ch {
		if stage.err != nil {
			continue
		}

		if measurement == nil {
			stage.err = errnie.Err(errnie.Validation, stage.Name()+": nil measurement on the tape", nil)
			continue
		}

		stage.err = stage.recordMeasurement(measurement)
	}

	if stage.err != nil {
		return stage.err
	}

	stage.auditPrecursorSeparation()
	stage.auditFrictionClearance()
	return nil
}

func (stage *PrecursorStage) Violations() []*Violation {
	return stage.violations
}

func (stage *PrecursorStage) recordMeasurement(measurement *data.Value) error {
	// 1. Sensory join: the symbol's dominant region per tick.
	if measurement.Role == "runtime:join" {
		symbol := measurement.Scope
		signals := make([]data.Signal, 0)

		for signal, err := range data.Signals(measurement) {
			if err != nil {
				return err
			}

			if signal.Symbol != symbol {
				continue
			}

			signals = append(signals, signal)
		}

		if err := stage.grid.Error(); err != nil {
			return err
		}

		regions := stage.grid.Observe(symbol, signals)

		if len(regions) > 0 && regions[0].Brightness > 0 {
			tokens, ok := stage.tickTokens[measurement.Scope]

			if !ok {
				tokens = make(map[int64]string)
				stage.tickTokens[measurement.Scope] = tokens
			}

			tokens[int64(measurement.Tick)] = regionToken(regions[0].Key)
		}

		return nil
	}

	if measurement.Origin != data.ExcursionOrigin {
		return errnie.Err(errnie.Validation, "audit:precursor: unexpected measurement "+measurement.Prefix, nil)
	}

	// 2. Excursions detected by strategy.Detector.
	fields, err := data.ExcursionFields(measurement)

	if err != nil {
		return errnie.Err(errnie.UnprocessableContent, "audit:precursor", err)
	}

	if fields["b_ask"] <= 0 || fields["c_bid"] <= 0 {
		return errnie.Err(errnie.Validation, fmt.Sprintf(
			"audit:precursor: %s has a non-positive touch (b_ask %v, c_bid %v)",
			measurement.Prefix, fields["b_ask"], fields["c_bid"],
		), nil)
	}

	episode := &EpisodeData{
		Class:     measurement.Role,
		Symbol:    measurement.Scope,
		StartTick: int64(fields["a_tick"]),
		BTick:     int64(fields["b_tick"]),
		CTick:     int64(fields["c_tick"]),
		EndTick:   int64(fields["d_tick"]),
		BPrice:    fields["b_price"],
		CPrice:    fields["c_price"],
	}
	stage.episodes = append(stage.episodes, episode)

	if measurement.Role == "up" {
		stage.excursions = append(stage.excursions, &ExcursionRun{
			Symbol:   measurement.Scope,
			EntryAsk: fields["b_ask"],
			ExitBid:  fields["c_bid"],
			Ticks:    episode.CTick - episode.BTick,
		})
	}

	return nil
}

/*
auditPrecursorSeparation tests whether token activations in the precursor
window [A, B] of up/down excursions diverge from the same symbol's
background (outside [A, C]), tested against a label-shuffled null.
*/
func (stage *PrecursorStage) auditPrecursorSeparation() {
	cfg := system.Cfg.Audit
	var precursorTokens []string
	var backgroundTokens []string

	for symbol, tokens := range stage.tickTokens {
		for tick, token := range tokens {
			isPrecursor := false
			isExcursion := false

			for _, ep := range stage.episodes {
				if ep.Symbol != symbol || (ep.Class != "up" && ep.Class != "down") {
					continue
				}

				if tick >= ep.StartTick && tick <= ep.BTick {
					isPrecursor = true
					break
				}

				if tick > ep.BTick && tick <= ep.CTick {
					isExcursion = true
					break
				}
			}

			if isPrecursor {
				precursorTokens = append(precursorTokens, token)
			} else if !isExcursion {
				backgroundTokens = append(backgroundTokens, token)
			}
		}
	}

	if len(precursorTokens) < cfg.MinPrecursorTokens || len(backgroundTokens) < cfg.MinPrecursorTokens {
		stage.violations = append(stage.violations, insufficient("precursor", fmt.Sprintf(
			"%d precursor and %d background tokens over %d episodes; need %d of each",
			len(precursorTokens), len(backgroundTokens), len(stage.episodes), cfg.MinPrecursorTokens,
		)))
		return
	}

	// Map iteration order is random: sort so the seeded null reproduces.
	slices.Sort(precursorTokens)
	slices.Sort(backgroundTokens)

	stage.jsd = tokenJSD(precursorTokens, backgroundTokens)
	allTokens := append(slices.Clone(precursorTokens), backgroundTokens...)
	stage.pValue, stage.nullJSDMean, stage.nullJSDP95 = permutationNull(
		allTokens, len(precursorTokens), stage.jsd, cfg.Permutations, tokenJSD,
	)

	if stage.jsd <= stage.nullJSDP95 && stage.pValue > cfg.Significance {
		stage.violations = append(stage.violations, NewViolation(
			"precursor",
			"precursor.no_divergence",
			fmt.Sprintf("Precursor tokens fail to separate from background noise: JSD = %.4f <= null P95 = %.4f (p-value = %.4f, null mean = %.4f)",
				stage.jsd, stage.nullJSDP95, stage.pValue, stage.nullJSDMean),
			nil,
			nil,
		))
	}
}

/*
auditFrictionClearance tests whether detected up excursions achieve positive
net returns when crossing the recorded touch (buy the ask at B, sell the bid
at C) after paying round-trip taker fees at audit.taker_fee.
*/
func (stage *PrecursorStage) auditFrictionClearance() {
	if len(stage.excursions) == 0 {
		stage.violations = append(stage.violations, insufficient("excursion", fmt.Sprintf(
			"no up excursions among %d detected episodes: run detect on the epoch", len(stage.episodes),
		)))
		return
	}

	takerFee := system.Cfg.Audit.TakerFee
	profitableCount := 0
	var sumNetReturn float64

	for _, excursion := range stage.excursions {
		netReturn := excursion.NetReturn(takerFee)
		sumNetReturn += netReturn

		if netReturn > 0 {
			profitableCount++
		}
	}

	stage.clearanceRate = float64(profitableCount) / float64(len(stage.excursions))
	stage.meanNetReturn = sumNetReturn / float64(len(stage.excursions))

	if profitableCount == 0 {
		stage.violations = append(stage.violations, NewViolation(
			"excursion",
			"precursor.unprofitable_friction",
			fmt.Sprintf("Excursions fail order book friction clearance: 0 of %d profitable after spread and %.2f%% round-trip fees (mean net return %.2f%%)",
				len(stage.excursions), 2.0*takerFee*100, stage.meanNetReturn*100),
			nil,
			nil,
		))
	}
}

func (stage *PrecursorStage) Report() string {
	var build strings.Builder

	for _, violation := range stage.violations {
		build.WriteString(violation.String())
	}

	return build.String()
}

func (stage *PrecursorStage) Data() any {
	takerFee := system.Cfg.Audit.TakerFee
	excursionData := make([]map[string]any, 0, len(stage.excursions))

	for _, ex := range stage.excursions {
		excursionData = append(excursionData, map[string]any{
			"symbol":     ex.Symbol,
			"entry_ask":  ex.EntryAsk,
			"exit_bid":   ex.ExitBid,
			"ticks":      ex.Ticks,
			"net_return": ex.NetReturn(takerFee),
		})
	}

	return map[string]any{
		"passed":          len(stage.violations) == 0,
		"excursion_count": len(stage.excursions),
		"clearance_rate":  stage.clearanceRate,
		"mean_net_return": stage.meanNetReturn,
		"taker_fee":       takerFee,
		"jsd":             stage.jsd,
		"null_jsd_mean":   stage.nullJSDMean,
		"null_jsd_p95":    stage.nullJSDP95,
		"p_value":         stage.pValue,
		"excursions":      excursionData,
		"episodes":        stage.episodes,
	}
}
