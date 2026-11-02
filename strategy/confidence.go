package strategy

import (
	"context"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

const confidenceStoreKey = "model/confidence/edge.bin"

/*
Confidence tracks paper and real settle stats that arm or halt live entries.
Virtual settles never count — they only refine path resistance.
*/
type Confidence struct {
	mu              sync.Mutex
	ctx             context.Context
	store           *store.Client
	paper           bookStats
	real            bookStats
	paperLossStreak int
	realLossStreak  int
	armed           bool
	eligibleFloor   float64
}

type bookStats struct {
	Closed int     `json:"closed"`
	Wins   int     `json:"wins"`
	SumRet float64 `json:"sum_return"`
}

type confidenceBlob struct {
	Paper           bookStats `json:"paper"`
	Real            bookStats `json:"real"`
	PaperLossStreak int       `json:"paper_loss_streak"`
	RealLossStreak  int       `json:"real_loss_streak"`
	Armed           bool      `json:"armed"`
}

/*
BookSnapshot is one book's published performance strip.
*/
type BookSnapshot struct {
	Active  bool     `json:"active"`
	Armed   bool     `json:"armed,omitempty"`
	Halted  bool     `json:"halted,omitempty"`
	Closed  int      `json:"closed"`
	Wins    int      `json:"wins"`
	WinRate *float64 `json:"win_rate"`
	Edge    *float64 `json:"edge"`
}

/*
ConfidenceSnapshot is the hub frame for the top-bar mode strip.
*/
type ConfidenceSnapshot struct {
	Virtual            BookSnapshot `json:"virtual"`
	Paper              BookSnapshot `json:"paper"`
	Real               BookSnapshot `json:"real"`
	EligibleResistance float64      `json:"eligible_resistance"`
	RealCeiling        bool         `json:"real_ceiling"`
}

func NewConfidence(ctx context.Context) *Confidence {
	confidence := &Confidence{
		ctx:           ctx,
		store:         store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
		eligibleFloor: PaperEligibleResistance(),
	}

	confidence.restore()
	confidence.reevaluateLocked()

	return confidence
}

/*
Armed reports whether new real entries may open.
*/
func (confidence *Confidence) Armed() bool {
	confidence.mu.Lock()
	defer confidence.mu.Unlock()

	return confidence.armed && RealCeilingAllowed()
}

/*
Record folds one settled venue lot into paper/real edge and re-arms.
*/
func (confidence *Confidence) Record(position *broker.Position) {
	if position == nil {
		return
	}

	book := position.Book
	if book == "" {
		if position.Virtual {
			return
		}

		book = string(TierPaper)
	}

	if book == string(TierVirtual) {
		return
	}

	ret := 0.0
	if position.RealizedPct != 0 {
		ret = position.RealizedPct
	} else if position.PnLPct != nil {
		ret = position.PnLPct.MustFloat64() / 100.0
	}

	win := ret > 0

	confidence.mu.Lock()
	defer confidence.mu.Unlock()

	switch book {
	case string(TierReal):
		confidence.real.Closed++
		confidence.real.SumRet += ret
		if win {
			confidence.real.Wins++
			confidence.realLossStreak = 0
		} else {
			confidence.realLossStreak++
		}
	default:
		confidence.paper.Closed++
		confidence.paper.SumRet += ret
		if win {
			confidence.paper.Wins++
			confidence.paperLossStreak = 0
		} else {
			confidence.paperLossStreak++
		}
	}

	confidence.reevaluateLocked()
	confidence.persistLocked()
	confidence.publishLocked()
}

/*
Snapshot returns the current confidence frame for the dashboard.
*/
func (confidence *Confidence) Snapshot() ConfidenceSnapshot {
	confidence.mu.Lock()
	defer confidence.mu.Unlock()

	return confidence.snapshotLocked()
}

/*
Publish pushes the confidence frame to the hub.
*/
func (confidence *Confidence) Publish() {
	confidence.mu.Lock()
	defer confidence.mu.Unlock()

	confidence.publishLocked()
}

func (confidence *Confidence) reevaluateLocked() {
	learn := system.Cfg.Learning
	confirm := learn.ConfirmAfter
	minWin := learn.MinWinRate
	minEdge := learn.MinMeanReturn
	demotion := learn.DemotionLosses

	// Paper establishes the edge; real settlements refine it once armed.
	closed := confidence.paper.Closed + confidence.real.Closed
	wins := confidence.paper.Wins + confidence.real.Wins
	sum := confidence.paper.SumRet + confidence.real.SumRet

	var rate, edge *float64
	if closed > 0 {
		r := float64(wins) / float64(closed)
		e := sum / float64(closed)
		rate, edge = &r, &e
	}

	armed := RealCeilingAllowed() &&
		closed >= confirm &&
		rate != nil && *rate >= minWin &&
		edge != nil && *edge >= minEdge

	if confidence.paperLossStreak >= demotion || confidence.realLossStreak >= demotion {
		armed = false
	}

	if rate != nil && *rate < minWin {
		armed = false
	}

	if edge != nil && *edge < minEdge {
		armed = false
	}

	confidence.armed = armed
	confidence.eligibleFloor = PaperEligibleResistance()
}

func (stats bookStats) rateEdge() (rate, edge *float64) {
	if stats.Closed <= 0 {
		return nil, nil
	}

	r := float64(stats.Wins) / float64(stats.Closed)
	e := stats.SumRet / float64(stats.Closed)

	return &r, &e
}

func (confidence *Confidence) snapshotLocked() ConfidenceSnapshot {
	paperRate, paperEdge := confidence.paper.rateEdge()
	realRate, realEdge := confidence.real.rateEdge()
	ceiling := RealCeilingAllowed()

	return ConfidenceSnapshot{
		Virtual: BookSnapshot{Active: true},
		Paper: BookSnapshot{
			Active:  true,
			Closed:  confidence.paper.Closed,
			Wins:    confidence.paper.Wins,
			WinRate: paperRate,
			Edge:    paperEdge,
		},
		Real: BookSnapshot{
			Active:  ceiling,
			Armed:   confidence.armed && ceiling,
			Halted:  ceiling && !confidence.armed,
			Closed:  confidence.real.Closed,
			Wins:    confidence.real.Wins,
			WinRate: realRate,
			Edge:    realEdge,
		},
		EligibleResistance: confidence.eligibleFloor,
		RealCeiling:        ceiling,
	}
}

func (confidence *Confidence) publishLocked() {
	if ui.UI == nil || !ui.UI.Active() {
		return
	}

	ui.UI.WriteJSON(map[string]any{
		"confidence": confidence.snapshotLocked(),
	})
}

func (confidence *Confidence) persistLocked() {
	blob := confidenceBlob{
		Paper:           confidence.paper,
		Real:            confidence.real,
		PaperLossStreak: confidence.paperLossStreak,
		RealLossStreak:  confidence.realLossStreak,
		Armed:           confidence.armed,
	}

	body, err := sonic.Marshal(map[string]any{"state": blob})

	if err != nil {
		errnie.Error(err)
		return
	}

	if _, err = confidence.store.WriteAtPrefix(confidenceStoreKey, body); err != nil {
		errnie.Error(err)
	}
}

func (confidence *Confidence) restore() {
	raw := confidence.store.Fetch(confidence.ctx, confidenceStoreKey)

	if len(raw) == 0 {
		return
	}

	blob, err := data.AttributeOk[confidenceBlob](raw, "state")

	if err != nil {
		return
	}

	confidence.paper = blob.Paper
	confidence.real = blob.Real
	confidence.paperLossStreak = blob.PaperLossStreak
	confidence.realLossStreak = blob.RealLossStreak
	confidence.armed = blob.Armed
}
