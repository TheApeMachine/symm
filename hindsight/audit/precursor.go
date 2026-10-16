package audit

import (
	"context"
	"fmt"
	"math/rand"
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
To enter a long excursion, we cross the book by buying at the Best Ask (EntryAsk).
To exit a long excursion, we cross the book by selling at the Best Bid (ExitBid).
The excursion is only profitable if the price appreciation clears both the bid-ask spread
and the round-trip taker trading fees.
*/
type ExcursionRun struct {
	Symbol         string
	EntryAsk       float64
	ExitBid        float64
	FrictionHurdle float64
	Ticks          int64
}

/*
NetReturn computes the realized return of crossing the order book on entry and exit:
NetReturn = (ExitBid - EntryAsk) / EntryAsk - 2 * feeRate
*/
func (run *ExcursionRun) NetReturn(takerFee float64) float64 {
	if run.EntryAsk <= 0 || run.ExitBid <= 0 {
		return 0
	}

	spreadReturn := (run.ExitBid - run.EntryAsk) / run.EntryAsk
	roundTripFee := 2.0 * takerFee

	return spreadReturn - roundTripFee
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
It measures whether regional tokens leading into ignition (Mark B) statistically diverge
from background market noise (outside [start, c]), and whether excursions clear friction.
*/
type PrecursorStage struct {
	*runtime.System
	ch                 chan *data.Value
	violations         []*Violation
	grid               *nmstore.Grid
	tickTokens         map[int64]string
	excursions         []*ExcursionRun
	detectorExcursions []*ExcursionRun
	episodes           []*EpisodeData
	activeRun          *ExcursionRun
	discoveredFeeRate  float64
	clearanceRate      float64
	meanNetReturn      float64
	jsd                float64
	nullJSDMean        float64
	nullJSDP95         float64
	pValue             float64
}

func NewPrecursorStage(ctx context.Context) *PrecursorStage {
	return &PrecursorStage{
		System:     runtime.NewSystem(ctx, "audit:precursor"),
		ch:         make(chan *data.Value, 64),
		violations: make([]*Violation, 0),
		grid:       nmstore.NewGrid(ctx),
		tickTokens: make(map[int64]string),
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
		if measurement == nil {
			stage.violations = append(stage.violations, NewViolation(
				"unknown",
				"measurement",
				"Nil measurement encountered in tape",
				nil,
				nil,
			))

			continue
		}

		if measurement.Role == "error" {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Scope,
				"measurement.error",
				"error measurement encountered",
				measurement,
				nil,
			))
		}

		stage.recordMeasurement(measurement)
	}

	stage.auditPrecursorSeparation()
	stage.auditFrictionClearance()
	return nil
}

func (stage *PrecursorStage) Violations() []*Violation {
	return stage.violations
}

func (stage *PrecursorStage) recordMeasurement(measurement *data.Value) {
	metrics := data.Metrics(measurement)

	if fee, ok := metrics["fee_rate"]; ok && fee > 0 {
		stage.discoveredFeeRate = fee
	}

	// 1. Sensory join: track regional token per tick
	if measurement.Role == "runtime:join" {
		if len(measurement.Layers) > 0 {
			regions := stage.grid.Observe(measurement)
			if len(regions) > 0 && regions[0].Brightness > 0 {
				stage.tickTokens[int64(measurement.Tick)] = fmt.Sprintf("R%02d", regions[0].Key)
			}
		}
		return
	}

	// 2. Production Path: Excursions detected by strategy.Detector
	if measurement.Origin == "detector" {
		class := measurement.Role
		if class != "up" && class != "up_friction" {
			return
		}

		bPrice := metrics["b_price"]
		cPrice := metrics["c_price"]
		bTick := int64(metrics["b_tick"])
		cTick := int64(metrics["c_tick"])
		startTick := int64(metrics["start_tick"])
		if startTick == 0 {
			startTick = int64(metrics["a_tick"])
		}
		endTick := int64(metrics["end_tick"])
		if endTick == 0 {
			endTick = int64(metrics["d_tick"])
		}

		if bPrice > 0 && cPrice > 0 {
			ticks := cTick - bTick

			if ticks <= 0 {
				ticks = 1
			}

			if class == "up" {
				run := &ExcursionRun{
					Symbol:   measurement.Scope,
					EntryAsk: bPrice,
					ExitBid:  cPrice,
					Ticks:    ticks,
				}
				stage.detectorExcursions = append(stage.detectorExcursions, run)
				stage.excursions = append(stage.excursions, run)
			}

			stage.episodes = append(stage.episodes, &EpisodeData{
				Class:     class,
				Symbol:    measurement.Scope,
				StartTick: startTick,
				BTick:     bTick,
				CTick:     cTick,
				EndTick:   endTick,
				BPrice:    bPrice,
				CPrice:    cPrice,
			})
		}

		return
	}

	// 3. Trade / Quote touch execution (evaluated on real and test excursion streams)
	askPrice := metrics["best_price:ask"]
	bidPrice := metrics["best_price:bid"]
	frictionHurdle := metrics["friction_hurdle"]

	if p, ok := metrics["price"]; ok {
		if askPrice <= 0 {
			askPrice = p
		}
		if bidPrice <= 0 {
			bidPrice = p
		}
	}

	if askPrice <= 0 || bidPrice <= 0 {
		return
	}

	if stage.activeRun == nil {
		stage.activeRun = &ExcursionRun{
			Symbol:         measurement.Scope,
			EntryAsk:       askPrice,
			ExitBid:        bidPrice,
			FrictionHurdle: frictionHurdle,
			Ticks:          1,
		}

		return
	}

	stage.activeRun.Ticks++
	stage.activeRun.ExitBid = bidPrice

	if stage.activeRun.Ticks >= 5 {
		stage.excursions = append(stage.excursions, stage.activeRun)
		stage.activeRun = nil
	}
}

/*
auditPrecursorSeparation tests whether token activations in the precursor window leading into Mark B
diverge from background noise outside excursions, tested against a label-shuffled null.
*/
func (stage *PrecursorStage) auditPrecursorSeparation() {
	if len(stage.episodes) == 0 || len(stage.tickTokens) == 0 {
		return
	}

	var precursorTokens []string
	var backgroundTokens []string

	for tick, token := range stage.tickTokens {
		isPrecursor := false
		isExcursion := false

		for _, ep := range stage.episodes {
			if tick >= ep.StartTick && tick <= ep.BTick {
				isPrecursor = true
				break
			}
			if tick >= ep.BTick && tick <= ep.CTick {
				isExcursion = true
				break
			}
		}

		if isPrecursor {
			precursorTokens = append(precursorTokens, token)
			continue
		}

		if !isExcursion {
			backgroundTokens = append(backgroundTokens, token)
		}
	}

	if len(precursorTokens) < 10 || len(backgroundTokens) < 10 {
		return
	}

	p := computeFrequencyDistribution(precursorTokens)
	q := computeFrequencyDistribution(backgroundTokens)
	jsd := computeJSD(p, q)
	stage.jsd = jsd

	permutations := system.Cfg.Audit.Permutations
	if permutations <= 0 {
		permutations = 50
	}

	significance := system.Cfg.Audit.Significance
	if significance <= 0 || significance >= 1 {
		significance = 0.05
	}

	allTokens := append(slices.Clone(precursorTokens), backgroundTokens...)
	nPrecursor := len(precursorTokens)
	exceedCount := 0
	nullJSDs := make([]float64, permutations)
	rng := rand.New(rand.NewSource(17))

	for iter := 0; iter < permutations; iter++ {
		rng.Shuffle(len(allTokens), func(i, j int) {
			allTokens[i], allTokens[j] = allTokens[j], allTokens[i]
		})

		nullP := computeFrequencyDistribution(allTokens[:nPrecursor])
		nullQ := computeFrequencyDistribution(allTokens[nPrecursor:])
		nullVal := computeJSD(nullP, nullQ)
		nullJSDs[iter] = nullVal

		if nullVal >= jsd {
			exceedCount++
		}
	}

	var sumNull float64
	for _, v := range nullJSDs {
		sumNull += v
	}
	nullMean := sumNull / float64(permutations)
	slices.Sort(nullJSDs)
	p95Idx := int(0.95 * float64(permutations))
	if p95Idx >= len(nullJSDs) {
		p95Idx = len(nullJSDs) - 1
	}
	nullP95 := nullJSDs[p95Idx]
	pValue := float64(1+exceedCount) / float64(permutations+1)

	stage.nullJSDMean = nullMean
	stage.nullJSDP95 = nullP95
	stage.pValue = pValue

	if jsd <= nullP95 && pValue > significance {
		stage.violations = append(stage.violations, NewViolation(
			"precursor",
			"precursor.no_divergence",
			fmt.Sprintf("Precursor tokens fail to separate from background noise: JSD = %.4f <= null P95 = %.4f (p-value = %.4f, null mean = %.4f)",
				jsd, nullP95, pValue, nullMean),
			nil,
			nil,
		))
	}
}

/*
auditFrictionClearance tests whether evaluated excursions achieve positive net returns
when crossing the order book (buying at Ask, selling at Bid) after paying round-trip fees.
*/
func (stage *PrecursorStage) auditFrictionClearance() {
	targetRuns := stage.detectorExcursions
	if len(targetRuns) == 0 {
		targetRuns = stage.excursions
	}

	if len(targetRuns) == 0 {
		return
	}

	takerFee := system.Cfg.Audit.TakerFee

	if takerFee <= 0 {
		takerFee = stage.discoveredFeeRate
	}

	if takerFee <= 0 {
		stage.violations = append(stage.violations, NewViolation(
			"excursion",
			"precursor.undefined_fee",
			"Taker fee is not defined (missing in telemetry stream and system.Cfg.Audit.TakerFee)",
			nil,
			nil,
		))
		return
	}

	profitableCount := 0
	var sumNetReturn float64

	for _, excursion := range targetRuns {
		netReturn := excursion.NetReturn(takerFee)
		sumNetReturn += netReturn

		if netReturn > 0 {
			profitableCount++
		}
	}

	clearanceRate := float64(profitableCount) / float64(len(targetRuns))
	meanNetReturn := sumNetReturn / float64(len(targetRuns))

	stage.clearanceRate = clearanceRate
	stage.meanNetReturn = meanNetReturn

	// If evaluated excursions fail to clear order book friction (0% clearance rate),
	// flag an economic violation.
	if clearanceRate == 0 {
		stage.violations = append(stage.violations, NewViolation(
			"excursion",
			"precursor.unprofitable_friction",
			fmt.Sprintf("Excursions fail order book friction clearance: 0 of %d profitable after spread and %.2f%% round-trip fees (mean net return %.2f%%)",
				len(targetRuns), 2.0*takerFee*100, meanNetReturn*100),
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

	if takerFee <= 0 {
		takerFee = stage.discoveredFeeRate
	}

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

	hasEvidence := len(stage.excursions) > 0 || len(stage.episodes) > 0

	return map[string]any{
		"passed":          len(stage.violations) == 0 && hasEvidence,
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
