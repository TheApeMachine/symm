package perspectives

/*
DashboardGaugeSources lists signal sources rendered on the gauge grid and signal
heatmap. Order is stable for layout documents and frontend row indexing.
*/
var DashboardGaugeSources = []SourceType{
	SourceHawkes,
	SourceFluid,
	SourcePumpDump,
	SourceCausal,
	SourceDepthFlow,
	SourceLeadLag,
	SourceLiquidity,
	SourceSentiment,
}

var dashboardGaugeLabels = map[SourceType]string{
	SourceHawkes:    "Hawkes",
	SourceFluid:     "Fluid",
	SourcePumpDump:  "Pump",
	SourceCausal:    "Causal",
	SourceDepthFlow: "Depth",
	SourceLeadLag:   "L-Lag",
	SourceLiquidity: "Basis",
	SourceSentiment: "Sent",
}

/*
DashboardGaugeNames returns canonical dashboard source names in layout order.
*/
func DashboardGaugeNames() []string {
	names := make([]string, 0, len(DashboardGaugeSources))

	for _, source := range DashboardGaugeSources {
		name := source.String()

		if name == "" {
			continue
		}

		names = append(names, name)
	}

	return names
}

/*
DashboardGaugeLabel returns the short UI label for a dashboard source name.
*/
func DashboardGaugeLabel(name string) string {
	for _, source := range DashboardGaugeSources {
		if source.String() == name {
			if label, ok := dashboardGaugeLabels[source]; ok {
				return label
			}

			return name
		}
	}

	return name
}

/*
DashboardGaugeLabelMap returns source-name to label entries for layout payloads.
*/
func DashboardGaugeLabelMap() map[string]string {
	labels := make(map[string]string, len(DashboardGaugeSources))

	for _, source := range DashboardGaugeSources {
		name := source.String()

		if name == "" {
			continue
		}

		labels[name] = DashboardGaugeLabel(name)
	}

	return labels
}
