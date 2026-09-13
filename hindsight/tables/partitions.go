package tables

import "github.com/apache/iceberg-go"

/*
Every canonical table partitions on epoch first.

A replay, a UI inspection, and a training pass all read exactly one epoch/run,
so an identity partition on epoch lets the planner discard every other epoch's
files without opening them.
*/
func partitionByEpoch(sourceID int) iceberg.PartitionSpec {
	return iceberg.NewPartitionSpec(
		iceberg.PartitionField{
			SourceIDs: []int{sourceID}, FieldID: 1000,
			Name: "epoch", Transform: iceberg.IdentityTransform{},
		},
	)
}

func SpotLevel3Partitioning() iceberg.PartitionSpec    { return partitionByEpoch(1) }
func SpotTickerPartitioning() iceberg.PartitionSpec    { return partitionByEpoch(1) }
func SpotTradePartitioning() iceberg.PartitionSpec     { return partitionByEpoch(1) }
func FuturesTickerPartitioning() iceberg.PartitionSpec { return partitionByEpoch(1) }
func FuturesTradePartitioning() iceberg.PartitionSpec  { return partitionByEpoch(1) }
func ExecutionsPartitioning() iceberg.PartitionSpec   { return partitionByEpoch(1) }
func MeasurementsPartitioning() iceberg.PartitionSpec  { return partitionByEpoch(1) }
func ModelsPartitioning() iceberg.PartitionSpec        { return partitionByEpoch(1) }
func GridsPartitioning() iceberg.PartitionSpec         { return partitionByEpoch(1) }
func PositionsPartitioning() iceberg.PartitionSpec     { return partitionByEpoch(1) }
func DecisionsPartitioning() iceberg.PartitionSpec     { return partitionByEpoch(1) }
func OutcomesPartitioning() iceberg.PartitionSpec      { return partitionByEpoch(1) }
