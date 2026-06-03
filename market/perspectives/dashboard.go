package perspectives

const GaugeFullSigma = 4.0

/*
DashboardGaugeNames returns registered telemetry sources for layout payloads.
*/
func DashboardGaugeNames() []string {
	return DefaultTelemetryRegistry().Names()
}

/*
DashboardGaugeLabel returns the short UI label for a dashboard source name.
*/
func DashboardGaugeLabel(name string) string {
	return DefaultTelemetryRegistry().Label(name)
}

/*
DashboardGaugeLabelMap returns source-name to label entries for layout payloads.
*/
func DashboardGaugeLabelMap() map[string]string {
	return DefaultTelemetryRegistry().LabelMap()
}
