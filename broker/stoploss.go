package broker

import (
	"fmt"
	"math"
	"sort"

	"github.com/theapemachine/symm/system"
)

const (
	stepEps = 1e-9

	// sharpBreakPeakMargin: peak must clear arm by this much (fee-net pct points)
	// before trail_sharp_break may fire on the first floor break. Until then only
	// trail_confirm (2 ticks) can exit — avoids chopping the first profit peak
	// with underwater-scale breathing room.
	sharpBreakPeakMargin = 0.50

	// minArmedPullbackFloor: absolute floor for TypicalPullback after arming so
	// CRAWL/SMOOTH room is never sized purely by loss-side chop.
	minArmedPullbackFloor = 0.25

	// totalTranches is the cake grid (4×25%). earlyScaleOuts banks half at arm /
	// adaptive targets; the remaining half is the trail runner.
	totalTranches   = 4
	earlyScaleOuts  = 2
	trancheFraction = 1.0 / float64(totalTranches)
)

/*
exitProfitFloor is PolicyExitMinProfitPct — every exit must clear this floor.
Never break-even, never loss.
*/
func exitProfitFloor() float64 {
	if system.Cfg == nil || system.Cfg.Learning == nil {
		return stepEps
	}

	floor := system.Cfg.Learning.PolicyExitMinProfitPct
	if floor <= 0 {
		return stepEps
	}

	return floor
}

func (stoploss *Stoploss) trailFloorMin() float64 {
	// Never trail to break-even or a loss — every exit must clear the
	// policy profit floor, including after banked tranches.
	return exitProfitFloor()
}

type StoplossState int

const (
	StoplossHealthy StoplossState = iota
	StoplossArmed
	StoplossTriggered
)

type MarketRegime string

const (
	RegimeCrawl     MarketRegime = "CRAWL"
	RegimeSmooth    MarketRegime = "SMOOTH"
	RegimeExplosive MarketRegime = "EXPLOSIVE"
	RegimeChaos     MarketRegime = "CHAOS"
)

type Stoploss struct {
	State           StoplossState `json:"State"`
	Regime          MarketRegime  `json:"Regime"`
	InitialPnLPct   float64       `json:"InitialPnLPct"`
	CurrentPnLPct   float64       `json:"CurrentPnLPct"`
	PeakPnLPct      float64       `json:"PeakPnLPct"`
	ArmPnlPct       float64       `json:"ArmPnlPct"`
	TrailingFloor   float64       `json:"TrailingFloor"`
	ExhaustScore    float64       `json:"ExhaustScore"`
	Confirms        int           `json:"Confirms"`
	TypicalPush     float64       `json:"TypicalPush"`     // Learned n1
	TypicalPullback float64       `json:"TypicalPullback"` // Learned n2
	CurrentDrop     float64       `json:"CurrentDrop"`
	EmaDelta        float64       `json:"EmaDelta"`
	TriggerReason   string        `json:"TriggerReason,omitempty"`

	// Staged exit: earlyScaleOuts×25% at arm / adaptive targets; trail keeps 50%.
	// Mid targets step by max(TypicalPush, Arm, 1%) so explosive names stretch.
	// TranchesExited advances only on fill CommitPending — emit/trip claim via
	// PendingTranches so abortExit can roll back without skipping banks or
	// sticking StoplossTriggered in a BUSY↔READY loop.
	TranchesExited  int     `json:"TranchesExited"`            // 0..4 filled tranches
	PendingTranches int     `json:"PendingTranches,omitempty"` // in-flight cake slices (not filled)
	// Excitation is Hawkes spectral radius — paces profitable tranche banks only.
	Excitation float64 `json:"Excitation,omitempty"`
	// FlowOFI is Cont event order-flow imbalance — negative = bid withdrawal.
	FlowOFI float64 `json:"FlowOFI,omitempty"`
	PendingExitPct  float64 `json:"PendingExitPct,omitempty"`  // Fraction of BasisQty to sell this tick
	NextTargetPct   float64 `json:"NextTargetPct"`             // PnL% line for the next mid tranche
	BasisQty        float64 `json:"BasisQty,omitempty"`        // Qty when the stop was first armed for sizing

	// Rolling active-tick memory (excludes 0-change ticks)
	Deltas    []float64 `json:"Deltas,omitempty"`
	MaxWindow int       `json:"MaxWindow"`

	// Swing-leg memory (n1 pushes & n2 recovered pullbacks)
	Pushes       []float64 `json:"Pushes,omitempty"`
	Pullbacks    []float64 `json:"Pullbacks,omitempty"`
	MaxSwings    int       `json:"MaxSwings"`
	RallyStart   float64   `json:"RallyStart"`
	PullbackLow  float64   `json:"PullbackLow"`
	PushRecorded bool      `json:"PushRecorded"`

	// Short-term momentum & stall tracking
	EmaInit   bool `json:"EmaInit"`
	FlatTicks int  `json:"FlatTicks"`

	// Explosive burst state
	ExplosiveActive bool    `json:"ExplosiveActive"`
	ExplosivePushes int     `json:"ExplosivePushes"`
	ExplosiveTTL    int     `json:"ExplosiveTTL"`
	PeakPushDelta   float64 `json:"PeakPushDelta"`
}

func NewStoploss(pnlPct float64) *Stoploss {
	stoploss := &Stoploss{
		State:           StoplossHealthy,
		Regime:          RegimeSmooth,
		InitialPnLPct:   pnlPct,
		CurrentPnLPct:   pnlPct,
		PeakPnLPct:      pnlPct,
		ArmPnlPct:       system.Cfg.Risk.StoplossArmPnLPct,
		TypicalPullback: 0.02,
		Deltas:          make([]float64, 0, 20),
		MaxWindow:       20,
		Pushes:          make([]float64, 0, 12),
		Pullbacks:       make([]float64, 0, 12),
		MaxSwings:       12,
		RallyStart:      pnlPct,
		PullbackLow:     pnlPct,
	}
	return stoploss
}

func (stoploss *Stoploss) Update(pnlPct float64, friction float64) {
	// Clear stale exit signal only when nothing is in-flight. Pending claims
	// survive until CommitPending (fill) or AbortPending (veto/cancel).
	if stoploss.PendingTranches == 0 {
		stoploss.PendingExitPct = 0
	}

	if stoploss.State == StoplossTriggered {
		return
	}
	if friction < 0 {
		friction = 0
	}

	prevRegime := stoploss.Regime

	delta := pnlPct - stoploss.CurrentPnLPct
	stoploss.CurrentPnLPct = pnlPct
	isMove := math.Abs(delta) > stepEps

	// 1. Measure baseline step size BEFORE recording current tick
	priorAvgStep, _, _, _, _ := stoploss.analyzeMemory()

	// 2. Update active-tick memory & short-term momentum (EMA)
	if isMove {
		stoploss.FlatTicks = 0
		stoploss.Deltas = append(stoploss.Deltas, delta)
		if len(stoploss.Deltas) > stoploss.MaxWindow {
			stoploss.Deltas = stoploss.Deltas[1:]
		}
		if !stoploss.EmaInit {
			stoploss.EmaDelta = delta
			stoploss.EmaInit = true
		} else {
			stoploss.EmaDelta = (0.40 * delta) + (0.60 * stoploss.EmaDelta)
		}
	} else {
		stoploss.FlatTicks++
		if stoploss.EmaInit {
			// Momentum cools off gently while price stalls
			stoploss.EmaDelta *= 0.70
		}
	}

	// 3. Track Peaks and Swing-Leg Memory (n1 pushes vs recovered n2 pullbacks)
	if pnlPct > stoploss.PeakPnLPct+stepEps {
		if stoploss.CurrentDrop > stepEps {
			// Pullback survived and made a new high -> record into n2 memory
			stoploss.recordPullback(stoploss.CurrentDrop)
			stoploss.RallyStart = stoploss.PullbackLow
		}
		stoploss.PeakPnLPct = pnlPct
		stoploss.PullbackLow = pnlPct
		stoploss.CurrentDrop = 0
		stoploss.PushRecorded = false
	} else {
		drop := stoploss.PeakPnLPct - pnlPct
		if drop > stoploss.CurrentDrop {
			stoploss.CurrentDrop = drop
		}
		if pnlPct < stoploss.PullbackLow {
			stoploss.PullbackLow = pnlPct
		}
		// First red tick off the peak ends this upward leg -> record n1
		if !stoploss.PushRecorded && delta < -stepEps {
			push := stoploss.PeakPnLPct - stoploss.RallyStart
			if push > stepEps {
				stoploss.recordPush(push)
			}
			stoploss.PushRecorded = true
		}
	}
	stoploss.refreshSwingStats()

	// 4. Analyze updated memory & classify Market Regime
	avgStep, avgDownStep, _, chaosRatio, flipRate := stoploss.analyzeMemory()

	isNewExplosive, ignitionPush := stoploss.detectExplosiveIgnition(delta, priorAvgStep, friction)
	if isNewExplosive {
		stoploss.ExplosiveActive = true
		stoploss.ExplosiveTTL = 4
		stoploss.ExplosivePushes++
		if ignitionPush > stoploss.PeakPushDelta {
			stoploss.PeakPushDelta = ignitionPush
		}
		stoploss.Regime = RegimeExplosive
	} else if stoploss.ExplosiveActive {
		stoploss.ExplosiveTTL--
		if delta > stepEps && delta >= stoploss.PeakPushDelta*0.60 {
			stoploss.ExplosivePushes++
			stoploss.ExplosiveTTL = 4
		}
		if stoploss.ExplosiveTTL <= 0 {
			stoploss.ExplosiveActive = false
			stoploss.ExplosivePushes = 0
			stoploss.PeakPushDelta = 0
		} else {
			stoploss.Regime = RegimeExplosive
		}
	}

	if !stoploss.ExplosiveActive {
		chaosStepFloor := math.Max(0.035, friction*0.35)
		if avgStep >= chaosStepFloor && (chaosRatio >= 3.5 || (len(stoploss.Deltas) >= 5 && flipRate >= 0.65)) {
			stoploss.Regime = RegimeChaos
		} else if avgStep < 0.028 {
			stoploss.Regime = RegimeCrawl
		} else {
			stoploss.Regime = RegimeSmooth
		}
	}

	if stoploss.Regime != prevRegime {
		stoploss.Confirms = 0
	}

	// 5. Arming & Disarming
	// Once a tranche has been banked, keep StoplossArmed active even if PnL dips
	// below ArmPnlPct so TrailingFloor continues protecting the runner. An
	// unfilled arm claim is aborted with the disarm — same as abortExit.
	if stoploss.State == StoplossArmed &&
		stoploss.TranchesExited == 0 &&
		stoploss.CurrentPnLPct < stoploss.ArmPnlPct-stepEps {
		stoploss.AbortPending()
		stoploss.Disarm()
	} else if stoploss.State == StoplossHealthy &&
		stoploss.CurrentPnLPct >= stoploss.ArmPnlPct {
		stoploss.onArm()
	}

	// 6. RULE 1: never exit below the policy profit floor (not mere break-even).
	// Floor/exit room must not inflate on the crash tick that just entered
	// Deltas — use priorAvgStep so a -2.45% drop cannot widen Explosive room
	// and drop TrailingFloor to 0 before explosive_floor_break can fire.
	stepForFloor := priorAvgStep
	if stepForFloor <= stepEps {
		stepForFloor = avgStep
	}
	stoploss.updateFloorAndExhaustion(stepForFloor, avgDownStep, chaosRatio, delta)

	// Every exit must clear the policy profit floor — never break-even, never loss.
	underFloor := stoploss.CurrentPnLPct < exitProfitFloor()
	if underFloor {
		stoploss.Confirms = 0
		if stoploss.ExplosiveActive && delta < -stepEps {
			stoploss.ExplosiveActive = false
			stoploss.ExplosivePushes = 0
			stoploss.PeakPushDelta = 0
		}
		return
	}

	// 7. Staged Scale-Out (Tranches 1..3) & Final Runner Exit (Tranche 4 / Floor Break)
	if stoploss.State == StoplossArmed {
		stoploss.evaluateTrancheScaleOut(delta, stepForFloor)

		if stoploss.State != StoplossTriggered {
			stoploss.evaluateRegimeExit(delta, stepForFloor)
		}
	}
}

func (stoploss *Stoploss) Disarm() {
	stoploss.State = StoplossHealthy
	stoploss.Confirms = 0
	stoploss.TriggerReason = ""
	stoploss.ExplosiveActive = false
	stoploss.ExplosivePushes = 0
	stoploss.ExplosiveTTL = 0
	stoploss.PeakPushDelta = 0
}

/*
onArm enters StoplossArmed, reseeds swing memory so CRAWL/SMOOTH breathing
room is not sized by underwater chop, and sets the Tranche-2 line at Arm+1.
*/
func (stoploss *Stoploss) onArm() {
	stoploss.State = StoplossArmed
	stoploss.Confirms = 0

	stoploss.Pushes = stoploss.Pushes[:0]
	stoploss.Pullbacks = stoploss.Pullbacks[:0]
	stoploss.TypicalPush = 0

	minRoom := math.Max(stoploss.ArmPnlPct*0.35, minArmedPullbackFloor)
	if recovery := stoploss.PeakPnLPct - stoploss.InitialPnLPct; recovery > stepEps {
		minRoom = math.Max(minRoom, recovery*0.15)
	}
	stoploss.TypicalPullback = minRoom

	stoploss.RallyStart = stoploss.CurrentPnLPct
	stoploss.PullbackLow = stoploss.CurrentPnLPct
	stoploss.PushRecorded = false

	if stoploss.TranchesExited == 0 {
		stoploss.NextTargetPct = stoploss.ArmPnlPct + stoploss.trancheStep()
	}
}

/*
trancheStep spaces mid banks by learned push size so explosive runners are not
skimmed at fixed +1/+2 pct points.
*/
func (stoploss *Stoploss) trancheStep() float64 {
	step := stoploss.TypicalPush
	if step < stoploss.ArmPnlPct {
		step = stoploss.ArmPnlPct
	}
	if step < 1.0 {
		step = 1.0
	}

	// Alfonsi/Blanc + Cont: when flow is critically self-excited or bid OFI
	// flips negative (liquidity fleeing), bank earlier into remaining demand.
	// Never shrink below the profit floor — no break-even/loss exits.
	floor := exitProfitFloor()
	if stoploss.CurrentPnLPct < floor {
		return step
	}

	if stoploss.Excitation > 0.55 {
		tight := 1.0 - 0.40*math.Min(stoploss.Excitation, 1)
		step *= tight
	}

	// Bid withdrawal: FlowOFI < -0.25 means net touch liquidity leaving the bid.
	if stoploss.FlowOFI < -0.25 {
		flee := math.Min(1, -stoploss.FlowOFI)
		step *= 1.0 - 0.35*flee
	}

	if step < floor {
		step = floor
	}

	return step
}

/*
claimedTranches is filled + in-flight slices — scale-out uses this so a pending
bank is not re-emitted before CommitPending, while abort can still roll back.
*/
func (stoploss *Stoploss) claimedTranches() int {
	return stoploss.TranchesExited + stoploss.PendingTranches
}

/*
evaluateTrancheScaleOut banks Tranche 1 at Arm and Tranche 2 at Arm+step (or CHAOS
upward whip). The remaining half is left for trail trip — not sold here.
*/
func (stoploss *Stoploss) evaluateTrancheScaleOut(delta, avgStep float64) {
	if stoploss.claimedTranches() == 0 &&
		stoploss.CurrentPnLPct >= stoploss.ArmPnlPct-stepEps {
		stoploss.emitTranche("tranche_1_arm")
	}

	for claimed := stoploss.claimedTranches(); claimed >= 1 && claimed < earlyScaleOuts; claimed = stoploss.claimedTranches() {
		hitTarget := stoploss.CurrentPnLPct >= stoploss.NextTargetPct-stepEps
		chaosWhip := stoploss.Regime == RegimeChaos &&
			delta > stepEps &&
			stoploss.CurrentPnLPct >= stoploss.ArmPnlPct &&
			(stoploss.CurrentPnLPct >= stoploss.PeakPnLPct-avgStep*0.5 || delta >= avgStep*1.1)
		if !hitTarget && !chaosWhip {
			break
		}
		reason := fmt.Sprintf("tranche_%d_target", claimed+1)
		if !hitTarget && chaosWhip {
			reason = fmt.Sprintf("tranche_%d_chaos_whip", claimed+1)
		}
		stoploss.emitTranche(reason)
		claimed = stoploss.claimedTranches()
		if claimed < earlyScaleOuts {
			stoploss.NextTargetPct = stoploss.ArmPnlPct +
				stoploss.trancheStep()*float64(claimed)
		}
	}
}

func (stoploss *Stoploss) emitTranche(reason string) {
	if stoploss.claimedTranches() >= earlyScaleOuts {
		return
	}
	stoploss.PendingTranches++
	stoploss.PendingExitPct += trancheFraction
	stoploss.TriggerReason = reason
}

/*
trip sells whatever is still open when the trailing floor / peak exhaustion
fires — the final cake slice (and any unsold earlier stages). Claims the
remainder; CommitPending advances TranchesExited only after the fill.
Never trips at break-even or a loss — PolicyExitMinProfitPct is absolute.
*/
func (stoploss *Stoploss) trip(reason string) {
	if stoploss.CurrentPnLPct < exitProfitFloor() {
		return
	}
	if stoploss.TranchesExited == 0 && stoploss.PendingTranches == 0 {
		if stoploss.CurrentPnLPct < stoploss.ArmPnlPct-stepEps {
			return
		}
	}
	remaining := totalTranches - stoploss.TranchesExited
	if remaining < 1 {
		remaining = 1
	}
	stoploss.PendingExitPct = 1
	stoploss.PendingTranches = remaining
	stoploss.State = StoplossTriggered
	stoploss.TriggerReason = reason
}

/*
CommitPending folds in-flight slices into TranchesExited after a sell fill.
*/
func (stoploss *Stoploss) CommitPending() {
	if stoploss == nil {
		return
	}
	stoploss.TranchesExited += stoploss.PendingTranches
	if stoploss.TranchesExited > totalTranches {
		stoploss.TranchesExited = totalTranches
	}
	stoploss.PendingTranches = 0
	stoploss.PendingExitPct = 0
}

/*
AbortPending drops an unfilled claim. A vetoed trip returns to Armed so the
next mark can re-evaluate instead of looping BUSY on StoplossTriggered.
Filled banks (TranchesExited) are left alone; Disarm still requires zero fills.
*/
func (stoploss *Stoploss) AbortPending() {
	if stoploss == nil {
		return
	}
	stoploss.PendingExitPct = 0
	stoploss.PendingTranches = 0
	if stoploss.State == StoplossTriggered {
		stoploss.State = StoplossArmed
		stoploss.TriggerReason = ""
		stoploss.Confirms = 0
	}
}

/*
WantsExit is true when a tranche slice or a full trail trip should hit the wire.
*/
func (stoploss *Stoploss) WantsExit() bool {
	return stoploss != nil &&
		(stoploss.State == StoplossTriggered || stoploss.PendingExitPct > stepEps)
}
func (stoploss *Stoploss) sharpBreakAllowed() bool {
	return stoploss.PeakPnLPct >= stoploss.ArmPnlPct+sharpBreakPeakMargin
}

/*
detectExplosiveIgnition returns whether a runner just ignited and the push size
to seed PeakPushDelta. Per-tick spikes still count; multi-tick climbs that sum
above the same thresholds also count so a vertical leg of small ticks gets the
wider explosive floor instead of CRAWL's tight trail.
*/
func (stoploss *Stoploss) detectExplosiveIgnition(delta, priorAvgStep, friction float64) (bool, float64) {
	if delta <= stepEps {
		return false, 0
	}
	threshold := math.Max(math.Max(friction*2.5, stoploss.ArmPnlPct*1.5), 0.50)
	if len(stoploss.Deltas) > 4 {
		baseline := math.Max(priorAvgStep, 0.10)
		threshold = math.Max(baseline*3.5, 0.50)
	}
	if delta >= threshold {
		return true, delta
	}
	// Early trade: 2-tick consecutive burst.
	if len(stoploss.Deltas) <= 4 {
		n := len(stoploss.Deltas)
		if n >= 2 && stoploss.Deltas[n-2] > 0 {
			burst := stoploss.Deltas[n-2] + delta
			if burst >= threshold*1.4 && delta >= threshold*0.55 {
				return true, burst
			}
		}
	}
	if climb := stoploss.recentClimb(); climb >= threshold {
		return true, climb
	}
	if live := stoploss.PeakPnLPct - stoploss.RallyStart; live >= threshold {
		return true, live
	}
	return false, 0
}

/*
recentClimb is net progress over the trailing active-tick window (up to 8),
used to catch multi-tick impulses that never print a single ≥0.50% delta.
*/
func (stoploss *Stoploss) recentClimb() float64 {
	n := len(stoploss.Deltas)
	if n == 0 {
		return 0
	}
	start := n - 8
	if start < 0 {
		start = 0
	}
	var net float64
	for _, d := range stoploss.Deltas[start:] {
		net += d
	}
	return net
}
func (stoploss *Stoploss) updateFloorAndExhaustion(avgStep, avgDownStep, chaosRatio, delta float64) {
	switch stoploss.Regime {
	case RegimeExplosive:
		// Give explosive runners room to breathe: floor trails below the peak by at least
		// typical pullback or 40% of the peak push, while staying strictly profitable.
		pushRef := math.Max(stoploss.PeakPushDelta, stoploss.PeakPnLPct-stoploss.InitialPnLPct)
		room := math.Max(stoploss.TypicalPullback*1.2, math.Max(pushRef*0.40, avgStep*1.5))
		if room < 0.20 {
			room = 0.20
		}
		minFloor := stoploss.trailFloorMin()
		stoploss.TrailingFloor = math.Max(stoploss.PeakPnLPct-room, minFloor)
		if delta < -stepEps {
			dropFrac := 0.0
			if room > stepEps {
				dropFrac = math.Min(1.0, stoploss.CurrentDrop/room)
			}
			stoploss.ExhaustScore = dropFrac
		} else if pushRef > stepEps {
			stoploss.ExhaustScore = math.Min(1.0, math.Max(0.0, 1.0-(delta/pushRef)))
		}
	case RegimeCrawl, RegimeSmooth:
		allowedRoom := stoploss.computeBreathingRoom(avgStep, avgDownStep)
		floor := stoploss.PeakPnLPct - allowedRoom
		minFloor := stoploss.trailFloorMin()
		if floor < minFloor {
			floor = minFloor
		}
		stoploss.TrailingFloor = floor
		dropRatio := 0.0
		if allowedRoom > stepEps {
			dropRatio = stoploss.CurrentDrop / allowedRoom
		}
		momPenalty := 0.0
		if stoploss.EmaDelta < 0 {
			momPenalty = 0.25
		}
		if stoploss.TypicalPush > stepEps && stoploss.TypicalPullback > stoploss.TypicalPush {
			momPenalty += 0.20
		}
		stoploss.ExhaustScore = math.Min(1.0, dropRatio*0.75+momPenalty)
	case RegimeChaos:
		chaosRoom := math.Max(stoploss.TypicalPullback*1.5, avgStep*2.5)
		floor := stoploss.PeakPnLPct - chaosRoom
		minFloor := stoploss.trailFloorMin()
		if floor < minFloor {
			floor = minFloor
		}
		stoploss.TrailingFloor = floor
		stoploss.ExhaustScore = math.Min(1.0, chaosRatio/8.0)
	}
}
func (stoploss *Stoploss) computeBreathingRoom(avgStep, avgDownStep float64) float64 {
	// Determine n1 vs n2 momentum health multiplier
	mult := 1.35
	if stoploss.TypicalPush > stepEps && stoploss.TypicalPullback > stepEps {
		if stoploss.TypicalPush >= stoploss.TypicalPullback*1.20 && stoploss.EmaDelta >= 0 {
			mult = 1.45 // Strong upward structure: respect normal pullbacks
		} else if stoploss.TypicalPush < stoploss.TypicalPullback*0.85 {
			mult = 1.15 // Pullbacks overtaking pushes: tighten tolerance
		}
	}
	downRef := avgDownStep
	if downRef <= stepEps {
		downRef = avgStep
	}
	if downRef <= stepEps {
		downRef = 0.01
	}
	var stepFloor float64
	if stoploss.Regime == RegimeCrawl {
		// Crawl tolerates 3-4 step pullbacks before breaking structure
		stepFloor = downRef * 4.5
	} else {
		// Smooth tolerates 1-2 step pullbacks
		stepFloor = downRef * 2.2
	}
	return math.Max(stoploss.TypicalPullback*mult, stepFloor)
}
func (stoploss *Stoploss) evaluateRegimeExit(delta, avgStep float64) {
	switch stoploss.Regime {
	case RegimeExplosive:
		// REGIME 2: Fast-moving runner.
		// 1) Exit if price breaks below the trailing floor on negative progress.
		if stoploss.CurrentPnLPct <= stoploss.TrailingFloor && delta < -stepEps {
			stoploss.trip("explosive_floor_break")
			return
		}
		// 2) Climax blow-off: only after at least 5 strong consecutive pushes AND momentum flips negative.
		if stoploss.ExplosivePushes >= 5 && delta < -stepEps {
			stoploss.trip("explosive_push_climax")
			return
		}
		// 3) Reversal after stall: if momentum turns distinctly negative after climbing well above minimum.
		if stoploss.CurrentPnLPct >= stoploss.ArmPnlPct && stoploss.EmaDelta < -stepEps && delta < -avgStep*0.5 {
			stoploss.trip("explosive_reversal")
			return
		}
	case RegimeCrawl, RegimeSmooth:
		// REGIMES 1 & 3: Exit when price breaks learned breathing room AND momentum is negative.
		if stoploss.CurrentPnLPct <= stoploss.TrailingFloor {
			// First-break sharp exit only after peak has cleared arm by a real margin;
			// early profit peaks fall through to trail_confirm instead.
			if stoploss.sharpBreakAllowed() &&
				delta < -stepEps && stoploss.EmaDelta < 0 &&
				stoploss.CurrentPnLPct <= stoploss.TrailingFloor-(avgStep*0.5) {
				stoploss.trip("trail_sharp_break")
				return
			}
			// Require a real down tick — flat books must not accumulate confirms.
			if delta < -stepEps && stoploss.EmaDelta <= 0 {
				stoploss.Confirms++
				if stoploss.Confirms >= 2 {
					stoploss.trip("trail_confirm")
				}
			} else if delta > stepEps {
				stoploss.Confirms = 0
			}
		} else {
			stoploss.Confirms = 0
		}
	case RegimeChaos:
		// REGIME 4: Wild swings. Do not stop out on down-noise; sell final runner into an upward whip
		// once Tranches 1..3 have already been banked.
		nearRangeTop := stoploss.CurrentPnLPct >= (stoploss.PeakPnLPct - avgStep*0.5)
		strongWhipUp := delta >= avgStep*1.1
		if stoploss.claimedTranches() >= totalTranches-1 &&
			delta > stepEps &&
			stoploss.CurrentPnLPct >= stoploss.ArmPnlPct &&
			(nearRangeTop || strongWhipUp) {
			stoploss.trip("chaos_whip_up")
			return
		}
		// Disaster protection if chaos stops whipping up and bleeds down
		if stoploss.CurrentPnLPct <= stoploss.TrailingFloor && delta < -stepEps && stoploss.EmaDelta < 0 {
			stoploss.Confirms++
			if stoploss.Confirms >= 2 {
				stoploss.trip("chaos_bleed")
			}
		} else if delta > stepEps {
			stoploss.Confirms = 0
		}
	}
}
func (stoploss *Stoploss) recordPush(push float64) {
	stoploss.Pushes = append(stoploss.Pushes, push)
	if len(stoploss.Pushes) > stoploss.MaxSwings {
		stoploss.Pushes = stoploss.Pushes[1:]
	}
}
func (stoploss *Stoploss) recordPullback(pb float64) {
	stoploss.Pullbacks = append(stoploss.Pullbacks, pb)
	if len(stoploss.Pullbacks) > stoploss.MaxSwings {
		stoploss.Pullbacks = stoploss.Pullbacks[1:]
	}
}
func (stoploss *Stoploss) refreshSwingStats() {
	livePush := stoploss.PeakPnLPct - stoploss.RallyStart

	if len(stoploss.Pushes) > 0 {
		var sum float64
		for _, p := range stoploss.Pushes {
			sum += p
		}
		avgPush := sum / float64(len(stoploss.Pushes))
		stoploss.TypicalPush = math.Max(avgPush, livePush*0.7)
	} else if livePush > stoploss.TypicalPush {
		stoploss.TypicalPush = livePush
	}

	if len(stoploss.Pullbacks) > 0 {
		// 80th percentile of recent recovered pullbacks preserves periodic 3-4 tick dips
		// while letting ancient freak dips expire.
		cp := make([]float64, len(stoploss.Pullbacks))
		copy(cp, stoploss.Pullbacks)
		sort.Float64s(cp)
		idx := max(int(math.Ceil(0.80*float64(len(cp))))-1, 0)

		if idx >= len(cp) {
			idx = len(cp) - 1
		}

		stoploss.TypicalPullback = cp[idx]
	}
}

func (stoploss *Stoploss) analyzeMemory() (
	avgStep, avgDownStep, netProgress, chaosRatio, flipRate float64,
) {
	n := len(stoploss.Deltas)

	if n == 0 {
		return 0, 0, 0, 0, 0
	}

	var totalMovement, downSum float64
	var downCount, flips int

	for i, d := range stoploss.Deltas {
		netProgress += d
		absD := math.Abs(d)
		totalMovement += absD

		if d < 0 {
			downSum += absD
			downCount++
		}

		if i > 0 && (d > 0) != (stoploss.Deltas[i-1] > 0) {
			flips++
		}
	}

	avgStep = totalMovement / float64(n)

	if downCount > 0 {
		avgDownStep = downSum / float64(downCount)
	} else {
		avgDownStep = avgStep
	}

	if n > 1 {
		flipRate = float64(flips) / float64(n-1)
	}

	// Continuous chaos ratio without threshold discontinuity
	denom := math.Max(math.Abs(netProgress), avgStep*0.1)

	if denom > stepEps {
		chaosRatio = totalMovement / denom
	}

	return avgStep, avgDownStep, netProgress, chaosRatio, flipRate
}

func (stoploss *Stoploss) Triggered() bool {
	return stoploss.State == StoplossTriggered
}
