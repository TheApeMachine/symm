package strategy

import (
	"math"

	nmstore "github.com/theapemachine/nomagique/store"
)

/*
gateFrame is the environmental hurdle snapshot appended on training steps so
the dashboard can explain enter, hold, and exit decisions in real time.
*/
type gateFrame struct {
	Phase            string  `json:"phase"`
	Passed           bool    `json:"passed"`
	Virtual          bool    `json:"virtual"`
	Reason           string  `json:"reason,omitempty"`
	IgnitionHurdle   float64 `json:"ignitionHurdle"`
	RiskTolerance    float64 `json:"riskTolerance"`
	RejectionHurdle  float64 `json:"rejectionHurdle"`
	NetSpark         float64 `json:"netSpark"`
	NetHold          float64 `json:"netHold"`
	NetRejection     float64 `json:"netRejection"`
	Exhaustion       float64 `json:"exhaustion"`
	ExitForce        float64 `json:"exitForce"`
	Risk             float64 `json:"risk"`
	CapacityRisk     float64 `json:"capacityRisk"`
	TreeAction       string  `json:"treeAction,omitempty"`
	TreeVirtual      bool    `json:"treeVirtual,omitempty"`
	TreeImpulse      float64 `json:"treeImpulse,omitempty"`
	TreeResistance   float64 `json:"treeResistance,omitempty"`
}

func (gate *Gate) frameForEnter(observed []*nmstore.Region, dec GateDecision, tree Decision) gateFrame {
	b := regionBrightness(observed)
	netSpark := (b[0] + b[1]) - b[2]
	netHold := (b[6] + b[7]) - b[8]
	risk := max(b[12], b[13], b[14])

	return gateFrame{
		Phase:           "enter",
		Passed:          dec.Allowed && !dec.Virtual,
		Virtual:         dec.Virtual,
		Reason:          dec.Reason,
		IgnitionHurdle:  gate.ignition,
		RiskTolerance:   gate.risk,
		RejectionHurdle: gate.rejection,
		NetSpark:        netSpark,
		NetHold:         netHold,
		NetRejection:    (b[3] + b[4]) - b[5],
		Exhaustion:      math.Max(b[8], b[9]),
		ExitForce:       math.Max((b[3]+b[4])-b[5], math.Max(b[8], b[9])),
		Risk:            risk,
		CapacityRisk:    b[14],
		TreeAction:      tree.Action.String(),
		TreeVirtual:     tree.Virtual,
		TreeImpulse:     tree.Impulse,
		TreeResistance:  tree.Resistance,
	}
}

func (gate *Gate) frameForHold(observed []*nmstore.Region, tree Decision) gateFrame {
	b := regionBrightness(observed)
	netHold := (b[6] + b[7]) - b[8]
	risk := max(b[12], b[13], b[14])

	return gateFrame{
		Phase:           "hold",
		Passed:          true,
		IgnitionHurdle:  gate.ignition,
		RiskTolerance:   gate.risk,
		RejectionHurdle: gate.rejection,
		NetSpark:        (b[0] + b[1]) - b[2],
		NetHold:         netHold,
		NetRejection:    (b[3] + b[4]) - b[5],
		Exhaustion:      math.Max(b[8], b[9]),
		ExitForce:       math.Max((b[3]+b[4])-b[5], math.Max(b[8], b[9])),
		Risk:            risk,
		CapacityRisk:    b[14],
		TreeAction:      tree.Action.String(),
		TreeVirtual:     tree.Virtual,
		TreeImpulse:     tree.Impulse,
		TreeResistance:  tree.Resistance,
	}
}

func (gate *Gate) frameForExit(observed []*nmstore.Region, dec GateDecision, tree Decision) gateFrame {
	b := regionBrightness(observed)
	netRejection := (b[3] + b[4]) - b[5]
	exhaustion := math.Max(b[8], b[9])
	exitForce := math.Max(netRejection, exhaustion)
	risk := max(b[12], b[13], b[14])

	return gateFrame{
		Phase:           "exit",
		Passed:          dec.Allowed,
		Virtual:         dec.Virtual,
		Reason:          dec.Reason,
		IgnitionHurdle:  gate.ignition,
		RiskTolerance:   gate.risk,
		RejectionHurdle: gate.rejection,
		NetSpark:        (b[0] + b[1]) - b[2],
		NetHold:         (b[6] + b[7]) - b[8],
		NetRejection:    netRejection,
		Exhaustion:      exhaustion,
		ExitForce:       exitForce,
		Risk:            risk,
		CapacityRisk:    b[14],
		TreeAction:      tree.Action.String(),
		TreeVirtual:     tree.Virtual,
		TreeImpulse:     tree.Impulse,
		TreeResistance:  tree.Resistance,
	}
}
