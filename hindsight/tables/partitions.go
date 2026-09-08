package tables

import "github.com/apache/iceberg-go"

/*
Every Hindsight table partitions on run first.

A replay, a UI inspection, and a training pass all read exactly one run, so an
identity partition on run lets the planner discard every other run's files
without opening them. Captures additionally bucket by day of arrival, because a
long run's captures are the one family large enough that a single partition
would defeat the pruning; the other families are small enough that a day bucket
would only fragment them.
*/
func partitionByRun(sourceID int) iceberg.PartitionSpec {
	return iceberg.NewPartitionSpec(
		iceberg.PartitionField{
			SourceIDs: []int{sourceID}, FieldID: 1000,
			Name: "run", Transform: iceberg.IdentityTransform{},
		},
	)
}

// RunsPartitioning leaves the runs table unpartitioned: it holds one row per
// process start, and partitioning it would create a directory per run to hold
// a single row.
func RunsPartitioning() iceberg.PartitionSpec { return *iceberg.UnpartitionedSpec }

// CapturesPartitioning partitions by run, then by day of arrival.
func CapturesPartitioning() iceberg.PartitionSpec {
	return iceberg.NewPartitionSpec(
		iceberg.PartitionField{
			SourceIDs: []int{1}, FieldID: 1000,
			Name: "run", Transform: iceberg.IdentityTransform{},
		},
		iceberg.PartitionField{
			SourceIDs: []int{6}, FieldID: 1001,
			Name: "received_at_day", Transform: iceberg.DayTransform{},
		},
	)
}

// ManifestsPartitioning partitions manifests by run.
func ManifestsPartitioning() iceberg.PartitionSpec { return partitionByRun(1) }

// WitnessesPartitioning partitions witnesses by run.
func WitnessesPartitioning() iceberg.PartitionSpec { return partitionByRun(1) }

// LifecyclePartitioning partitions lifecycle events by run.
func LifecyclePartitioning() iceberg.PartitionSpec { return partitionByRun(1) }

// OutcomesPartitioning partitions graded decisions by run.
func OutcomesPartitioning() iceberg.PartitionSpec { return partitionByRun(1) }

// GapsPartitioning partitions gaps by run.
func GapsPartitioning() iceberg.PartitionSpec { return partitionByRun(1) }

// DecisionsPartitioning partitions decisions by run.
func DecisionsPartitioning() iceberg.PartitionSpec { return partitionByRun(1) }
