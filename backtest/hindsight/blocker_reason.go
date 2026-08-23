package hindsight

import "strings"

func reasonCategory(context SignalContext) string {
	reason := strings.ToLower(strings.Join(
		[]string{context.Reason, context.Cause},
		" ",
	))

	for _, token := range []string{
		"global regulator", "regulator is observing", "regulator is adapting",
	} {
		if strings.Contains(reason, token) {
			return DiagnosisRegulator
		}
	}

	if strings.Contains(reason, "admission") {
		return DiagnosisAdmission
	}

	for _, token := range []string{
		"allocation", "capacity", "capital", "cash", "slot", "reserve", "displace",
	} {
		if strings.Contains(reason, token) {
			return DiagnosisAllocation
		}
	}

	for _, token := range []string{
		"execution", "executable", "liquidity", "spread", "impact", "fee",
		"quantity", "depth", "order", "venue", "risk plan", "stoploss",
	} {
		if strings.Contains(reason, token) {
			return DiagnosisExecution
		}
	}

	return ""
}
