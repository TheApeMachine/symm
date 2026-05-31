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
		document, err := LoadDocumentFile("../../config/perspectives.yaml")
		So(err, ShouldBeNil)

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
		builder := NewProfileBuilder()
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

func TestDefaultDocumentMovesFrictionIntoTree(t *testing.T) {
	Convey("Given the default pump playbook", t, func() {
		document, err := LoadDocumentFile("../../config/perspectives.yaml")
		So(err, ShouldBeNil)

		strategies, err := BuildStrategies(document)
		So(err, ShouldBeNil)

		var pump *strategy

		for _, candidate := range strategies {
			strategy := candidate.(*strategy)

			if strategy.Name() == PlaybookPump {
				pump = strategy
			}
		}

		So(pump, ShouldNotBeNil)

		threshold, path, found := entryCostGateAndPath(pumpSpec(document))
		So(found, ShouldBeTrue)

		Convey("It should return a tree deny when the cost ratio cannot clear", func() {
			action := pump.DecideWithContext(
				measurementsForPath(path),
				nil,
				DecisionContext{Metrics: map[string]float64{MetricScoreCostRatio: threshold * 0.5}},
			)

			So(action, ShouldNotBeNil)
			So(*action, ShouldEqual, ActionDeny)
		})

		Convey("It should authorize a reachable entry path once cost clears", func() {
			action := pump.DecideWithContext(
				measurementsForPath(path),
				nil,
				DecisionContext{Metrics: map[string]float64{
					MetricScoreCostRatio: threshold + 0.1,
					MetricInPlay:         1,
				}},
			)

			So(action, ShouldNotBeNil)
			So(*action, ShouldEqual, ActionEnter)
		})
	})
}

func pumpSpec(document Document) PlaybookSpec {
	for _, playbook := range document.Playbooks {
		if cleanName(playbook.Name) == string(PlaybookPump) {
			return playbook
		}
	}

	return PlaybookSpec{}
}

func entryCostGateAndPath(playbook PlaybookSpec) (float64, []BranchSpec, bool) {
	for _, branch := range playbook.Entry {
		if branch.Metric != MetricScoreCostRatio || branch.Condition != ">=" {
			continue
		}

		path, found := findEnterPath(branch.Branches, nil)

		if found && branch.Value != nil {
			return *branch.Value, path, true
		}
	}

	return 0, nil, false
}

func findEnterPath(branches []BranchSpec, path []BranchSpec) ([]BranchSpec, bool) {
	for _, branch := range branches {
		nextPath := append(append([]BranchSpec(nil), path...), branch)

		if branch.Action == ActionLabel(ActionEnter) {
			return nextPath, true
		}

		if foundPath, found := findEnterPath(branch.Branches, nextPath); found {
			return foundPath, true
		}
	}

	return nil, false
}

func measurementsForPath(path []BranchSpec) []Measurement {
	byCategory := make(map[CategoryType]float64)

	for _, branch := range path {
		if branch.Category == "" {
			continue
		}

		category, err := parseCategory(branch.Category)

		if err != nil {
			continue
		}

		byCategory[category] = satisfyingSNR(branch)
	}

	measurements := make([]Measurement, 0, len(byCategory))

	for category, snr := range byCategory {
		measurements = append(measurements, measurement(category, snr))
	}

	return measurements
}

func satisfyingSNR(branch BranchSpec) float64 {
	if branch.Value == nil {
		return 2
	}

	switch branch.Condition {
	case "<", "<=":
		return *branch.Value * 0.5
	default:
		return *branch.Value + 0.1
	}
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

func BenchmarkBuildStrategies(b *testing.B) {
	document, err := LoadDocumentFile("../../config/perspectives.yaml")

	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		if _, err := BuildStrategies(document); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMutateDocument(b *testing.B) {
	document, err := LoadDocumentFile("../../config/perspectives.yaml")

	if err != nil {
		b.Fatal(err)
	}

	random := rand.New(rand.NewSource(7))

	for b.Loop() {
		mutated := MutateDocument(document, random)

		if _, err := BuildStrategies(mutated); err != nil {
			b.Fatal(err)
		}
	}
}
