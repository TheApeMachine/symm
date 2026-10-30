package resonance

/*
Features is the predictive coder's input vector in grid region order R00–R14
(nomagique/store/grid_pins.go, symm/SIGNALS.md). forward_curve_i predicts the
next standardized value of Features[i].
*/
var Features = []Feature{
	// Enter R00–R02
	{"pumpdump", "coiled_compression", ""},
	{"correlation", "structural_alignment", "aggregate"},
	{"pumpdump", "starved_tape", ""},
	// Exit R03–R05
	{"pumpdump", "rejection_tick", ""},
	{"depthflow", "bid_ladder_release", ""},
	{"depthflow", "ask_ladder_release", ""},
	// Hold R06–R08
	{"hawkes", "supercritical_cascade", ""},
	{"depthflow", "bid_ladder_build", ""},
	{"pumpdump", "blowoff_dip", ""},
	// Board confidence R09–R11
	{"pumpdump", "board_coiled", "aggregate"},
	{"hawkes", "board_cascade", "aggregate"},
	{"depthflow", "board_bid_release", "aggregate"},
	// Board risk R12–R14
	{"liquidity", "market_friction", "aggregate"},
	{"correlation", "structure_uncoupling", "aggregate"},
	{"liquidity", "capacity_stress", "aggregate"},
}

const storyFeatureCount = 9

/*
ForwardStoryNets collapses the task head's next-step prediction into the same
net-force shape the grid and gate use: (detector + support) − opposition per
story band, plus board confidence and risk. Values are standardized (z-scores).
*/
func ForwardStoryNets(curve []float64) (enterNet, holdNet, exitNet, risk float64, ok bool) {
	if len(curve) < storyFeatureCount {
		return 0, 0, 0, 0, false
	}

	enterNet = curve[0] + curve[1] - curve[2]
	exitNet = curve[3] + curve[4] - curve[5]
	holdNet = curve[6] + curve[7] - curve[8]

	if len(curve) >= len(Features) {
		enterNet += curve[9]
		holdNet += curve[10]
		exitNet += curve[11]
		risk = maxFloat(curve[12], curve[13], curve[14])
	}

	return enterNet, holdNet, exitNet, risk, true
}

func maxFloat(values ...float64) float64 {
	m := values[0]
	for _, v := range values[1:] {
		if v > m {
			m = v
		}
	}
	return m
}
