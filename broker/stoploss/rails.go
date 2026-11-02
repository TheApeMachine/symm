package stoploss

import (
	"context"
	"math"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/system"
)

const railsStoreKey = "model/stoploss/rails.bin"

/*
Rails holds desk-wide stop statics that Learning nudges from
official-vs-shadow P/L regret. Snapshots freeze per lot so open episodes
do not chase live updates.
*/
type Rails struct {
	mu           sync.Mutex
	maxLoss      float64
	armPct       float64
	baseGiveback float64
	minGiveback  float64
	episodes     int
	edgeSum      float64
	store        *store.Client
	ctx          context.Context
}

/*
Snapshot is the frozen rail set one learning episode runs against.
*/
type Snapshot struct {
	MaxLoss      float64 `json:"max_loss"`
	ArmPct       float64 `json:"arm_pct"`
	BaseGiveback float64 `json:"base_giveback"`
	MinGiveback  float64 `json:"min_giveback"`
}

type railsBlob struct {
	MaxLoss      float64 `json:"max_loss"`
	ArmPct       float64 `json:"arm_pct"`
	BaseGiveback float64 `json:"base_giveback"`
	MinGiveback  float64 `json:"min_giveback"`
	Episodes     int     `json:"episodes"`
	EdgeSum      float64 `json:"edge_sum"`
}

func NewRails() *Rails {
	rails := &Rails{}
	rails.resetFromConfig()
	return rails
}

/*
AttachStore wires object storage and hydrates any prior Learn state.
*/
func (rails *Rails) AttachStore(ctx context.Context, client *store.Client) {
	rails.mu.Lock()
	defer rails.mu.Unlock()

	rails.ctx = ctx
	rails.store = client
	rails.restoreLocked()
}

func (rails *Rails) resetFromConfig() {
	risk := system.Cfg.Risk
	rails.maxLoss = risk.MaxAllowedLossPct
	rails.armPct = risk.TrailPeakArmPct
	rails.baseGiveback = risk.TrailBaseGivebackPct
	rails.minGiveback = risk.TrailMinGivebackPct
}

/*
Snapshot copies the current rails for one lot.
*/
func (rails *Rails) Snapshot() Snapshot {
	rails.mu.Lock()
	defer rails.mu.Unlock()

	return Snapshot{
		MaxLoss:      rails.maxLoss,
		ArmPct:       rails.armPct,
		BaseGiveback: rails.baseGiveback,
		MinGiveback:  rails.minGiveback,
	}
}

/*
Edge is the running mean of (shadowExit − officialExit). Positive means the
learning stop has been beating the active structural exit on average.
*/
func (rails *Rails) Edge() float64 {
	rails.mu.Lock()
	defer rails.mu.Unlock()

	if rails.episodes == 0 {
		return 0
	}

	return rails.edgeSum / float64(rails.episodes)
}

func (rails *Rails) Episodes() int {
	rails.mu.Lock()
	defer rails.mu.Unlock()

	return rails.episodes
}

/*
Learn nudges rails from one finished A/B pair, separating invalidation
adjustments from take-profit trailing adjustments.
*/
func (rails *Rails) Learn(officialPct, shadowPct float64) {
	rails.mu.Lock()
	defer rails.mu.Unlock()

	edge := shadowPct - officialPct
	rails.episodes++
	rails.edgeSum += edge

	step := 0.05 * math.Abs(edge)

	if step < 0.02 {
		step = 0.02
	}

	if step > 0.25 {
		step = 0.25
	}

	ceiling := system.Cfg.Risk.MaxInvalidationPct

	if shadowPct > 0 || officialPct > 0 {
		switch {
		case edge > 0.05:
			// Shadow held a better trail exit — more giveback, earlier arm.
			rails.baseGiveback = math.Min(2.5, rails.baseGiveback+step*0.5)
			rails.armPct = math.Max(0.5, rails.armPct-step*0.25)

		case edge < -0.05:
			// Shadow gave back too much — tighten the trail.
			rails.baseGiveback = math.Max(0.5, rails.baseGiveback-step*0.5)
			rails.minGiveback = math.Max(0.3, rails.minGiveback-step*0.25)
		}
	} else {
		switch {
		case edge > 0.05:
			// Shadow cut the red lot cleaner — tighten invalidation.
			rails.maxLoss = math.Max(1.5, rails.maxLoss-step*0.5)

		case edge < -0.05:
			// Shadow took a worse loss — widen invalidation up to the ceiling.
			rails.maxLoss = math.Min(ceiling, rails.maxLoss+step*0.5)
		}
	}

	rails.persistLocked()
}

func (rails *Rails) persistLocked() {
	if rails.store == nil {
		return
	}

	body, err := sonic.Marshal(map[string]any{
		"state": railsBlob{
			MaxLoss:      rails.maxLoss,
			ArmPct:       rails.armPct,
			BaseGiveback: rails.baseGiveback,
			MinGiveback:  rails.minGiveback,
			Episodes:     rails.episodes,
			EdgeSum:      rails.edgeSum,
		},
	})

	if err != nil {
		errnie.Error(err)
		return
	}

	if _, err = rails.store.WriteAtPrefix(railsStoreKey, body); err != nil {
		errnie.Error(err)
	}
}

func (rails *Rails) restoreLocked() {
	if rails.store == nil {
		return
	}

	ctx := rails.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	raw := rails.store.Fetch(ctx, railsStoreKey)

	if len(raw) == 0 {
		return
	}

	blob, err := data.AttributeOk[railsBlob](raw, "state")

	if err != nil {
		return
	}

	if blob.Episodes <= 0 {
		return
	}

	rails.maxLoss = blob.MaxLoss
	rails.armPct = blob.ArmPct
	rails.baseGiveback = blob.BaseGiveback
	rails.minGiveback = blob.MinGiveback
	rails.episodes = blob.Episodes
	rails.edgeSum = blob.EdgeSum
}
