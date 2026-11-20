package resonance

/*
Features is the predictive coder's input — one dim per real signal Map() label
that lands on envelope.Metrics(). No broker fiction: volume / day / spread /
book are desk gates painted by training, not manifold observations.
*/
var Features = []Feature{
	// Enter
	{"pumpdump", "compression"},
	{"correlation", "herd_score"},
	{"cvd", "starvation"},
	// Exit
	{"pumpdump", "exhaustion"},
	{"depthflow", "thin_score"},
	{"morphology", "ask_vacuum"},
	// Hold
	{"hawkes", "spectral_radius"},
	{"depthflow", "loaded_score"},
	{"liquidity", "scarcity_score"},
	// Board
	{"sentiment", "slump_score"},
	{"correlation", "peak_score"},
	// Contagion
	{"leadlag", "decoupled_score"},
	{"leadlag", "sync_score"},
	// Capacity
	{"toxicity", "vacuum_score"},
	{"liquidity", "depth_score"},
}

/*
storyFeatureCount, oracleFirst, and oraclePairs partition the forward curve —
story nets use the first nine features; oracle approve/veto pairs follow in
ForwardStoryNets without re-listing Features order at every call site.
*/
const (
	storyFeatureCount = 9
	oracleFirst       = 9
	oraclePairs       = 3
)

/*
ForwardStoryNets is the Model story contract — enter/hold/exit nets sum opposing
features from resonance.Features, then oracle approve/veto pairs adjust nets so
grid brightness becomes tradability without hardcoding observe rules in strategy.

Oracle pairs (board / contagion / capacity) are ordered (pressure, support):
approve sides lift enter; capacity support also lifts hold; pressure sides lift exit.
*/
func ForwardStoryNets(curve []float64) (enterNet, holdNet, exitNet float64, ok bool) {
	if len(curve) < storyFeatureCount {
		return 0, 0, 0, false
	}

	enterNet = curve[0] + curve[1] - curve[2]
	exitNet = curve[3] + curve[4] - curve[5]
	holdNet = curve[6] + curve[7] - curve[8]

	need := oracleFirst + 2*oraclePairs
	if len(curve) < need {
		return enterNet, holdNet, exitNet, true
	}

	for pair := range oraclePairs {
		pressure := curve[oracleFirst+2*pair]
		support := curve[oracleFirst+2*pair+1]
		enterNet += support
		exitNet += pressure

		if pair == oraclePairs-1 {
			holdNet += support
		}
	}

	return enterNet, holdNet, exitNet, true
}
