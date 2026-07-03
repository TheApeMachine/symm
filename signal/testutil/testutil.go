package testutil

import (
	"fmt"
	"iter"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

/*
ObservePeers records a ticker datapoint's rows into the cross-section exactly as
the trader's Signal.Observe does in production (trader/signal.go) before any
signal Measure runs each tick. crossSection-backed signals only READ peer
snapshots in Measure (crossSection.Volumes / peer stats); nothing in Measure
populates them. A test that feeds peers through Measure alone leaves the peer
caches empty, so the signal sees no cross-section and silently falls back to a
degenerate self-reference (median = own value, relative = 1). Call this for every
row a tick presents — including the subject — to restore the real data path.
*/
func ObservePeers(crossSection *market.CrossSection, datapoint *datura.Artifact) {
	if crossSection == nil || datapoint == nil {
		return
	}

	_ = crossSection.Observe(map[string][]*datura.Artifact{
		"ticker": {datapoint},
	})
}

/*
FirstMeasured returns the first artifact from a Measure iterator, if any.
*/
func FirstMeasured(measurements iter.Seq[*datura.Artifact]) *datura.Artifact {
	for measured := range measurements {
		return measured
	}

	return nil
}

/*
HasConfidence reports whether artifact carries a scored classifier output.
*/
func HasConfidence(measurement *datura.Artifact) bool {
	if measurement == nil {
		return false
	}

	return datura.Peek[float64](measurement, "output", "confidence") > 0
}

/*
StoreMeasurement inserts a measurement artifact into the tree the way the trader
does after Measure returns, so signal tests can warm history from prior frames.
It returns the updated tree head because dmt.Tree is persistent/immutable.
*/
func StoreMeasurement(tree *dmt.Tree, measurement *datura.Artifact) *dmt.Tree {
	if tree == nil || measurement == nil {
		return tree
	}

	updated, _, _ := tree.InsertArtifact(measurement.Prefix(), measurement)

	if updated == nil {
		return tree
	}

	return updated
}

/*
CategoryMass reads one category's normalised share from a measurement artifact.
*/
func CategoryMass(result *datura.Artifact, category logic.CategoryType) float64 {
	distribution := datura.Peek[map[string]any](result, "output", "distribution")
	mass, _ := distribution[fmt.Sprintf("%d", logic.CategoryIndex(category))].(float64)

	return mass
}

/*
DominantCategory returns the category with the highest published mass.
*/
func DominantCategory(
	result *datura.Artifact,
	categories []logic.CategoryType,
) logic.CategoryType {
	best := categories[0]
	bestMass := CategoryMass(result, best)

	for _, category := range categories[1:] {
		mass := CategoryMass(result, category)

		if mass > bestMass {
			bestMass = mass
			best = category
		}
	}

	return best
}

/*
DominantCategoryIndex returns the global classifier index of the dominant category.
*/
func DominantCategoryIndex(
	result *datura.Artifact,
	categories []logic.CategoryType,
) int {
	return logic.CategoryIndex(DominantCategory(result, categories))
}

/*
DistributionSum returns the total mass across the listed categories (should be ~1).
*/
func DistributionSum(result *datura.Artifact, categories []logic.CategoryType) float64 {
	total := 0.0

	for _, category := range categories {
		total += CategoryMass(result, category)
	}

	return total
}
