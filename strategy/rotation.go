package strategy

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
)

/*
Rotation remembers settle history so recurring Kraken rotators earn board score.
*/
type Rotation struct {
	*runtime.System
	mu    sync.RWMutex
	stats map[string]*rotationStat
}

type rotationStat struct {
	Closes   int
	Wins     int
	LastSeen time.Time
}

/*
NewRotation starts an in-memory rotation ledger for board ranking.
*/
func NewRotation(ctx context.Context) *Rotation {
	return &Rotation{
		System: runtime.NewSystem(ctx, "rotation"),
		stats:  make(map[string]*rotationStat),
	}
}

/*
NoteSettled ingests a closed lot for rotation scoring.
*/
func (rotation *Rotation) NoteSettled(position *broker.Position) {
	if rotation == nil || position == nil || position.Symbol == "" {
		return
	}

	rotation.mu.Lock()
	defer rotation.mu.Unlock()

	stat := rotation.stats[position.Symbol]

	if stat == nil {
		stat = &rotationStat{}
		rotation.stats[position.Symbol] = stat
	}

	stat.Closes++
	stat.LastSeen = time.Now().UTC()

	if position.PnLPct != nil && position.PnLPct.Sign() > 0 {
		stat.Wins++
	}
}

/*
Score returns a bounded boost for symbols with prior profitable closes here.
*/
func (rotation *Rotation) Score(symbol string) float64 {
	if rotation == nil || symbol == "" {
		return 0
	}

	rotation.mu.RLock()
	stat := rotation.stats[symbol]
	rotation.mu.RUnlock()

	if stat == nil || stat.Closes == 0 {
		return 0
	}

	boost := system.Cfg.Selection.RotationCloseBoost
	winRate := float64(stat.Wins) / float64(stat.Closes)

	return boost * winRate * float64(min(stat.Closes, 5))
}
