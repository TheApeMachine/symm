package advisor

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
RelativeStateAdvisor answers, per symbol: "how does this symbol's current
regime compare with the rest of the market population's regimes?"

It consumes the same ranked Category batch as HistoricalAnalogueAdvisor, reduces
each tick to its dominant regime, and maintains a bounded population of each
symbol's current regime. From that it reports the symbol's regime breadth (the
fraction of the population currently in the same regime), plus the population's
majority regime and its share — measured cross-sectional facts, never an outlier
or leader classification.
*/
type RelativeStateAdvisor struct {
	ctx           context.Context
	cancel        context.CancelFunc
	regimes       map[string]uint8
	counts        map[uint8]int
	sequence      uint64
	ObserveModule func(string, time.Duration)

	mu sync.Mutex
}

/*
NewRelativeStateAdvisor constructs the advisor and, when a workspace is
supplied, wires it on ChannelCategories → ChannelPerspectives.
*/
func NewRelativeStateAdvisor(ctx context.Context, bus *runtime.Workspace) *RelativeStateAdvisor {
	ctx, cancel := context.WithCancel(ctx)

	advisor := &RelativeStateAdvisor{
		ctx:     ctx,
		cancel:  cancel,
		regimes: make(map[string]uint8),
		counts:  make(map[uint8]int),
	}

	if bus != nil {
		runtime.WireFunc[[]types.Category, *types.Perspective](
			bus,
			types.ChannelCategories,
			types.ChannelPerspectives,
			advisor.Step,
		)
	}

	return advisor
}

var _ Advisor[[]types.Category] = (*RelativeStateAdvisor)(nil)

func (advisor *RelativeStateAdvisor) Name() string { return "advisor:relative_state" }

func (advisor *RelativeStateAdvisor) Error() error { return nil }

/*
Close cancels the advisor context.
*/
func (advisor *RelativeStateAdvisor) Close() error {
	advisor.cancel()
	return nil
}

/*
Step folds one ranked category batch into the population and returns the current
relative-state Perspective for that symbol. A batch with no categories returns
nil.
*/
func (advisor *RelativeStateAdvisor) Step(categories []types.Category) *types.Perspective {
	if len(categories) == 0 {
		return nil
	}

	symbol := categories[0].Symbol
	now := categories[0].At
	regime := dominantRegime(categories)

	advisor.mu.Lock()
	defer advisor.mu.Unlock()

	advisor.observe(symbol, regime)

	peerCount := len(advisor.regimes)
	sameRegime := advisor.counts[regime]
	majorityRegime, majorityCount := advisor.majority()

	perspective := &types.Perspective{
		Symbol:   symbol,
		Kind:     types.KindRelativeState,
		From:     now,
		At:       now,
		Sequence: advisor.sequence,
		Maturity: populationMaturity(peerCount),
		Relative: types.RelativeStatePayload{
			PeerCount:       peerCount,
			SameRegime:      sameRegime,
			Breadth:         share(sameRegime, peerCount),
			MajorityRegime:  majorityRegime,
			MajorityBreadth: share(majorityCount, peerCount),
		},
	}

	advisor.sequence++

	return perspective
}

/*
observe moves one symbol's current regime in the population counts. A symbol
whose regime is unchanged costs one map probe and returns.
*/
func (advisor *RelativeStateAdvisor) observe(symbol string, regime uint8) {
	if previous, found := advisor.regimes[symbol]; found {
		if previous == regime {
			return
		}

		advisor.decrement(previous)
	}

	advisor.regimes[symbol] = regime
	advisor.counts[regime]++
}

func (advisor *RelativeStateAdvisor) decrement(regime uint8) {
	advisor.counts[regime]--

	if advisor.counts[regime] <= 0 {
		delete(advisor.counts, regime)
	}
}

/*
majority returns the most frequent regime and its count, breaking ties by the
lowest interned index so the answer is deterministic regardless of map iteration
order.
*/
func (advisor *RelativeStateAdvisor) majority() (uint8, int) {
	best := uint8(0)
	bestCount := 0

	for regime, count := range advisor.counts {
		if count > bestCount || (count == bestCount && regime < best) {
			best = regime
			bestCount = count
		}
	}

	return best, bestCount
}

/*
share is the fraction count/total, guarded to zero when the population is empty.
*/
func share(count, total int) float64 {
	if total <= 0 {
		return 0
	}

	return float64(count) / float64(total)
}

/*
populationMaturity is the established support-maturity convention (1 - 1/N): a
single-symbol population is not yet a cross-section, and the estimate grows
toward one as the population does.
*/
func populationMaturity(peerCount int) float64 {
	if peerCount <= 1 {
		return 0
	}

	return 1 - 1/float64(peerCount)
}
