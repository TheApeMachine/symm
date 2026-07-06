package logic

type decisionIntent struct {
	actionType string
	side       string
}

func newDecisionIntent(evidence decisionEvidence) decisionIntent {
	if evidence.momentum < 0 {
		return decisionIntent{
			actionType: "exit",
			side:       "sell",
		}
	}

	return decisionIntent{
		actionType: "entry",
		side:       "buy",
	}
}
