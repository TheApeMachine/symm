package engine

import "testing"

func reading(category Category) CategoryReading {
	return CategoryReading{Category: category, Confidence: 0.7, Direction: 1}
}

func storyOf(readings map[string]CategoryReading) MarketStory {
	story := NewMarketStory()

	for source, r := range readings {
		story.Observe(source, r)
	}

	return story
}

// fullEntry is a story that navigates every entry node through to ALLOW ENTRY.
// Individual cases mutate one source to exercise a single node's branch.
func fullEntry() map[string]CategoryReading {
	return map[string]CategoryReading{
		"sentiment":   reading(CatRiskOnSurge),      // Stage 1, Node 1
		"correlation": reading(CatSystemicHerd),     // Stage 1, Node 2
		"causal":      reading(CatEndogenousAlpha),  // Stage 2, Node 3
		"leadlag":     reading(CatInefficientLag),   // Stage 2, Node 4
		"bookflow":    reading(CatHardSupport),      // Stage 3, Node 5
		"depthflow":   reading(CatLoadedImbalance),  // Stage 3, Node 5
		"cvd":         reading(CatHiddenAbsorption), // Stage 3, Node 6
		"fluid":       reading(CatLaminar),          // Stage 4, Node 7
		"hawkes":      reading(CatFrenzy),           // Stage 4, Node 8
	}
}

func TestDecideEntryTree(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]CategoryReading)
		action Action
		node   string
	}{
		{
			name:   "full confluence allows entry",
			mutate: func(map[string]CategoryReading) {},
			action: ActionEnter,
			node:   "stage4_node8",
		},
		{
			name: "vertical ignition is a standalone entry",
			mutate: func(s map[string]CategoryReading) {
				for k := range s {
					delete(s, k)
				}
				s["pumpdump"] = reading(CatVerticalIgnition)
			},
			action: ActionEnter,
			node:   "pump_ignition",
		},
		{
			name: "vertical ignition enters even during systemic slump",
			mutate: func(s map[string]CategoryReading) {
				s["sentiment"] = reading(CatSystemicSlump)
				s["pumpdump"] = reading(CatVerticalIgnition)
			},
			action: ActionEnter,
			node:   "pump_ignition",
		},
		{
			name: "systemic slump halts at node 1",
			mutate: func(s map[string]CategoryReading) {
				s["sentiment"] = reading(CatSystemicSlump)
			},
			action: ActionNone,
			node:   "stage1_node1",
		},
		{
			name: "silent sentiment halts at node 1",
			mutate: func(s map[string]CategoryReading) {
				delete(s, "sentiment")
			},
			action: ActionNone,
			node:   "stage1_node1",
		},
		{
			name: "systemic beta halts at node 3",
			mutate: func(s map[string]CategoryReading) {
				s["causal"] = reading(CatSystemicBeta)
			},
			action: ActionNone,
			node:   "stage2_node3",
		},
		{
			name: "anchor stall waits at node 4",
			mutate: func(s map[string]CategoryReading) {
				s["leadlag"] = reading(CatAnchorStall)
			},
			action: ActionWait,
			node:   "stage2_node4",
		},
		{
			name: "spoof/toxic denies at node 5",
			mutate: func(s map[string]CategoryReading) {
				s["bookflow"] = reading(CatToxicBluff)
			},
			action: ActionDeny,
			node:   "stage3_node5",
		},
		{
			name: "missing structural support halts at node 5",
			mutate: func(s map[string]CategoryReading) {
				delete(s, "bookflow")
			},
			action: ActionNone,
			node:   "stage3_node5",
		},
		{
			name: "turbulent waits at node 7",
			mutate: func(s map[string]CategoryReading) {
				s["fluid"] = reading(CatTurbulent)
			},
			action: ActionWait,
			node:   "stage4_node7",
		},
		{
			name: "saturation denies at node 8",
			mutate: func(s map[string]CategoryReading) {
				s["hawkes"] = reading(CatSaturation)
			},
			action: ActionDeny,
			node:   "stage4_node8",
		},
		{
			name: "empty story does nothing",
			mutate: func(s map[string]CategoryReading) {
				for k := range s {
					delete(s, k)
				}
			},
			action: ActionNone,
			node:   "stage1_node1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := fullEntry()
			tc.mutate(s)
			verdict := Decide(storyOf(s), false)

			if verdict.Action != tc.action {
				t.Fatalf("action: got %v (%s), want %v", verdict.Action, verdict.Node, tc.action)
			}

			if verdict.Node != tc.node {
				t.Fatalf("node: got %q, want %q", verdict.Node, tc.node)
			}
		})
	}
}

func TestDecideExitTree(t *testing.T) {
	cases := []struct {
		name    string
		story   map[string]CategoryReading
		action  Action
		urgency float64
	}{
		{
			name:    "mechanical collapse is an urgent exit",
			story:   map[string]CategoryReading{"exhaust": reading(CatMechanicalCollapse)},
			action:  ActionExit,
			urgency: 1,
		},
		{
			name:    "active reversal is an urgent exit",
			story:   map[string]CategoryReading{"exhaust": reading(CatActiveReversal)},
			action:  ActionExit,
			urgency: 1,
		},
		{
			name:    "saturation is an urgent exit",
			story:   map[string]CategoryReading{"hawkes": reading(CatSaturation)},
			action:  ActionExit,
			urgency: 1,
		},
		{
			name:    "systemic beta exits",
			story:   map[string]CategoryReading{"causal": reading(CatSystemicBeta)},
			action:  ActionExit,
			urgency: 0.5,
		},
		{
			name:    "thermal exhaustion is a soft exit",
			story:   map[string]CategoryReading{"exhaust": reading(CatThermalExhaustion)},
			action:  ActionExit,
			urgency: 0.5,
		},
		{
			name:    "viscous grind holds",
			story:   map[string]CategoryReading{"fluid": reading(CatViscous)},
			action:  ActionHold,
			urgency: 0,
		},
		{
			name:    "quiet holds",
			story:   map[string]CategoryReading{},
			action:  ActionHold,
			urgency: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict := Decide(storyOf(tc.story), true)

			if verdict.Action != tc.action {
				t.Fatalf("action: got %v (%s), want %v", verdict.Action, verdict.Reason, tc.action)
			}

			if verdict.Action == ActionExit && verdict.Urgency != tc.urgency {
				t.Fatalf("urgency: got %v, want %v", verdict.Urgency, tc.urgency)
			}
		})
	}
}
