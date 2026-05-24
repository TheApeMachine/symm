package fluid

/*
FieldPublisher receives incremental fluid telemetry as each symbol is sampled.
*/
type FieldPublisher interface {
	PublishFieldRow(row SymbolSnapshot)
	PublishFieldAggregate(sampledCount int, aggregate FieldAggregate)
	PublishFieldGrid(grid FluidGrid)
}
