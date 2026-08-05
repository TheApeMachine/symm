package audit

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func validMeasurement(
	source types.SourceType,
	at time.Time,
	raw float64,
) *types.Measurement {
	normalized := raw

	return &types.Measurement{
		Source:       source,
		Symbol:       "BTC/USD",
		At:           at,
		ObservedFrom: at.Add(-time.Second),
		Horizon:      time.Second,
		Validity: types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		},
		Metrics: map[string]types.MetricSample{
			"metric": {
				Raw: raw, Normalized: &normalized,
				Unit: types.UnitDimensionless,
			},
		},
	}
}

func pricedDecision(at time.Time) types.Decision {
	return types.Decision{
		ID:                uuid.NewString(),
		Action:            types.ActionNothing,
		Symbol:            "BTC/USD",
		At:                at,
		Cause:             "non_positive_utility",
		Reason:            "executable utility did not clear costs",
		ReferencePrice:    decimal.NewFromInt64(100),
		ExpectedReturn:    decimal.NewFromFloat64(0.20),
		ExpectedFees:      decimal.NewFromFloat64(0.10),
		ExpectedSpread:    decimal.NewFromFloat64(0.08),
		ExpectedImpact:    decimal.NewFromFloat64(0.04),
		ForecastSource:    string(types.SourceResonance),
		ForecastEpoch:     12,
		ValidThroughEpoch: 12,
		Confidence:        0.8,
		Uncertainty:       0.1,
		Trace: &types.DecisionTrace{
			MCTS: types.DecisionMCTSTrace{HorizonSteps: 4},
		},
	}
}

func TestDecisionEvents(t *testing.T) {
	Convey("Given a completed decision cycle with retained signal history", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Tick = 12
		decisionAt := time.Unix(100, 0).UTC()
		thesis.Decisions = []types.Decision{pricedDecision(decisionAt)}
		thesis.AppendMeasurements(
			types.SourceCVD,
			validMeasurement(types.SourceCVD, decisionAt.Add(-2*time.Second), 0.1),
			validMeasurement(types.SourceCVD, decisionAt.Add(-time.Second), 0.2),
		)
		provisional := validMeasurement(
			types.SourceToxicity, decisionAt.Add(-time.Second), 0.7,
		)
		provisional.Validity.State = types.ValidityProvisional
		provisional.Validity.Reason = "awaiting subsequent touch"
		thesis.AppendMeasurements(types.SourceToxicity, provisional)
		malformed := validMeasurement(
			types.SourceCorrelation, decisionAt.Add(-time.Second), 0.5,
		)
		malformed.Metrics["metric"] = types.MetricSample{Raw: math.NaN()}
		thesis.AppendMeasurements(types.SourceCorrelation, malformed)

		events := DecisionEvents(thesis)

		Convey("It should retain decision, forecast, and only latest valid evidence", func() {
			contexts := 0
			forecasts := 0
			evidence := make([]SignalEvidence, 0)
			unavailable := 0

			for _, event := range events {
				switch typed := event.(type) {
				case DecisionContext:
					contexts++
				case ForecastIssued:
					forecasts++
				case SignalEvidence:
					evidence = append(evidence, typed)
				case EvidenceUnavailable:
					unavailable++
				}
			}

			So(contexts, ShouldEqual, 1)
			So(forecasts, ShouldEqual, 1)
			So(evidence, ShouldHaveLength, 1)
			So(evidence[0].Evidence.Source, ShouldEqual, types.SourceCVD)
			So(evidence[0].Evidence.At, ShouldResemble, decisionAt.Add(-time.Second))
			So(evidence[0].Evidence.Metrics["metric"].Raw, ShouldEqual, 0.2)
			So(unavailable, ShouldEqual, 0)
		})
	})

	Convey("Given a rejection caused by unavailable allocation evidence", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Tick = 9
		decisionAt := time.Unix(200, 0).UTC()
		decision := pricedDecision(decisionAt)
		decision.Cause = "allocation_evidence_unavailable"
		decision.Reason = "valid liquidity and toxicity evidence required"
		thesis.Decisions = []types.Decision{decision}
		liquidity := validMeasurement(
			types.SourceLiquidity, decisionAt.Add(-time.Second), 0,
		)
		liquidity.Validity.State = types.ValidityInvalid
		liquidity.Validity.Reason = "normalization scale unavailable"
		thesis.AppendMeasurements(types.SourceLiquidity, liquidity)

		events := DecisionEvents(thesis)

		Convey("It should keep reasons but omit invalid metric payloads", func() {
			unavailable := make(map[types.SourceType]EvidenceUnavailable)

			for _, event := range events {
				if typed, ok := event.(EvidenceUnavailable); ok {
					unavailable[typed.Source] = typed
				}
			}

			So(unavailable, ShouldHaveLength, 2)
			So(unavailable[types.SourceLiquidity].Reason, ShouldEqual, "normalization scale unavailable")
			So(unavailable[types.SourceLiquidity].State, ShouldEqual, types.ValidityInvalid)
			So(unavailable[types.SourceToxicity].Reason, ShouldEqual, decision.Reason)
		})
	})

	Convey("Given a continuation deferral with no forecast timestamp", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Tick = 21
		thesis.At = time.Unix(250, 0).UTC()
		thesis.Decisions = []types.Decision{{
			ID:     uuid.NewString(),
			Action: types.ActionHold,
			Symbol: "BTC/USD",
			Cause:  "continuation",
			Reason: "awaiting eligible forecast for continuation scoring",
		}}

		events := DecisionEvents(thesis)

		Convey("It should use the evaluation time and retain the material absence", func() {
			contexts := make([]DecisionContext, 0)
			unavailable := make([]EvidenceUnavailable, 0)

			for _, event := range events {
				switch typed := event.(type) {
				case DecisionContext:
					contexts = append(contexts, typed)
				case EvidenceUnavailable:
					unavailable = append(unavailable, typed)
				}
			}

			So(contexts, ShouldHaveLength, 1)
			So(contexts[0].At, ShouldResemble, thesis.At)
			So(unavailable, ShouldHaveLength, 1)
			So(unavailable[0].At, ShouldResemble, thesis.At)
			So(unavailable[0].Source, ShouldEqual, types.SourceResonance)
		})
	})

	Convey("Given a continuation forecast without an MCTS trace", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Tick = 30
		decisionAt := time.Unix(275, 0).UTC()
		decision := pricedDecision(decisionAt)
		decision.Trace = nil
		decision.ForecastEpoch = 34
		decision.ValidThroughEpoch = 34
		thesis.Decisions = []types.Decision{decision}

		events := DecisionEvents(thesis)

		Convey("It should retain the horizon carried by the validity epoch", func() {
			forecasts := make([]ForecastIssued, 0)

			for _, event := range events {
				if forecast, ok := event.(ForecastIssued); ok {
					forecasts = append(forecasts, forecast)
				}
			}

			So(forecasts, ShouldHaveLength, 1)
			So(forecasts[0].HorizonSteps, ShouldEqual, 4)
		})
	})
}

func TestRecordDecisionCycle(t *testing.T) {
	Convey("Given a completed cycle projected into the audit file", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		recorder, err := NewRecorder(path)
		So(err, ShouldBeNil)
		thesis := types.NewThesis(nil)
		thesis.Tick = 3
		decisionAt := time.Unix(300, 0).UTC()
		thesis.Decisions = []types.Decision{pricedDecision(decisionAt)}
		thesis.AppendMeasurements(
			types.SourceCVD,
			validMeasurement(types.SourceCVD, decisionAt.Add(-time.Second), 0.3),
		)

		So(RecordDecisionCycle(recorder, thesis), ShouldBeNil)
		So(recorder.Close(), ShouldBeNil)

		Convey("It should contain only typed curated categories", func() {
			file, openErr := os.Open(path)
			So(openErr, ShouldBeNil)
			defer file.Close()
			scanner := bufio.NewScanner(file)
			categories := make([]string, 0)

			for scanner.Scan() {
				var decoded map[string]any
				So(json.Unmarshal(scanner.Bytes(), &decoded), ShouldBeNil)
				categories = append(categories, decoded["type"].(string))
				So(decoded["channel"], ShouldEqual, "analysis")
			}

			So(scanner.Err(), ShouldBeNil)

			So(categories, ShouldResemble, []string{
				string(CategoryDecisionContext),
				string(CategoryForecastIssued),
				string(CategorySignalEvidence),
			})
		})
	})
}

func BenchmarkDecisionEvents(b *testing.B) {
	thesis := types.NewThesis(nil)
	thesis.Tick = 100
	decisionAt := time.Unix(1_000, 0).UTC()
	thesis.Decisions = []types.Decision{pricedDecision(decisionAt)}

	for index := range 100 {
		at := decisionAt.Add(-time.Duration(100-index) * time.Second)
		thesis.AppendMeasurements(
			types.SourceCVD,
			validMeasurement(types.SourceCVD, at, float64(index)/100),
		)
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = DecisionEvents(thesis)
	}
}
