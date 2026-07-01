package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	balancefixtures "github.com/theapemachine/symm/tests/fixtures/balances"
)

func TestTreeEvaluateSequentialMeasurementStream(testingTB *testing.T) {
	Convey("Given a two-stage playbook branch and a balances fixture", testingTB, func() {
		tree := &Tree{}
		branches := []*Branch{
			{
				ConditionGroup: &ConditionGroup{
					Boolean: BooleanTypeAnd,
					Conditions: []Condition{
						{
							Type: ConditionIsTrue,
							Left: ConditionOperand{
								Type:     SubjectCategory,
								Source:   SourcePumpDump,
								Category: NewCategory(CategoryCoiledCompression),
							},
						},
					},
				},
				Branches: []*Branch{
					{
						ConditionGroup: &ConditionGroup{
							Boolean: BooleanTypeAnd,
							Conditions: []Condition{
								{
									Type: ConditionIsTrue,
									Left: ConditionOperand{
										Type:     SubjectCategory,
										Source:   SourcePumpDump,
										Category: NewCategory(CategoryVerticalIgnition),
									},
								},
							},
						},
						Action: &Action{
							Type:     ActionMarket,
							Side:     SideBuy,
							Quantity: 0.1,
						},
					},
				},
			},
		}

		var holdings *datura.Artifact
		for artifact := range balancefixtures.NewFixture(balancefixtures.SNAPSHOT, 1).Artifacts() {
			holdings = artifact
			break
		}

		first := testMeasurementArtifact(SourcePumpDump, "MATIC/USD", CategoryCoiledCompression, 0.71, 1)
		first.SetTimestamp(time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC).UnixNano())
		second := testMeasurementArtifact(SourcePumpDump, "MATIC/USD", CategoryVerticalIgnition, 0.83, 1)
		second.SetTimestamp(time.Date(2026, 6, 29, 8, 0, 1, 0, time.UTC).UnixNano())

		Convey("When the first stream tick is evaluated", func() {
			actions, err := tree.Evaluate("MATIC/USD", []*datura.Artifact{first}, holdings, branches)

			Convey("Then it should only arm the sequential parent", func() {
				So(err, ShouldBeNil)
				So(actions, ShouldBeEmpty)
			})
		})

		Convey("When the next stream tick confirms the child branch", func() {
			_, err := tree.Evaluate("MATIC/USD", []*datura.Artifact{first}, holdings, branches)
			So(err, ShouldBeNil)

			actions, err := tree.Evaluate("MATIC/USD", []*datura.Artifact{second}, holdings, branches)

			Convey("Then it should emit one buy candidate with measurement evidence", func() {
				So(err, ShouldBeNil)
				So(actions, ShouldHaveLength, 1)
				So(actions[0].Symbol, ShouldEqual, "MATIC/USD")
				So(actions[0].EntryConfidence, ShouldEqual, 0.83)
				So(actions[0].ReasonCategory, ShouldEqual, CategoryVerticalIgnition)
			})
		})
	})
}

func TestPlaybookDoesNotBorrowMeasurementFromOtherSymbol(testingTB *testing.T) {
	Convey("Given a BTC playbook evaluation with only ETH measurement evidence", testingTB, func() {
		tree := &Tree{}
		branches := singleCategoryBuyBranch(SourceFluid, CategoryLaminar)
		eth := testMeasurementArtifact(SourceFluid, "ETH/USD", CategoryLaminar, 0.8, 1)

		actions, err := tree.Evaluate("BTC/USD", []*datura.Artifact{eth}, nil, branches)

		Convey("Then it should fail closed instead of borrowing ETH evidence", func() {
			So(err, ShouldBeNil)
			So(actions, ShouldBeEmpty)
		})
	})
}

func TestPlaybookRejectsStaleMeasurement(testingTB *testing.T) {
	Convey("Given a target measurement older than the story max age", testingTB, func() {
		previous := viper.GetString("market.story.measurement_max_age")
		viper.Set("market.story.measurement_max_age", "30s")
		defer viper.Set("market.story.measurement_max_age", previous)

		tree := &Tree{}
		branches := singleCategoryBuyBranch(SourceFluid, CategoryLaminar)
		base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
		stale := testMeasurementArtifact(SourceFluid, "BTC/USD", CategoryLaminar, 0.8, 1)
		stale.SetTimestamp(base.UnixNano())
		freshAnchor := testMeasurementArtifact(SourceHawkes, "BTC/USD", CategoryFrenzy, 0.8, 1)
		freshAnchor.SetTimestamp(base.Add(time.Minute).UnixNano())

		actions, err := tree.Evaluate(
			"BTC/USD",
			[]*datura.Artifact{stale, freshAnchor},
			nil,
			branches,
		)

		Convey("Then it should fail closed instead of using stale evidence", func() {
			So(err, ShouldBeNil)
			So(actions, ShouldBeEmpty)
		})
	})
}

func TestPlaybookEmitsGuardMissingDiagnostic(t *testing.T) {
	tree := &Tree{}
	branches := guardFalseBuyBranch(SourceToxicity, CategoryToxicBluff)
	measurement := testMeasurementArtifact(SourcePumpDump, "BTC/USD", CategoryVerticalIgnition, 0.8, 1)

	trace := tree.EvaluateTrace("BTC/USD", []*datura.Artifact{measurement}, nil, branches)

	if len(trace.Conditions) != 2 {
		t.Fatalf("conditions = %d, want 2", len(trace.Conditions))
	}
	guard := traceConditionForSource(trace.Conditions, SourceToxicity)
	if guard == nil {
		t.Fatalf("toxicity guard trace missing: %#v", trace.Conditions)
	}
	if guard.Result {
		t.Fatal("missing guard source passed")
	}
	if guard.Reason != TraceReasonMissingSource {
		t.Fatalf("reason = %s, want %s", guard.Reason, TraceReasonMissingSource)
	}
}

func TestNoCandidateWhenRequiredGuardMissing(t *testing.T) {
	tree := &Tree{}
	branches := guardFalseBuyBranch(SourceToxicity, CategoryToxicBluff)
	measurement := testMeasurementArtifact(SourcePumpDump, "BTC/USD", CategoryVerticalIgnition, 0.8, 1)

	actions, err := tree.Evaluate("BTC/USD", []*datura.Artifact{measurement}, nil, branches)

	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("actions = %d, want 0", len(actions))
	}
}

func TestGuardNeutralMeasurementAllowsBranchEvaluation(t *testing.T) {
	tree := &Tree{}
	branches := guardFalseBuyBranch(SourceToxicity, CategoryToxicBluff)
	entry := testMeasurementArtifact(SourcePumpDump, "BTC/USD", CategoryVerticalIgnition, 0.8, 1)
	guard := testMeasurementArtifact(SourceToxicity, "BTC/USD", CategoryHardSupport, 0.8, 1)

	actions, err := tree.Evaluate("BTC/USD", []*datura.Artifact{entry, guard}, nil, branches)

	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(actions))
	}
}

func TestEvaluateTraceMarksStaleAndWrongSymbolEvidence(t *testing.T) {
	previous := viper.GetString("market.story.measurement_max_age")
	viper.Set("market.story.measurement_max_age", "30s")
	defer viper.Set("market.story.measurement_max_age", previous)

	tree := &Tree{}
	branches := singleCategoryBuyBranch(SourceFluid, CategoryLaminar)
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	stale := testMeasurementArtifact(SourceFluid, "BTC/USD", CategoryLaminar, 0.8, 1)
	stale.SetTimestamp(base.UnixNano())
	anchor := testMeasurementArtifact(SourceHawkes, "BTC/USD", CategoryFrenzy, 0.8, 1)
	anchor.SetTimestamp(base.Add(time.Minute).UnixNano())

	staleTrace := tree.EvaluateTrace("BTC/USD", []*datura.Artifact{stale, anchor}, nil, branches)
	if len(staleTrace.Conditions) != 1 {
		t.Fatalf("stale conditions = %d, want 1", len(staleTrace.Conditions))
	}
	if staleTrace.Conditions[0].Reason != TraceReasonStaleSource {
		t.Fatalf("stale reason = %s, want %s", staleTrace.Conditions[0].Reason, TraceReasonStaleSource)
	}

	wrongSymbol := testMeasurementArtifact(SourceFluid, "ETH/USD", CategoryLaminar, 0.8, 1)
	wrongTrace := tree.EvaluateTrace("BTC/USD", []*datura.Artifact{wrongSymbol}, nil, branches)
	if len(wrongTrace.Conditions) != 1 {
		t.Fatalf("wrong-symbol conditions = %d, want 1", len(wrongTrace.Conditions))
	}
	if wrongTrace.Conditions[0].Reason != TraceReasonWrongSymbol {
		t.Fatalf("wrong-symbol reason = %s, want %s", wrongTrace.Conditions[0].Reason, TraceReasonWrongSymbol)
	}
}

func TestPlaybookRequiresExplicitEvaluationSymbol(testingTB *testing.T) {
	Convey("Given a playbook evaluation without a target symbol", testingTB, func() {
		tree := &Tree{}
		branches := singleCategoryBuyBranch(SourceFluid, CategoryLaminar)
		btc := testMeasurementArtifact(SourceFluid, "BTC/USD", CategoryLaminar, 0.8, 1)

		actions, err := tree.Evaluate("", []*datura.Artifact{btc}, nil, branches)

		Convey("Then it should not infer the target from the first measurement", func() {
			So(err, ShouldBeNil)
			So(actions, ShouldBeEmpty)
		})
	})
}

func guardFalseBuyBranch(source SourceType, category CategoryType) []*Branch {
	return []*Branch{
		{
			ConditionGroup: &ConditionGroup{
				Boolean: BooleanTypeAnd,
				Conditions: []Condition{
					{
						Type: ConditionIsTrue,
						Left: ConditionOperand{
							Type:     SubjectCategory,
							Source:   SourcePumpDump,
							Category: NewCategory(CategoryVerticalIgnition),
						},
					},
					{
						Type: ConditionIsFalse,
						Left: ConditionOperand{
							Type:     SubjectCategory,
							Source:   source,
							Category: NewCategory(category),
						},
					},
				},
			},
			Action: &Action{
				Type:     ActionMarket,
				Side:     SideBuy,
				Quantity: 0.1,
			},
		},
	}
}

func traceConditionForSource(conditions []ConditionTrace, source SourceType) *ConditionTrace {
	for index := range conditions {
		if conditions[index].Source == source {
			return &conditions[index]
		}
	}

	return nil
}

func singleCategoryBuyBranch(source SourceType, category CategoryType) []*Branch {
	return []*Branch{
		{
			ConditionGroup: &ConditionGroup{
				Boolean: BooleanTypeAnd,
				Conditions: []Condition{
					{
						Type: ConditionIsTrue,
						Left: ConditionOperand{
							Type:     SubjectCategory,
							Source:   source,
							Category: NewCategory(category),
						},
					},
				},
			},
			Action: &Action{
				Type:     ActionMarket,
				Side:     SideBuy,
				Quantity: 0.1,
			},
		},
	}
}
