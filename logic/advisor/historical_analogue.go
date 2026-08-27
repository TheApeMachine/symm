package advisor

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
historicalTrajectoryLength is how many regime observations one archived
trajectory window holds. It is a declared structural bound on the comparison
window, not a tuned market-time horizon: the window counts category transitions,
not seconds.
*/
const historicalTrajectoryLength = 32

/*
historicalArchiveCapacity bounds how many completed windows the archive retains
per symbol. Resident state therefore scales with symbols plus this fixed cap,
never with elapsed history.
*/
const historicalArchiveCapacity = 128

/*
historicalMinComparable is the smallest trajectory length that yields a
non-degenerate distance. A one-observation prefix is a single point, so its
Hamming distance carries no ordering information; two observations are the
smallest meaningful comparison. It is a structural minimum, not a threshold.
*/
const historicalMinComparable = 2

/*
resident is one symbol's bounded analogue state: the in-progress trajectory
window, its fill count and start time, the bounded archive of completed windows,
a reusable distance scratch buffer, and the emitted-perspective sequence.
Delivery is serialized per subscriber, so one resident is touched one symbol at
a time and needs no per-symbol lock.
*/
type resident struct {
	window    [historicalTrajectoryLength]uint8
	fill      int
	from      time.Time
	archive   [][historicalTrajectoryLength]uint8
	distances []float64
	sequence  uint64
}

/*
HistoricalAnalogueAdvisor answers, per symbol: "has this symbol previously
exhibited a regime trajectory similar to the one observed now, and where does
the present trajectory sit relative to those archived episodes?"

It consumes the ranked Category batch (the same semantic output the opportunity
synthesizer reads), reduces each tick to its dominant regime as an interned
category index, and matches the in-progress trajectory against the symbol's own
bounded archive of completed trajectory windows. It is descriptive context only:
it never emits a gate, a score, or an action.
*/
type HistoricalAnalogueAdvisor struct {
	ctx           context.Context
	cancel        context.CancelFunc
	slots         sync.Map
	ObserveModule func(string, time.Duration)
}

/*
NewHistoricalAnalogueAdvisor constructs the advisor and, when a workspace is
supplied, wires it on ChannelCategories → ChannelPerspectives.
*/
func NewHistoricalAnalogueAdvisor(ctx context.Context, bus *runtime.Workspace) *HistoricalAnalogueAdvisor {
	ctx, cancel := context.WithCancel(ctx)

	advisor := &HistoricalAnalogueAdvisor{
		ctx:    ctx,
		cancel: cancel,
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

var _ Advisor[[]types.Category] = (*HistoricalAnalogueAdvisor)(nil)

func (advisor *HistoricalAnalogueAdvisor) Name() string { return "advisor:historical_analogue" }

func (advisor *HistoricalAnalogueAdvisor) Error() error { return nil }

/*
Close cancels the advisor context.
*/
func (advisor *HistoricalAnalogueAdvisor) Close() error {
	advisor.cancel()
	return nil
}

/*
Step folds one ranked category batch into the symbol's analogue state and
returns the current Perspective. A batch with no categories returns nil: nothing
changed, so no context is emitted.
*/
func (advisor *HistoricalAnalogueAdvisor) Step(categories []types.Category) *types.Perspective {
	if len(categories) == 0 {
		return nil
	}

	symbol := categories[0].Symbol
	now := categories[0].At
	slot := advisor.slotFor(symbol)

	slot.advance(dominantRegime(categories), now)

	return slot.perspective(symbol, now)
}

/*
slotFor returns the resident analogue slot for one symbol, creating it on first
sight.
*/
func (advisor *HistoricalAnalogueAdvisor) slotFor(symbol string) *resident {
	if stored, found := advisor.slots.Load(symbol); found {
		return stored.(*resident)
	}

	candidate := &resident{}
	actual, _ := advisor.slots.LoadOrStore(symbol, candidate)

	return actual.(*resident)
}

/*
advance appends one regime observation to the in-progress window and, when the
window fills, archives it and starts a fresh one. The archive stays bounded at
historicalArchiveCapacity by dropping the oldest window.
*/
func (slot *resident) advance(regime uint8, now time.Time) {
	if slot.fill == 0 {
		slot.from = now
	}

	slot.window[slot.fill] = regime
	slot.fill++

	if slot.fill < historicalTrajectoryLength {
		return
	}

	slot.archiveWindow()
	slot.fill = 0
}

/*
archiveWindow stores the completed window, shifting the oldest out when the
archive is at capacity so the backing array stays bounded.
*/
func (slot *resident) archiveWindow() {
	if len(slot.archive) >= historicalArchiveCapacity {
		copy(slot.archive, slot.archive[1:])
		slot.archive[len(slot.archive)-1] = slot.window
		return
	}

	slot.archive = append(slot.archive, slot.window)
}

/*
perspective materializes the current descriptive output. Distance fields stay
zero-valued and are only populated once there is at least one archived window
and a comparable-length trajectory, so a zero distance is never mistaken for a
real estimate.
*/
func (slot *resident) perspective(symbol string, now time.Time) *types.Perspective {
	perspective := &types.Perspective{
		Symbol:   symbol,
		Kind:     types.KindHistoricalAnalogue,
		From:     slot.from,
		At:       now,
		Sequence: slot.sequence,
		Maturity: stageAlignment(slot.fill),
		Analogue: types.HistoricalAnaloguePayload{
			Support:        len(slot.archive),
			StageAlignment: stageAlignment(slot.fill),
		},
	}

	slot.sequence++

	if len(slot.archive) == 0 || slot.fill < historicalMinComparable {
		return perspective
	}

	perspective.Analogue.NearestDistance, perspective.Analogue.MedianDistance = slot.distanceSummary()

	return perspective
}

/*
distanceSummary computes the normalized Hamming distance from the current
trajectory prefix to every archived window's aligned prefix, then returns the
nearest (minimum) and the median — the symbol's own typical self-distance, which
gives the nearest its scale without a tuned threshold.
*/
func (slot *resident) distanceSummary() (nearest, median float64) {
	count := len(slot.archive)

	if cap(slot.distances) < count {
		slot.distances = make([]float64, 0, count)
	}

	slot.distances = slot.distances[:0]

	for _, archived := range slot.archive {
		slot.distances = append(slot.distances, normalizedHamming(slot.window[:slot.fill], archived[:slot.fill]))
	}

	slices.Sort(slot.distances)

	return slot.distances[0], slot.distances[count/2]
}

/*
dominantRegime reduces a ranked category batch to the single regime with the
greatest supporting strength, interned through the stable category vocabulary.
A batch with no positively supported category resolves to the zero regime,
recording an observation with no active regime rather than fabricating one.
*/
func dominantRegime(categories []types.Category) uint8 {
	bestStrength := 0.0
	bestIndex := 0

	for _, category := range categories {
		if category.Strength <= 0 {
			continue
		}

		if category.Strength > bestStrength {
			bestStrength = category.Strength
			bestIndex = types.CategoryIndex(category.Type)
		}
	}

	return uint8(bestIndex)
}

/*
normalizedHamming is the fraction of mismatching positions between two equal
length regime sequences, in [0,1]. A distance of zero means identical regime
sequences; one means no position agrees.
*/
func normalizedHamming(left, right []uint8) float64 {
	if len(left) == 0 {
		return 0
	}

	mismatches := 0

	for index := range left {
		if left[index] != right[index] {
			mismatches++
		}
	}

	return float64(mismatches) / float64(len(left))
}

/*
stageAlignment is the in-progress trajectory's fill fraction, in [0,1].
*/
func stageAlignment(fill int) float64 {
	return float64(fill) / float64(historicalTrajectoryLength)
}
