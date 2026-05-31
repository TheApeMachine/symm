package perspectives

const (
	MetricThesisScore      = "thesis_score"
	MetricSpreadBPS        = "spread_bps"
	MetricFeePct           = "fee_pct"
	MetricRoundTripCostBPS = "round_trip_cost_bps"
	MetricRequiredScore    = "required_score"
	MetricScoreCostRatio   = "score_cost_ratio"
	MetricInPlay           = "in_play"
)

/*
DecisionContext carries trader-side numeric state into perspective trees so
economic gates can live in the playbook instead of in the desk.
*/
type DecisionContext struct {
	Metrics map[string]float64
}

func (context DecisionContext) Metric(name string) (float64, bool) {
	if context.Metrics == nil {
		return 0, false
	}

	value, ok := context.Metrics[name]

	return value, ok
}
