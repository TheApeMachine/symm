package config

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

/*
TunablesSearch hill-climbs numeric desk parameters using reward feedback from replay evals.
*/
type TunablesSearch struct {
	mu         sync.Mutex
	random     *rand.Rand
	source     *Config
	best       *Tunables
	bestReward float64
	hasBest    bool
	attempts   int
}

func NewTunablesSearch(source *Config, random *rand.Rand) *TunablesSearch {
	if source == nil {
		source = NewConfig()
	}

	if random == nil {
		random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	return &TunablesSearch{
		random: random,
		source: source,
	}
}

func (search *TunablesSearch) Next() Tunables {
	search.mu.Lock()
	defer search.mu.Unlock()

	search.attempts++

	if !search.hasBest || search.best == nil {
		return MutateTunables(search.source, search.random)
	}

	exploitRate := 0.45 + 0.25*math.Min(1, float64(search.attempts)/32)

	if search.random.Float64() < exploitRate {
		near := MutateTunablesNear(*search.best, search.random)

		return near
	}

	return MutateTunables(search.source, search.random)
}

func (search *TunablesSearch) Observe(tunables Tunables, reward float64) {
	search.mu.Lock()
	defer search.mu.Unlock()

	if search.hasBest && reward <= search.bestReward {
		return
	}

	clone := CloneTunables(tunables)
	search.best = &clone
	search.bestReward = reward
	search.hasBest = true
}

func (search *TunablesSearch) BestReward() float64 {
	search.mu.Lock()
	defer search.mu.Unlock()

	return search.bestReward
}
