package engine

/*
This file is the decision tree the perspectives feed into. Each signal ships a
Category (its self-classified verdict). The trader collects the latest verdict
per signal for a symbol into a MarketStory, then walks the tree node by node:
at each node it reads the relevant signal's category and follows the matching
branch. A branch either CONTINUES to the next node or TERMINATES with an action.
If no branch at a node matches (the signal we need to hear from is silent), the
story cannot be navigated and the verdict is ActionNone -- we do nothing.

Entry and exit are the same continuous thesis, so Decide serves both: flat, it
walks the entry stages (1-4) looking for ALLOW ENTRY; holding, it walks the exit
stage (5) looking for whether the thesis still persists or has decayed. The tree
is the sole authority -- there is no separate friction gate, exploration mode,
or sizing veto layered on top of it.

The stages are exactly:

  Strategic Pluralism: PumpDump Vertical Ignition is a standalone entry,
  independent of market breadth (ride a solo launch even in a systemic slump).

  Stage 1 -- Systemic Filter
    Node 1 (Sentiment): Risk-On Surge -> continue; Systemic Slump -> No Action
    Node 2 (Correlation): Decoupled Alpha -> continue; Systemic Herd -> continue
  Stage 2 -- Origin Check
    Node 3 (Causal): Endogenous Alpha -> continue; Systemic Beta -> No Action
    Node 4 (LeadLag): Inefficient Lag -> continue; Anchor Stall -> Wait
  Stage 3 -- Quality Check
    Node 5 (Toxicity + DepthFlow): Hard Support AND Loaded Imbalance -> continue;
            Toxic Bluff OR Spoof Trap -> Deny
    Node 6 (CVD): Hidden Absorption -> continue; Aggressive Drive -> continue
  Stage 4 -- Timing
    Node 7 (Fluid): Laminar OR Inertial -> continue; Turbulent -> Wait
    Node 8 (Hawkes): Frenzy -> ALLOW ENTRY; Saturation -> Deny

  Stage 5 -- Exit Thesis (holding)
    Node 9: Mechanical Collapse OR Active Reversal -> Urgent Exit;
            Saturation -> Exit; Systemic Beta -> Exit;
            Thermal Exhaustion -> Soft Exit; Viscous Resistance -> Hold
*/

// Action is the decision a node terminates with.
type Action uint8

const (
	ActionNone  Action = iota // the story could not be navigated: do nothing
	ActionEnter               // ALLOW ENTRY
	ActionHold                // holding and the thesis still persists
	ActionExit                // holding and the thesis has decayed: close
	ActionWait                // a setup is forming but not yet actionable
	ActionDeny                // explicitly blocked
)

func (action Action) String() string {
	switch action {
	case ActionEnter:
		return "enter"
	case ActionHold:
		return "hold"
	case ActionExit:
		return "exit"
	case ActionWait:
		return "wait"
	case ActionDeny:
		return "deny"
	default:
		return "none"
	}
}

// Verdict is the outcome of navigating the tree. Direction is +1 for the long
// entries this engine takes; Urgency in [0,1] carries the exit intensity
// (Urgent vs Soft); Node names the node that terminated; Reason is its branch.
type Verdict struct {
	Action    Action
	Direction int
	Urgency   float64
	Node      string
	Reason    string
}

// CategoryReading is one signal's current verdict for a symbol.
type CategoryReading struct {
	Category   Category
	Confidence float64
	Direction  int
}

/*
MarketStory is the set of latest per-signal verdicts for one symbol -- the
discrete, named state the tree walks. The trader keeps the most recent reading
from each source.
*/
type MarketStory struct {
	readings map[string]CategoryReading
}

// NewMarketStory returns an empty story.
func NewMarketStory() MarketStory {
	return MarketStory{readings: make(map[string]CategoryReading)}
}

// Observe records a signal's current verdict, keyed by source. An empty
// category is ignored so a reading that did not commit to a labelled state
// cannot erase a prior verdict.
func (story *MarketStory) Observe(source string, reading CategoryReading) {
	if story.readings == nil {
		story.readings = make(map[string]CategoryReading)
	}

	if reading.Category == CategoryNone {
		return
	}

	story.readings[source] = reading
}

// is reports whether the owning signal's current verdict for this symbol is
// exactly category.
func (story MarketStory) is(category Category) bool {
	reading, ok := story.readings[CategorySignal(category)]
	return ok && reading.Category == category
}

// urgencyOf returns the confidence the owning signal attached to category, used
// to carry an exhaust pulse's urgency through to the exit.
func (story MarketStory) urgencyOf(category Category) float64 {
	reading, ok := story.readings[CategorySignal(category)]

	if !ok || reading.Category != category {
		return 0
	}

	return reading.Confidence
}

// Decide walks the tree. holding selects the entry stages (flat) or the exit
// stage (holding).
func Decide(story MarketStory, holding bool) Verdict {
	if holding {
		return decideExit(story)
	}

	return decideEntry(story)
}

/*
decideEntry walks Stages 1-4. A node advances only when one of its branches
matches; an unmatched node means the signal we need is silent, so the story
cannot be navigated and we do nothing.
*/
func decideEntry(story MarketStory) Verdict {
	// Strategic Pluralism: a vertical ignition is its own entry, independent of
	// the systemic filter -- ride a solo launch even during a slump.
	if story.is(CatVerticalIgnition) {
		return Verdict{Action: ActionEnter, Direction: 1, Node: "pump_ignition", Reason: "vertical ignition"}
	}

	// Stage 1, Node 1: Is there global conviction? (Sentiment)
	switch {
	case story.is(CatSystemicSlump):
		return Verdict{Action: ActionNone, Node: "stage1_node1", Reason: "systemic slump: wait for breadth"}
	case story.is(CatRiskOnSurge):
		// continue
	default:
		return Verdict{Action: ActionNone, Node: "stage1_node1", Reason: "no global conviction"}
	}

	// Stage 1, Node 2: Is the market moving as a single block? (Correlation)
	switch {
	case story.is(CatDecoupledAlpha), story.is(CatSystemicHerd):
		// continue (decoupled = high conviction, herd = proceed with caution)
	default:
		return Verdict{Action: ActionNone, Node: "stage1_node2", Reason: "correlation regime unknown"}
	}

	// Stage 2, Node 3: Is this move independent or a passenger? (Causal)
	switch {
	case story.is(CatSystemicBeta):
		return Verdict{Action: ActionNone, Node: "stage2_node3", Reason: "asset is a drifter (systemic beta)"}
	case story.is(CatEndogenousAlpha):
		// continue
	default:
		return Verdict{Action: ActionNone, Node: "stage2_node3", Reason: "no local driver"}
	}

	// Stage 2, Node 4: Is there a leadership lag? (LeadLag)
	switch {
	case story.is(CatAnchorStall):
		return Verdict{Action: ActionWait, Node: "stage2_node4", Reason: "anchor stall: move may be exhausted"}
	case story.is(CatInefficientLag):
		// continue (high-probability catch-up)
	default:
		return Verdict{Action: ActionNone, Node: "stage2_node4", Reason: "no leadership lag"}
	}

	// Stage 3, Node 5: Are the walls real? (Toxicity + DepthFlow)
	switch {
	case story.is(CatToxicBluff), story.is(CatSpoofTrap):
		return Verdict{Action: ActionDeny, Node: "stage3_node5", Reason: "manipulation detected"}
	case story.is(CatHardSupport) && story.is(CatLoadedImbalance):
		// continue (strong structural support)
	default:
		return Verdict{Action: ActionNone, Node: "stage3_node5", Reason: "no real structural support"}
	}

	// Stage 3, Node 6: Is there hidden resistance? (CVD)
	switch {
	case story.is(CatHiddenAbsorption), story.is(CatAggressiveDrive):
		// continue (iceberg exhausting / trend confirmed on tape)
	default:
		return Verdict{Action: ActionNone, Node: "stage3_node6", Reason: "no tape confirmation"}
	}

	// Stage 4, Node 7: Is the flow orderly? (Fluid)
	switch {
	case story.is(CatTurbulent):
		return Verdict{Action: ActionWait, Node: "stage4_node7", Reason: "turbulent chaos: mechanical breakdown"}
	case story.is(CatLaminar), story.is(CatInertial):
		// continue (mechanically healthy)
	default:
		return Verdict{Action: ActionNone, Node: "stage4_node7", Reason: "flow not mechanically healthy"}
	}

	// Stage 4, Node 8: Is the chain reaction ignited? (Hawkes)
	switch {
	case story.is(CatSaturation):
		return Verdict{Action: ActionDeny, Node: "stage4_node8", Reason: "dangerously overheated"}
	case story.is(CatFrenzy):
		return Verdict{Action: ActionEnter, Direction: 1, Node: "stage4_node8", Reason: "frenzy: momentum ignited"}
	default:
		return Verdict{Action: ActionNone, Node: "stage4_node8", Reason: "chain reaction not ignited"}
	}
}

/*
decideExit walks Stage 5 for an open position. Urgent exits fire on a collapsing
or reversing structure; a soft exit harvests a fading move; a viscous grind or
silence holds and lets the runway manage the trade.
*/
func decideExit(story MarketStory) Verdict {
	// Urgent: the structure under the position is breaking or has reversed.
	if story.is(CatMechanicalCollapse) {
		return Verdict{Action: ActionExit, Urgency: 1, Node: "stage5_node9", Reason: "mechanical collapse"}
	}

	if story.is(CatActiveReversal) {
		return Verdict{Action: ActionExit, Urgency: 1, Node: "stage5_node9", Reason: "active reversal"}
	}

	if story.is(CatSaturation) {
		return Verdict{Action: ActionExit, Urgency: 1, Node: "stage5_node9", Reason: "overheated: saturation"}
	}

	// The local driver has decayed to a passenger.
	if story.is(CatSystemicBeta) {
		return Verdict{Action: ActionExit, Urgency: 0.5, Node: "stage5_node9", Reason: "driver decayed to systemic beta"}
	}

	// Soft: momentum is fading -- harvest rather than scramble.
	if story.is(CatThermalExhaustion) {
		return Verdict{Action: ActionExit, Urgency: 0.5, Node: "stage5_node9", Reason: "thermal exhaustion"}
	}

	// Grinding against a wall, or quiet: hold and let the runway manage it.
	return Verdict{Action: ActionHold, Direction: 1, Node: "stage5_node9", Reason: "thesis intact"}
}
