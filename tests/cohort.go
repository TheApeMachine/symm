package tests

import (
	"iter"
	"math"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

/*
CohortMode selects how multi-symbol ticker paths relate across frames.
*/
type CohortMode int

const (
	// CohortHerd moves every symbol with the same price multiplier path.
	CohortHerd CohortMode = iota
	// CohortLag delays follower symbols by lagFrames relative to the leader.
	CohortLag
	// CohortNoise applies independent deterministic drifts per symbol.
	CohortNoise
	// CohortAlpha moves the subject with its peers but at greater return energy.
	CohortAlpha
	// CohortDivergent moves the subject up while both peers move down.
	CohortDivergent
	// CohortSlump moves every symbol down together without inventing a leader.
	CohortSlump
	// CohortStall establishes a leader, then stops it while peers remain active.
	CohortStall
)

/*
Cohort emits one ticker update per step containing every symbol, so Market.Cut
builds a real CrossSection for liquidity, leadlag, correlation, and sentiment.
*/
type Cohort struct {
	symbols   []string
	horizon   int
	mode      CohortMode
	lagFrames int
	base      map[string]any
}

/*
NewCohort constructs a multi-symbol ticker fixture from a single-symbol ticker
update template. The first symbol is the leader for CohortLag.
*/
func NewCohort(
	basePayload []byte,
	symbols []string,
	horizon int,
	mode CohortMode,
	lagFrames int,
) *Cohort {
	if horizon < 1 {
		panic(errnie.Err(errnie.Validation, "tests: cohort horizon must be positive", nil))
	}

	if len(symbols) < 2 {
		panic(errnie.Err(errnie.Validation, "tests: cohort needs at least two symbols", nil))
	}

	var base map[string]any

	if err := sonic.Unmarshal(basePayload, &base); err != nil {
		panic(errnie.Err(errnie.Validation, "tests: cohort base decode failed", err))
	}

	return &Cohort{
		symbols:   append([]string(nil), symbols...),
		horizon:   horizon,
		mode:      mode,
		lagFrames: max(lagFrames, 1),
		base:      base,
	}
}

/*
Generate yields cohort ticker payloads.
*/
func (cohort *Cohort) Generate() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for frame := range cohort.Frames() {
			if !yield(frame.Payload) {
				return
			}
		}
	}
}

/*
Frames yields channel-tagged cohort ticker updates.
*/
func (cohort *Cohort) Frames() iter.Seq[Frame] {
	return func(yield func(Frame) bool) {
		template := cohort.templateRow()

		for step := 0; step < cohort.horizon; step++ {
			rows := make([]any, 0, len(cohort.symbols))

			for symbolIndex, symbol := range cohort.symbols {
				row := cloneMap(template)
				row["symbol"] = symbol
				mul := cohort.priceMul(step, symbolIndex)
				scaleRowFields(row, mul, priceFields["ticker"])
				scaleRowFields(row, 1+0.02*float64(step), volumeFields["ticker"])
				row["change_pct"] = (mul - 1) * 100

				if stamp, ok := row["timestamp"].(string); ok {
					row["timestamp"] = advanceTimestamp(
						stamp, time.Duration(step)*5*time.Second,
					)
				}

				rows = append(rows, row)
			}

			payload := cloneMap(cohort.base)
			payload["type"] = "update"
			payload["data"] = rows

			frame := Frame{
				Channel: "ticker",
				Type:    "update",
				Payload: marshalFrame(payload),
			}

			if !yield(frame) {
				return
			}
		}
	}
}

func (cohort *Cohort) templateRow() map[string]any {
	rows := frameRows(cohort.base)

	if len(rows) == 0 {
		panic(errnie.Err(errnie.Validation, "tests: cohort base has no ticker rows", nil))
	}

	return cloneMap(rows[0])
}

func (cohort *Cohort) priceMul(step int, symbolIndex int) float64 {
	switch cohort.mode {
	case CohortHerd:
		return 1 + 0.01*float64(step)
	case CohortLag:
		if symbolIndex == 0 {
			return cohort.lagPriceMul(step)
		}

		delayed := step - cohort.lagFrames

		if delayed < 0 {
			delayed = 0
		}

		return cohort.lagPriceMul(delayed)
	case CohortNoise:
		phase := float64(symbolIndex+1) * 1.7
		// Cap the oscillation so long paths cannot produce non-positive prices.
		drift := 0.008 * float64(step) * math.Sin(phase+float64(step)*0.35)
		const noiseDriftCeiling = 0.15

		return 1 + math.Max(-noiseDriftCeiling, math.Min(noiseDriftCeiling, drift))
	case CohortAlpha:
		if symbolIndex == 0 {
			return 1 + 0.03*float64(step)
		}

		return 1 + 0.005*float64(step)
	case CohortDivergent:
		if symbolIndex == 0 {
			return 1 + 0.02*float64(step)
		}

		return 1 - 0.01*float64(step)
	case CohortSlump:
		return 1 - 0.01*float64(step)
	case CohortStall:
		if symbolIndex == 0 {
			return 1 + 0.01*float64(min(step, cohort.horizon/2))
		}

		phase := float64(symbolIndex+1) * 1.3
		return 1 + 0.002*float64(step)*math.Sin(phase+float64(step)*0.55)
	default:
		return 1
	}
}

/*
lagPriceMul creates a drifting but non-linear leader path whose delayed copy
has identifiable timing; a straight ramp would remain contemporaneously
correlated after an intercept shift and would not constitute a lead-lag proof.
*/
func (cohort *Cohort) lagPriceMul(step int) float64 {
	return 1 + 0.004*float64(step) + 0.02*math.Sin(float64(step)*0.7)
}

func cloneMap(base map[string]any) map[string]any {
	raw, err := sonic.Marshal(base)

	if err != nil {
		panic(err)
	}

	var out map[string]any

	if err := sonic.Unmarshal(raw, &out); err != nil {
		panic(err)
	}

	return out
}
