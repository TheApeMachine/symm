package broker

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/system"
)

const stepEps = 1e-9

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

	isNewExplosive := stoploss.detectExplosiveIgnition(delta, priorAvgStep, friction)
	if isNewExplosive {
		stoploss.ExplosiveActive = true
		stoploss.ExplosiveTTL = 4
		stoploss.ExplosivePushes++
		if delta > stoploss.PeakPushDelta {
			stoploss.PeakPushDelta = delta
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

	// 5. Arming Logic
	effectiveArm := math.Max(stoploss.ArmPnlPct, friction*1.5)
	if stoploss.State == StoplossHealthy && stoploss.PeakPnLPct >= effectiveArm {
		stoploss.State = StoplossArmed
	}

	// 6. RULE 1: NEVER EXIT AT A LOSS
	minProfitableExit := math.Max(friction+0.01, 0.01)
	stoploss.updateFloorAndExhaustion(avgStep, avgDownStep, chaosRatio, minProfitableExit, delta)

	if stoploss.CurrentPnLPct <= minProfitableExit {
		stoploss.Confirms = 0
		// If an early spike faded before even reaching profit, exit burst mode
		if stoploss.ExplosiveActive && delta < -stepEps {
			stoploss.ExplosiveActive = false
			stoploss.ExplosivePushes = 0
			stoploss.PeakPushDelta = 0
		}
		return
	}

	// 7. Regime-Specific Exit Decisions (only once Armed and Profitable)
	if stoploss.State == StoplossArmed {
		stoploss.evaluateRegimeExit(delta, avgStep, friction, minProfitableExit)
	}
}

func (stoploss *Stoploss) trip(reason string) {
	stoploss.State = StoplossTriggered
	stoploss.TriggerReason = reason
}

func (stoploss *Stoploss) detectExplosiveIgnition(delta, priorAvgStep, friction float64) bool {
	if delta <= stepEps {
		return false
	}

	// Early trade (fewer than 4 active moves recorded): catch 1-2 tick ignitions immediately.
	// Requires a genuine momentum impulse above friction and arming threshold.
	if len(stoploss.Deltas) <= 4 {
		earlySpike := math.Max(math.Max(friction*2.5, stoploss.ArmPnlPct*1.5), 0.50)
		if delta >= earlySpike {
			return true
		}
		// Check 2-tick consecutive burst right out of the gate
		n := len(stoploss.Deltas)
		if n >= 2 && stoploss.Deltas[n-2] > 0 && (stoploss.Deltas[n-2]+delta) >= earlySpike*1.4 && delta >= earlySpike*0.55 {
			return true
		}
		return false
	}

	// Established trade: impulse must clearly exceed normal volatility step size and be at least 0.50%.
	baseline := math.Max(priorAvgStep, 0.10)
	return delta >= baseline*3.5 && delta >= 0.50
}

func (stoploss *Stoploss) updateFloorAndExhaustion(avgStep, avgDownStep, chaosRatio, minProfitableExit, delta float64) {
	switch stoploss.Regime {
	case RegimeExplosive:
		// Give explosive runners room to breathe: floor trails below the peak by at least
		// typical pullback or 40% of the peak push, while staying strictly profitable.
		pushRef := math.Max(stoploss.PeakPushDelta, stoploss.PeakPnLPct-stoploss.InitialPnLPct)
		room := math.Max(stoploss.TypicalPullback*1.2, math.Max(pushRef*0.40, avgStep*1.5))
		if room < 0.20 {
			room = 0.20
		}
		stoploss.TrailingFloor = math.Max(stoploss.PeakPnLPct-room, minProfitableExit)
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
		if floor < minProfitableExit {
			floor = minProfitableExit
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
		if floor < minProfitableExit {
			floor = minProfitableExit
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

func (stoploss *Stoploss) evaluateRegimeExit(delta, avgStep, friction, minProfitableExit float64) {
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
		minCapture := math.Max(minProfitableExit*1.5, friction*2.0)
		if stoploss.CurrentPnLPct >= minCapture && stoploss.EmaDelta < -stepEps && delta < -avgStep*0.5 {
			stoploss.trip("explosive_reversal")
			return
		}

	case RegimeCrawl, RegimeSmooth:
		// REGIMES 1 & 3: Exit when price breaks learned breathing room AND momentum is negative.
		if stoploss.CurrentPnLPct <= stoploss.TrailingFloor {
			// Sharp breakdown well past the floor on a down tick -> exit on 1st break
			if delta < -stepEps && stoploss.EmaDelta < 0 && stoploss.CurrentPnLPct <= stoploss.TrailingFloor-(avgStep*0.5) {
				stoploss.trip("trail_sharp_break")
				return
			}
			// Otherwise wait for confirmation that it is not immediately bouncing
			if delta <= stepEps && stoploss.EmaDelta <= 0 {
				stoploss.Confirms++
				if stoploss.Confirms >= 2 {
					stoploss.trip("trail_confirm")
				}
			} else if delta > stepEps {
				// Actively bouncing back up: don't sell into recovery
				stoploss.Confirms = 0
			}
		} else {
			stoploss.Confirms = 0
		}

	case RegimeChaos:
		// REGIME 4: Wild swings. Do not stop out on down-noise; sell into an upward whip.
		profitTarget := math.Max(minProfitableExit, math.Max(stoploss.ArmPnlPct, friction*2.0))
		nearRangeTop := stoploss.CurrentPnLPct >= (stoploss.PeakPnLPct - avgStep*0.5)
		strongWhipUp := delta >= avgStep*1.1

		if delta > stepEps && stoploss.CurrentPnLPct >= profitTarget && (nearRangeTop || strongWhipUp) {
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
		idx := int(math.Ceil(0.80*float64(len(cp)))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(cp) {
			idx = len(cp) - 1
		}
		stoploss.TypicalPullback = cp[idx]
	}
}

func (stoploss *Stoploss) analyzeMemory() (avgStep, avgDownStep, netProgress, chaosRatio, flipRate float64) {
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
