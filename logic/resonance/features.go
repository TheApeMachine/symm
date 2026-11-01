package resonance

/*
Features is the predictive coder's input vector — one dim per grid region
(SIGNALS.md / RegionCount=23). Indices match region keys:

  R00–R02 Enter  detector / support / opposition
  R03–R05 Exit   detector / support / opposition
  R06–R08 Hold   detector / support / opposition
  R09–R10 Volume Quote     veto / approve  (broker)
  R11–R12 Day Move         veto / approve  (broker)
  R13–R14 Spread           veto / approve  (broker)
  R15–R16 Book Integrity   veto / approve  (broker)
  R17–R18 Board Confidence veto / approve  (envelope)
  R19–R20 Contagion        veto / approve  (envelope)
  R21–R22 Capacity         veto / approve  (envelope)

Broker labels are resolved via Solver.price; envelope labels must match
equation/excitation Map() keys on envelope.Metrics().
*/
var Features = []Feature{
	// Enter R00–R02
	{"pumpdump", "compression", ""},
	{"correlation", "herd_score", ""},
	{"cvd", "starvation", ""},
	// Exit R03–R05
	{"pumpdump", "exhaustion", ""},
	{"depthflow", "thin_score", ""},
	{"morphology", "ask_vacuum", ""},
	// Hold R06–R08
	{"hawkes", "spectral_radius", ""},
	{"depthflow", "loaded_score", ""},
	{"liquidity", "scarcity_score", ""},
	// Volume Quote R09–R10
	{"broker", "volume_veto", ""},
	{"broker", "volume_approve", ""},
	// Day Move R11–R12
	{"broker", "day_veto", ""},
	{"broker", "day_approve", ""},
	// Spread R13–R14
	{"broker", "spread_veto", ""},
	{"broker", "spread_approve", ""},
	// Book Integrity R15–R16
	{"broker", "book_veto", ""},
	{"broker", "book_approve", ""},
	// Board Confidence R17–R18
	{"sentiment", "slump_score", ""},
	{"correlation", "peak_score", ""},
	// Contagion R19–R20
	{"leadlag", "decoupled_score", ""},
	{"leadlag", "sync_score", ""},
	// Capacity R21–R22
	{"toxicity", "vacuum_score", ""},
	{"liquidity", "depth_score", ""},
}

const (
	storyFeatureCount = 9
	oracleFirst       = 9
	oraclePairs       = 7
)

/*
ForwardStoryNets collapses the 23-dim task head into Enter/Exit/Hold nets
plus envelope/broker oracle approve lifts and veto risk.
*/
func ForwardStoryNets(curve []float64) (enterNet, holdNet, exitNet, risk float64, ok bool) {
	if len(curve) < storyFeatureCount {
		return 0, 0, 0, 0, false
	}

	enterNet = curve[0] + curve[1] - curve[2]
	exitNet = curve[3] + curve[4] - curve[5]
	holdNet = curve[6] + curve[7] - curve[8]

	if len(curve) < len(Features) {
		return enterNet, holdNet, exitNet, 0, true
	}

	risk = 0

	for pair := range oraclePairs {
		veto := curve[oracleFirst+2*pair]
		approve := curve[oracleFirst+2*pair+1]

		enterNet += approve

		// Board Confidence approve also supports hold.
		if pair == 4 {
			holdNet += approve
		}

		if veto > risk {
			risk = veto
		}
	}

	return enterNet, holdNet, exitNet, risk, true
}
