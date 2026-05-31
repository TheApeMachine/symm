package perspectives

import (
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDecodeDocumentBuildStrategies(t *testing.T) {
	Convey("Given a YAML playbook with a metric gate", t, func() {
		payload := []byte(`
version: 1
playbooks:
  - name: pump
    regime: trending
    policy: pump
    entry:
      - metric: score_cost_ratio
        condition: ">="
        value: 1
        branches:
          - category: spoof_trap
            action: enter
    exit:
      - category: active_reversal
        action: stop_loss
`)

		document, err := DecodeDocument(payload)
		So(err, ShouldBeNil)

		strategies, err := BuildStrategies(document)
		So(err, ShouldBeNil)
		So(len(strategies), ShouldEqual, 1)

		Convey("It should require the metric before the category can enter", func() {
			strategy := strategies[0].(*strategy)
			measurements := []Measurement{measurement(CategorySpoofTrap, 2)}

			blocked := strategy.DecideWithContext(
				measurements, nil, DecisionContext{Metrics: map[string]float64{
					MetricScoreCostRatio: 0.5,
				}},
			)
			So(blocked, ShouldBeNil)

			action := strategy.DecideWithContext(
				measurements, nil, DecisionContext{Metrics: map[string]float64{
					MetricScoreCostRatio: 1.2,
				}},
			)
			So(action, ShouldNotBeNil)
			So(*action, ShouldEqual, ActionEnter)
		})
	})
}

func TestDecodeDocumentRejectsUnknownPrimitive(t *testing.T) {
	Convey("Given a YAML playbook with an unknown category", t, func() {
		payload := []byte(`
version: 1
playbooks:
  - name: pump
    regime: trending
    policy: pump
    entry:
      - category: not_real
        action: enter
    exit:
      - category: active_reversal
        action: stop_loss
`)

		document, err := DecodeDocument(payload)
		So(err, ShouldBeNil)

		_, err = BuildStrategies(document)

		Convey("It should fail validation", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestMutateDocumentBuildsValidStrategies(t *testing.T) {
	Convey("Given the default perspective document", t, func() {
		document := BuiltinDocument()

		mutated := MutateDocument(document, rand.New(rand.NewSource(1)))
		strategies, err := BuildStrategies(mutated)

		Convey("It should produce a loadable candidate tree set", func() {
			So(err, ShouldBeNil)
			So(len(strategies), ShouldEqual, len(document.Playbooks))
		})
	})
}

func TestGenerateDocumentBuildsFromProfile(t *testing.T) {
	Convey("Given a replay-derived primitive profile", t, func() {
		builder := NewProfileBuilder(rand.New(rand.NewSource(2)))
		builder.Record(Measurement{
			Source:   SourceCVD,
			Category: CategoryAggressiveDrive,
			SNR:      1.4,
		})
		builder.Record(Measurement{
			Source:   SourcePumpDump,
			Category: CategoryVerticalIgnition,
			SNR:      2.1,
		})
		builder.Record(Measurement{
			Source:   SourceToxicity,
			Category: CategoryToxicBluff,
			SNR:      1.7,
		})

		profile := builder.Profile()
		document := GenerateDocument(profile, rand.New(rand.NewSource(2)))
		strategies, err := BuildStrategies(document)

		Convey("It should synthesize valid playbook trees without a predefined shape", func() {
			So(err, ShouldBeNil)
			So(len(strategies), ShouldEqual, len(searchPlaybookTemplates))
			So(document.Playbooks, ShouldNotBeEmpty)
			So(document.Playbooks[0].Entry, ShouldNotBeEmpty)
			So(document.Playbooks[0].Entry[0].Metric, ShouldEqual, MetricScoreCostRatio)
			So(denyEntryOverlaps(document), ShouldBeEmpty)
		})
	})
}

func TestMutateDocumentPrunesEntryShadowingDenies(t *testing.T) {
	Convey("Given a document whose deny branch shadows its only entry path", t, func() {
		value := 1.0
		document := Document{
			Version: 1,
			Playbooks: []PlaybookSpec{{
				Name:   string(PlaybookPump),
				Regime: "trending",
				Policy: "pump",
				Deny: []BranchSpec{{
					Category:  CategorySpoofTrap.String(),
					Condition: ">",
					Value:     &value,
					Action:    ActionLabel(ActionDeny),
				}},
				Entry: []BranchSpec{{
					Category:  CategorySpoofTrap.String(),
					Condition: ">",
					Value:     &value,
					Action:    ActionLabel(ActionEnter),
				}},
				Exit: []BranchSpec{{
					Category: CategoryActiveReversal.String(),
					Action:   ActionLabel(ActionStopLoss),
				}},
			}},
		}

		mutated := MutateDocument(document, rand.New(rand.NewSource(3)))

		Convey("It should remove the self-blocking deny primitive", func() {
			So(mutated.Playbooks[0].Deny, ShouldBeEmpty)
			So(denyEntryOverlaps(mutated), ShouldBeEmpty)
		})
	})
}

func TestDocumentSearchKeepsBuildableAfterObserve(t *testing.T) {
	Convey("Given a document search that received reward feedback", t, func() {
		profile := SearchProfile{Categories: []CategoryStat{{
			Name:    CategoryAggressiveDrive.String(),
			Source:  SourceCVD.String(),
			Count:   10,
			MeanSNR: 1.4,
			MaxSNR:  2.0,
			P50SNR:  1.2,
			P75SNR:  1.5,
			P90SNR:  1.8,
		}}}
		search, err := NewDocumentSearch(profile, rand.New(rand.NewSource(4)))
		So(err, ShouldBeNil)

		document := search.Next()
		search.Observe(document, 12)
		next := search.Next()
		_, err = BuildStrategies(next)

		Convey("It should keep producing buildable candidates after Observe", func() {
			So(err, ShouldBeNil)
		})
	})
}

func TestBuiltinPumpPerspectiveAuthorizesEntry(t *testing.T) {
	Convey("Given the builtin pump playbook", t, func() {
		pump := NewPumpPerspective()

		Convey("It should authorize entry on coiled compression without desk metrics", func() {
			action := pump.Decide(
				[]Measurement{measurement(CategoryCoiledCompression, 2)},
				nil,
			)

			So(action, ShouldNotBeNil)
			So(*action, ShouldEqual, ActionEnter)
		})
	})
}

func denyEntryOverlaps(document Document) []string {
	overlaps := make([]string, 0)

	for _, playbook := range document.Playbooks {
		entryCategories := branchCategorySet(playbook.Entry)

		for category := range branchCategorySet(playbook.Deny) {
			if _, found := entryCategories[category]; found {
				overlaps = append(overlaps, cleanName(playbook.Name)+":"+category)
			}
		}
	}

	return overlaps
}

func TestCloneDocument(t *testing.T) {
	Convey("Given a playbook document", t, func() {
		source := Document{
			Version: 1,
			Playbooks: []PlaybookSpec{{
				Name:   "pump",
				Regime: "trending",
				Entry: []BranchSpec{{
					Category: "spoof_trap",
					Action:   "enter",
				}},
			}},
		}

		clone := CloneDocument(source)

		Convey("It should not alias nested branches", func() {
			source.Playbooks[0].Entry[0].Category = "mutated"
			So(clone.Playbooks[0].Entry[0].Category, ShouldEqual, "spoof_trap")
		})
	})
}

func BenchmarkBuildStrategies(b *testing.B) {
	document := BuiltinDocument()

	for b.Loop() {
		if _, err := BuildStrategies(document); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMutateDocument(b *testing.B) {
	document := BuiltinDocument()

	random := rand.New(rand.NewSource(7))

	for b.Loop() {
		mutated := MutateDocument(document, random)

		if _, err := BuildStrategies(mutated); err != nil {
			b.Fatal(err)
		}
	}
}
