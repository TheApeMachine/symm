package audit

import (
	"context"
	"fmt"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
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
PrecursorStage audits event-centred market excursions and friction clearance economics
using order book touch execution (Best Ask entry, Best Bid exit) and taker fees.
*/
type PrecursorStage struct {
	*runtime.System
	ch                chan *data.Measurement
	violations        []*Violation
	excursions        []*ExcursionRun
	episodes          []*EpisodeData
	activeRun         *ExcursionRun
	discoveredFeeRate float64
	clearanceRate     float64
	meanNetReturn     float64
}

func NewPrecursorStage(ctx context.Context) *PrecursorStage {
	return &PrecursorStage{
		System:     runtime.NewSystem(ctx, "audit:precursor"),
		ch:         make(chan *data.Measurement, 64),
		violations: make([]*Violation, 0),
		excursions: make([]*ExcursionRun, 0),
		episodes:   make([]*EpisodeData, 0),
	}
}

func (stage *PrecursorStage) Channel() chan *data.Measurement {
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

		if measurement.Error() != nil {
			stage.violations = append(stage.violations, NewViolation(
				measurement.Label,
				"measurement.error",
				measurement.Error().Error(),
				measurement,
				nil,
			))
		}

		stage.trackExcursions(measurement)
	}

	stage.auditFrictionClearance()
	return nil
}

func (stage *PrecursorStage) Violations() []*Violation {
	return stage.violations
}

/*
trackExcursions processes empirical excursions emitted by strategy.Detector
or evaluates touch execution on explicit test excursion runs.
*/
func (stage *PrecursorStage) trackExcursions(measurement *data.Measurement) {
	if measurement.Source == "runtime:join" {
		return
	}

	for entry := range measurement.Read("fee_rate") {
		if entry != nil && entry.Metric != nil && entry.Metric.Raw() > 0 {
			stage.discoveredFeeRate = entry.Metric.Raw()
		}
	}

	// 1. Production Path: Excursions detected by strategy.Detector
	if measurement.Source == "detector" {
		typeEntry := data.Pull(measurement.Meta("type"))

		if typeEntry == nil {
			return
		}

		class := typeEntry.Value

		if class != "up" && class != "up_friction" {
			return
		}

		var bPrice float64
		var cPrice float64
		var bTick int64
		var cTick int64
		var startTick int64
		var endTick int64

		for entry := range measurement.Read() {
			if entry == nil || entry.Metric == nil {
				continue
			}

			switch entry.Metric.Label() {
			case "b_price":
				bPrice = entry.Metric.Raw()
			case "c_price":
				cPrice = entry.Metric.Raw()
			case "b_tick":
				bTick = int64(entry.Metric.Raw())
			case "c_tick":
				cTick = int64(entry.Metric.Raw())
			case "start_tick":
				startTick = int64(entry.Metric.Raw())
			case "end_tick":
				endTick = int64(entry.Metric.Raw())
			}
		}

		if bPrice > 0 && cPrice > 0 {
			ticks := cTick - bTick

			if ticks <= 0 {
				ticks = 1
			}

			if class == "up" {
				stage.excursions = append(stage.excursions, &ExcursionRun{
					Symbol:   measurement.Label,
					EntryAsk: bPrice,
					ExitBid:  cPrice,
					Ticks:    ticks,
				})
			}

			stage.episodes = append(stage.episodes, &EpisodeData{
				Class:     class,
				Symbol:    measurement.Label,
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

	// 2. Test Path: Order book touch execution in test runs
	if measurement.Source != "test" {
		return
	}

	var askPrice float64
	var bidPrice float64
	var frictionHurdle float64

	for entry := range measurement.Read() {
		if entry == nil || entry.Metric == nil {
			continue
		}

		switch entry.Metric.Label() {
		case "best_ask_price":
			askPrice = entry.Metric.Raw()
		case "best_bid_price":
			bidPrice = entry.Metric.Raw()
		case "friction_hurdle":
			frictionHurdle = entry.Metric.Raw()
		case "price":
			if askPrice <= 0 {
				askPrice = entry.Metric.Raw()
			}

			if bidPrice <= 0 {
				bidPrice = entry.Metric.Raw()
			}
		}
	}

	if askPrice <= 0 || bidPrice <= 0 {
		return
	}

	if stage.activeRun == nil {
		stage.activeRun = &ExcursionRun{
			Symbol:         measurement.Label,
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
auditFrictionClearance tests whether evaluated excursions achieve positive net returns
when crossing the order book (buying at Ask, selling at Bid) after paying round-trip fees.
*/
func (stage *PrecursorStage) auditFrictionClearance() {
	if len(stage.excursions) == 0 {
		return
	}

	takerFee := system.Cfg.Audit.TakerFee

	if takerFee <= 0 {
		takerFee = stage.discoveredFeeRate
	}

	if takerFee <= 0 {
		takerFee = 0.0026
	}

	profitableCount := 0
	var sumNetReturn float64

	for _, excursion := range stage.excursions {
		netReturn := excursion.NetReturn(takerFee)
		sumNetReturn += netReturn

		if netReturn > 0 {
			profitableCount++
		}
	}

	clearanceRate := float64(profitableCount) / float64(len(stage.excursions))
	meanNetReturn := sumNetReturn / float64(len(stage.excursions))

	stage.clearanceRate = clearanceRate
	stage.meanNetReturn = meanNetReturn

	// If evaluated excursions fail to clear order book friction (0% clearance rate),
	// flag an economic violation.
	if len(stage.excursions) >= 5 && clearanceRate == 0 {
		stage.violations = append(stage.violations, NewViolation(
			"excursion",
			"precursor.unprofitable_friction",
			fmt.Sprintf("Excursions fail order book friction clearance: 0 of %d profitable after spread and %.2f%% round-trip fees (mean net return %.2f%%)",
				len(stage.excursions), 2.0*takerFee*100, meanNetReturn*100),
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

	if takerFee <= 0 {
		takerFee = 0.0026
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

	return map[string]any{
		"passed":          len(stage.violations) == 0,
		"excursion_count": len(stage.excursions),
		"clearance_rate":  stage.clearanceRate,
		"mean_net_return": stage.meanNetReturn,
		"taker_fee":       takerFee,
		"excursions":      excursionData,
		"episodes":        stage.episodes,
	}
}

