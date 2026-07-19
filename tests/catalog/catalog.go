package catalog

import (
	"iter"

	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

/*
ScenarioKind names an opportunity or trap tape identity from AGENTS.md.
*/
type ScenarioKind string

const (
	KindPump                ScenarioKind = "pump"
	KindCoil                ScenarioKind = "coil"
	KindExhaustion          ScenarioKind = "exhaustion"
	KindVacuum              ScenarioKind = "vacuum"
	KindSectorLift          ScenarioKind = "sector_lift"
	KindThinBook            ScenarioKind = "thin_book"
	KindNoise               ScenarioKind = "noise"
	KindPhantomQuote        ScenarioKind = "phantom_quote"
	KindToxicChase          ScenarioKind = "toxic_chase"
	KindUnfundableLot       ScenarioKind = "unfundable_lot"
	KindReversalExit        ScenarioKind = "reversal_exit"
	KindLagNoLead           ScenarioKind = "lag_no_lead"
	KindFundedSlice         ScenarioKind = "funded_slice"
	KindPhantomHold         ScenarioKind = "phantom_hold"
	KindShallowAdverseHold  ScenarioKind = "shallow_adverse_hold"
	KindSincereStop         ScenarioKind = "sincere_stop"
	KindCalibratedFloorHold ScenarioKind = "calibrated_floor_hold"
	KindUngatedAboveStop    ScenarioKind = "ungated_above_stop"
	KindMonotonePullback    ScenarioKind = "monotone_pullback"
	KindStickyRetreatHold   ScenarioKind = "sticky_retreat_hold"
)

/*
MeasureBound is how AssertMeasure reads PeakSourceMetric.
*/
type MeasureBound string

const (
	// MeasureBoundPositive (default/empty) requires peak > 0.
	MeasureBoundPositive MeasureBound = ""
	// MeasureBoundPresent requires a found non-zero peak (signed ok).
	MeasureBoundPresent MeasureBound = "present"
	// MeasureBoundZero requires a found peak == 0 (e.g. no lead claim).
	MeasureBoundZero MeasureBound = "zero"
)

/*
WalletBound is the known-correct cash outcome after strategy.
*/
type WalletBound string

const (
	// WalletBoundNone leaves wallet cash unclaimed.
	WalletBoundNone WalletBound = ""
	// WalletBoundPreserve requires after >= before (no deploy).
	WalletBoundPreserve WalletBound = "preserve"
	// WalletBoundDeploy requires after < before (cash left the wallet).
	WalletBoundDeploy WalletBound = "deploy"
)

/*
StageTruth is the known-correct system behavior at one verification stage.
*/
type StageTruth struct {
	MeasureSource types.SourceType
	MeasureMetric types.MetricType
	MeasureBound  MeasureBound
	DecideAction  types.Action
	MustNotEnter  bool
	SizedEnter    bool
	WalletBound   WalletBound

	// Exit honesty (open-lot Regulate via CommitStrategy after PlayOpen):
	// MustNotExit requires no ActionExit; ExitCause requires that Cause.
	MustNotExit bool
	ExitCause   string
	// MarkMul is the regulate mark relative to entry (e.g. 0.992 ≈ −0.8%).
	MarkMul float64
	// PeakMul ratchets PeakReturn before an adverse MarkMul (sincere stops).
	PeakMul float64
	// RetreatPressure > 0 seeds toxicity retreat so quote marks freeze geometry.
	RetreatPressure float64
	// TrailDistance binds Stoploss at PlayOpen; required > 0 for exit proofs.
	TrailDistance float64
	// KeepForecast seeds a forward path so take-profit does not fire on noise.
	KeepForecast bool
	ForecastER   float64
	ForecastUnc  float64
	// StickyRetreat re-runs CommitStrategy without re-seeding retreat pressure.
	StickyRetreat bool
	// MinStopReturn when > 0 requires Stoploss.StopReturn >= MinStopReturn.
	MinStopReturn float64
}

/*
Entry is one controllable multi-leg tape with known stage and wallet outcomes.
*/
type Entry struct {
	Kind    ScenarioKind
	Name    string
	Symbol  string
	Capital float64
	FeePct  float64
	Truth   StageTruth
	Market  func() *tests.Market
}

/*
All is the opportunity/trap surface the simulation must prove.
*/
func All() []Entry {
	return []Entry{
		{
			Kind: KindPump, Name: "pump_ignition", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MeasureSource: types.SourcePumpDump,
				MeasureMetric: types.MetricRVOL,
				DecideAction:  types.ActionEnter,
				SizedEnter:    true,
				WalletBound:   WalletBoundDeploy,
			},
			Market: conditions.TapePump,
		},
		{
			Kind: KindCoil, Name: "coil_breakout", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MeasureSource: types.SourcePumpDump,
				MeasureMetric: types.MetricRVOL,
				DecideAction:  types.ActionEnter,
				SizedEnter:    true,
				WalletBound:   WalletBoundDeploy,
			},
			Market: conditions.TapeCoil,
		},
		{
			Kind: KindExhaustion, Name: "exhaustion_reject", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MeasureSource: types.SourceExhaustion,
				MeasureMetric: types.MetricMechanical,
				MustNotEnter:  true,
				WalletBound:   WalletBoundPreserve,
			},
			Market: conditions.TapeExhaustion,
		},
		{
			Kind: KindVacuum, Name: "liquidity_vacuum", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MeasureSource: types.SourceExhaustion,
				MeasureMetric: types.MetricMechanical,
				MustNotEnter:  true,
				WalletBound:   WalletBoundPreserve,
			},
			Market: conditions.TapeVacuum,
		},
		{
			Kind: KindSectorLift, Name: "sector_herd_lift", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MeasureSource: types.SourceCorrelation,
				MeasureMetric: types.MetricHerdScore,
				DecideAction:  types.ActionEnter,
				SizedEnter:    true,
				WalletBound:   WalletBoundDeploy,
			},
			Market: conditions.TapeSectorLift,
		},
		{
			Kind: KindThinBook, Name: "thin_book_trap", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MeasureSource: types.SourceLiquidity,
				MeasureMetric: types.MetricScarcityScore,
				MustNotEnter:  true,
				WalletBound:   WalletBoundPreserve,
			},
			Market: conditions.TapeThinBook,
		},
		{
			Kind: KindNoise, Name: "noise_no_herd", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MeasureSource: types.SourceSentiment,
				MeasureMetric: types.MetricChange,
				MeasureBound:  MeasureBoundPresent,
				MustNotEnter:  true,
				WalletBound:   WalletBoundPreserve,
			},
			Market: conditions.TapeNoise,
		},
		{
			Kind: KindPhantomQuote, Name: "phantom_quote_retreat", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MustNotEnter: true,
				WalletBound:  WalletBoundPreserve,
			},
			Market: conditions.TapePhantomQuote,
		},
		{
			Kind: KindToxicChase, Name: "toxic_aggression", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MeasureSource: types.SourceCVD,
				MeasureMetric: types.MetricDrive,
				MustNotEnter:  true,
				WalletBound:   WalletBoundPreserve,
			},
			Market: conditions.TapeToxicChase,
		},
		{
			Kind: KindToxicChase, Name: "hawkes_aggression", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MeasureSource: types.SourceHawkes,
				MeasureMetric: types.MetricEventCount,
				MustNotEnter:  true,
				WalletBound:   WalletBoundPreserve,
			},
			Market: conditions.TapeToxicChase,
		},
		{
			Kind: KindUnfundableLot, Name: "unfundable_minimum", Symbol: "MATIC/USD",
			Capital: 1, FeePct: 0.26,
			Truth: StageTruth{
				MustNotEnter: true,
				WalletBound:  WalletBoundPreserve,
			},
			Market: conditions.TapePump,
		},
		{
			Kind: KindReversalExit, Name: "reversal_after_entry", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				DecideAction: types.ActionEnter,
				SizedEnter:   true,
				WalletBound:  WalletBoundDeploy,
			},
			Market: conditions.TapePumpThenReversal,
		},
		{
			Kind: KindLagNoLead, Name: "lag_without_lead", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MeasureSource: types.SourceLeadLag,
				MeasureMetric: types.MetricStrength,
				MeasureBound:  MeasureBoundZero,
				MustNotEnter:  true,
				WalletBound:   WalletBoundPreserve,
			},
			Market: conditions.TapeLag,
		},
		{
			Kind: KindFundedSlice, Name: "funded_max_fraction", Symbol: "MATIC/USD",
			Capital: 5_000, FeePct: 0.26,
			Truth: StageTruth{
				DecideAction: types.ActionEnter,
				SizedEnter:   true,
				WalletBound:  WalletBoundDeploy,
			},
			Market: conditions.TapePump,
		},
		{
			Kind: KindPhantomHold, Name: "phantom_hold_under_retreat", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MustNotExit:     true,
				MarkMul:         0.985,
				RetreatPressure: 0.95,
				TrailDistance:   0.0026,
				KeepForecast:    true,
				ForecastER:      0.05,
				ForecastUnc:     0.02,
			},
			Market: conditions.TapePhantomQuote,
		},
		{
			Kind: KindShallowAdverseHold, Name: "shallow_adverse_hold", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MustNotExit:     true,
				MarkMul:         0.992,
				RetreatPressure: 0.95,
				TrailDistance:   0.0026,
				KeepForecast:    true,
				ForecastER:      0.05,
				ForecastUnc:     0.02,
			},
			Market: conditions.TapeShallowAdverse,
		},
		{
			Kind: KindSincereStop, Name: "sincere_drawdown_stop", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				ExitCause:     "stop",
				PeakMul:       1.02,
				MarkMul:       0.97,
				TrailDistance: 0.0026,
				KeepForecast:  true,
				ForecastER:    0.01,
				ForecastUnc:   0.01,
			},
			Market: conditions.TapeDrawdownStop,
		},
		{
			Kind: KindCalibratedFloorHold, Name: "calibrated_floor_hold", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MustNotExit:   true,
				MarkMul:       1.04,
				TrailDistance: 0.02,
				KeepForecast:  true,
				ForecastER:    0.05,
				ForecastUnc:   0.02,
			},
			Market: conditions.TapeCalibratedLift,
		},
		{
			Kind: KindUngatedAboveStop, Name: "ungated_above_stop_hold", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MustNotExit:   true,
				MarkMul:       0.999,
				TrailDistance: 0.0026,
				KeepForecast:  true,
				ForecastER:    0.05,
				ForecastUnc:   0.02,
			},
			Market: conditions.TapeShallowAdverse,
		},
		{
			Kind: KindMonotonePullback, Name: "monotone_stop_after_pullback", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MustNotExit:   true,
				PeakMul:       1.08,
				MarkMul:       1.07,
				TrailDistance: 0.02,
				KeepForecast:  true,
				ForecastER:    0.05,
				ForecastUnc:   0.02,
				MinStopReturn: 0.06,
			},
			Market: conditions.TapeCalibratedLift,
		},
		{
			Kind: KindStickyRetreatHold, Name: "sticky_retreat_without_reseed", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MustNotExit:     true,
				MarkMul:         0.985,
				RetreatPressure: 0.95,
				TrailDistance:   0.0026,
				KeepForecast:    true,
				ForecastER:      0.05,
				ForecastUnc:     0.02,
				StickyRetreat:   true,
			},
			Market: conditions.TapePhantomQuote,
		},
	}
}

/*
IsExitProof reports whether the entry's known truth is open-lot Regulate behavior.
*/
func (entry Entry) IsExitProof() bool {
	return entry.Truth.MustNotExit || entry.Truth.ExitCause != ""
}

/*
Frames yields the catalog tape frames for Play.
*/
func (entry Entry) Frames() iter.Seq[tests.Frame] {
	if entry.Market == nil {
		return func(yield func(tests.Frame) bool) {}
	}

	return entry.Market().Frames()
}
