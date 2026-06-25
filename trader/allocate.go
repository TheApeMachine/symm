package trader

import (
	"math"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

/*
rankedEntry is a scored entry candidate awaiting slot allocation: the action to
dispatch, its expected-free-energy score (rank key), and the playbook confidence
that scales its risk size.
*/
type rankedEntry struct {
	action     *datura.Artifact
	score      float64
	confidence float64
}

/*
allocation is the trader's risk policy: how many concurrent positions to run and
how large each is. Slot counts and the base risk fraction are explicit policy
(config, not market-derived), but the reserved-slot bar is derived from the live
candidate distribution so it adapts instead of being a magic threshold.

Two normal slots are held back as reserved slots that only a statistically
standout entry may claim — so a sudden, high-conviction pump is never missed just
because the normal slots are full.
*/
type allocation struct {
	normalSlots   int
	reservedSlots int
	baseFraction  float64
	quote         string
}

/*
newAllocation reads the risk policy from config, defaulting to four normal slots,
two reserved slots, and a ten-percent base risk fraction when unset.
*/
func newAllocation() allocation {
	normal := viper.GetInt("trading.slots.normal")

	if normal <= 0 {
		normal = 4
	}

	reserved := viper.GetInt("trading.slots.reserved")

	if reserved <= 0 {
		reserved = 2
	}

	base := viper.GetFloat64("trading.sizing.base_fraction")

	if base <= 0 {
		base = 0.10
	}

	return allocation{
		normalSlots:   normal,
		reservedSlots: reserved,
		baseFraction:  base,
		quote:         strings.ToUpper(viper.GetString("market.quote_currency")),
	}
}

/*
fraction is the risk size for an entry: the base fraction scaled by the playbook
confidence (a 1.0-confidence entry sizes at the full base fraction; weaker
convictions size down proportionally). Absent confidence sizes at the base.
*/
func (alloc allocation) fraction(confidence float64) float64 {
	if confidence <= 0 {
		return alloc.baseFraction
	}

	if confidence > 1 {
		confidence = 1
	}

	return alloc.baseFraction * confidence
}

/*
heldCount is the number of open positions the ledger already carries, excluding
the quote currency (which funds entries rather than being one). Each open
position occupies a normal slot.
*/
func (alloc allocation) heldCount(balances *logic.Balances) int {
	if balances == nil {
		return 0
	}

	count := 0

	for _, asset := range balances.Asset {
		if asset.Balance > 0 && strings.ToUpper(asset.Asset) != alloc.quote {
			count++
		}
	}

	return count
}

/*
eliteBar is the reserved-slot threshold derived from the candidate scores this
tick: one standard deviation above the mean. Only a statistical standout clears
it, which is exactly the sudden-pump case the reserved slots exist to catch. With
too few candidates to form a distribution it falls back to the strongest score,
so a lone outlier can still claim a reserved slot.
*/
func eliteBar(entries []rankedEntry) float64 {
	// A standout needs peers to stand out from; with fewer than two candidates
	// there is no distribution, so nothing qualifies as elite.
	if len(entries) < 2 {
		return math.Inf(1)
	}

	sum := 0.0

	for _, entry := range entries {
		sum += entry.score
	}

	mean := sum / float64(len(entries))

	variance := 0.0

	for _, entry := range entries {
		delta := entry.score - mean
		variance += delta * delta
	}

	std := math.Sqrt(variance / float64(len(entries)))

	// Homogeneous batch: no dispersion means no standout, so nothing is elite.
	if std == 0 {
		return math.Inf(1)
	}

	return mean + std
}

/*
admit applies the slot policy to score-sorted entries. The strongest fill the
available normal slots (total normal slots minus open positions); entries beyond
that are admitted only into a reserved slot, and only if their score clears the
derived elite bar. Each admitted entry is sized by risk fraction in place.
*/
func (alloc allocation) admit(
	entries []rankedEntry,
	balances *logic.Balances,
) []rankedEntry {
	normalAvailable := max(alloc.normalSlots - alloc.heldCount(balances), 0)

	bar := eliteBar(entries)
	reservedUsed := 0
	admitted := make([]rankedEntry, 0, len(entries))

	for index, entry := range entries {
		switch {
		case index < normalAvailable:
		case reservedUsed < alloc.reservedSlots && entry.score > bar:
			reservedUsed++
		default:
			continue
		}

		entry.action.Merge("fraction", alloc.fraction(entry.confidence))
		admitted = append(admitted, entry)
	}

	return admitted
}
