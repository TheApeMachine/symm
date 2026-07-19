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
	KindPump          ScenarioKind = "pump"
	KindCoil          ScenarioKind = "coil"
	KindExhaustion    ScenarioKind = "exhaustion"
	KindVacuum        ScenarioKind = "vacuum"
	KindSectorLift    ScenarioKind = "sector_lift"
	KindThinBook      ScenarioKind = "thin_book"
	KindNoise         ScenarioKind = "noise"
	KindPhantomQuote  ScenarioKind = "phantom_quote"
	KindToxicChase    ScenarioKind = "toxic_chase"
	KindUnfundableLot ScenarioKind = "unfundable_lot"
	KindReversalExit  ScenarioKind = "reversal_exit"
	KindLagNoLead     ScenarioKind = "lag_no_lead"
	KindFundedSlice   ScenarioKind = "funded_slice"
)

/*
StageTruth is the known-correct system behavior at one verification stage.
*/
type StageTruth struct {
	MeasureSource types.SourceType
	MeasureMetric types.MetricType
	// MeasureBound is how AssertMeasure reads PeakSourceMetric: "positive"
	// (default, peak > 0), "present" (found and non-zero, signed ok), or
	// "zero" (found with peak == 0 — e.g. subject has no lead claim).
	MeasureBound string
	DecideAction types.Action
	MustNotEnter bool
	SizedEnter   bool
	// WalletBound is "preserve", "deploy", or empty when unclaimed.
	WalletBound string
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
				WalletBound:   "deploy",
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
				WalletBound:   "deploy",
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
				WalletBound:   "preserve",
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
				WalletBound:   "preserve",
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
				WalletBound:   "deploy",
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
				WalletBound:   "preserve",
			},
			Market: conditions.TapeThinBook,
		},
		{
			Kind: KindNoise, Name: "noise_no_herd", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MeasureSource: types.SourceSentiment,
				MeasureMetric: types.MetricChange,
				MeasureBound:  "present",
				MustNotEnter:  true,
				WalletBound:   "preserve",
			},
			Market: conditions.TapeNoise,
		},
		{
			Kind: KindPhantomQuote, Name: "phantom_quote_retreat", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MustNotEnter: true,
				WalletBound:  "preserve",
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
				WalletBound:   "preserve",
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
				WalletBound:   "preserve",
			},
			Market: conditions.TapeToxicChase,
		},
		{
			Kind: KindUnfundableLot, Name: "unfundable_minimum", Symbol: "MATIC/USD",
			Capital: 1, FeePct: 0.26,
			Truth: StageTruth{
				MustNotEnter: true,
				WalletBound:  "preserve",
			},
			Market: conditions.TapePump,
		},
		{
			Kind: KindReversalExit, Name: "reversal_after_entry", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				DecideAction: types.ActionEnter,
				SizedEnter:   true,
				WalletBound:  "deploy",
			},
			Market: conditions.TapePumpThenReversal,
		},
		{
			Kind: KindLagNoLead, Name: "lag_without_lead", Symbol: "MATIC/USD",
			Capital: 10_000, FeePct: 0.26,
			Truth: StageTruth{
				MeasureSource: types.SourceLeadLag,
				MeasureMetric: types.MetricStrength,
				MeasureBound:  "zero",
				MustNotEnter:  true,
				WalletBound:   "preserve",
			},
			Market: conditions.TapeLag,
		},
		{
			Kind: KindFundedSlice, Name: "funded_max_fraction", Symbol: "MATIC/USD",
			Capital: 5_000, FeePct: 0.26,
			Truth: StageTruth{
				DecideAction: types.ActionEnter,
				SizedEnter:   true,
				WalletBound:  "deploy",
			},
			Market: conditions.TapePump,
		},
	}
}

/*
Frames yields the catalog tape frames for Play.
*/
func Frames(entry Entry) iter.Seq[tests.Frame] {
	if entry.Market == nil {
		return func(yield func(tests.Frame) bool) {}
	}

	return entry.Market().Frames()
}
