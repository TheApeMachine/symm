package trader

import (
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
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
allocation is the trader's WALLET RISK POLICY — not market logic. How many
concurrent positions the wallet runs and what fraction of it each entry risks are
deliberate capital-management choices read from config (trading.slots.*,
trading.sizing.base_fraction), the same way a desk sets position limits. They are
not, and should not be, derived from market microstructure: the market tells the
trader WHICH opportunities exist (via the ranked candidate scores); the wallet
policy decides how much of the wallet to expose to them.

The one value that IS market-derived is the reserved-slot bar: it adapts to the
live candidate score distribution (eliteBar) so a standout entry is recognised
relative to its peers this tick, not against a magic constant.

Reserved slots are held back from the normal slots so a sudden, high-conviction
pump is never missed just because the normal slots are full. ponytail (ceiling):
base_fraction is a flat wallet fraction; a fuller policy would size against
per-symbol liquidity and broker qty_min/tick_size from the instrument tree so an
entry never sizes past what the book can fill. The slot/fraction knobs are the
upgrade surface for that.
*/
type allocation struct {
	normalSlots   int
	reservedSlots int
	maxPositions  int
	baseFraction  float64
	stopOffset    float64
	quote         string
}

/*
newAllocation reads the risk policy from config. max_concurrent_positions is the
hard cap; normal and reserved slots are just how that cap is partitioned.
*/
func newAllocation() allocation {
	maxPositions := viper.GetInt("trading.max_concurrent_positions")
	if maxPositions < 0 {
		panic("trader: trading.max_concurrent_positions cannot be negative")
	}

	normal := 4
	if viper.IsSet("trading.slots.normal") {
		normal = viper.GetInt("trading.slots.normal")
		if normal < 0 {
			panic("trader: trading.slots.normal cannot be negative")
		}
	} else if maxPositions > 0 {
		normal = maxPositions
	}

	reserved := 2
	if viper.IsSet("trading.slots.reserved") {
		reserved = viper.GetInt("trading.slots.reserved")
		if reserved < 0 {
			panic("trader: trading.slots.reserved cannot be negative")
		}
	}

	if maxPositions == 0 {
		maxPositions = normal + reserved
	}
	if normal > maxPositions {
		normal = maxPositions
	}
	if normal+reserved > maxPositions {
		reserved = max(maxPositions-normal, 0)
	}

	base := viper.GetFloat64("trading.sizing.base_fraction")

	if base <= 0 {
		if viper.IsSet("trading.sizing.base_fraction") {
			panic("trader: trading.sizing.base_fraction must be positive")
		}
		base = 0.10
	}

	return allocation{
		normalSlots:   normal,
		reservedSlots: reserved,
		maxPositions:  maxPositions,
		baseFraction:  base,
		stopOffset:    mustStopOffset(),
		quote:         strings.ToUpper(viper.GetString("market.quote_currency")),
	}
}

func ValidateStopPolicy() error {
	_, _, _, err := stopPolicyBPS()
	return err
}

func mustStopOffset() float64 {
	trailing, _, _, err := stopPolicyBPS()
	if err != nil {
		panic(err)
	}

	return trailing / 10000
}

func stopPolicyBPS() (trailing, minOffset, maxOffset float64, err error) {
	trailing = configFloatDefault("trading.stop.trailing_offset_bps", 100)
	minOffset = configFloatDefault("trading.stop.min_offset_bps", 20)
	maxOffset = configFloatDefault("trading.stop.max_offset_bps", 500)

	if minOffset <= 0 {
		return 0, 0, 0, fmt.Errorf("trading.stop.min_offset_bps must be positive")
	}
	if maxOffset < minOffset {
		return 0, 0, 0, fmt.Errorf("trading.stop.max_offset_bps must be >= trading.stop.min_offset_bps")
	}
	if trailing < minOffset || trailing > maxOffset {
		return 0, 0, 0, fmt.Errorf(
			"trading.stop.trailing_offset_bps must be between trading.stop.min_offset_bps and trading.stop.max_offset_bps",
		)
	}

	return trailing, minOffset, maxOffset, nil
}

func configFloatDefault(key string, fallback float64) float64 {
	if !viper.IsSet(key) {
		return fallback
	}

	return viper.GetFloat64(key)
}

/*
fraction is the actual risk size the desk applies to an entry: the wallet base
fraction scaled by the playbook confidence (a 1.0-confidence entry sizes at the
full base fraction; weaker convictions size down proportionally), then clamped to
the playbook's cap when one is set. The playbook fraction is a CEILING, not the
size — sizing is a wallet decision; the playbook only bounds how large any one of
its setups may ever risk. A non-positive or absent cap means "no extra ceiling",
so the wallet policy alone governs. Absent confidence sizes at the base.
*/
func (alloc allocation) fraction(confidence float64, cap float64) float64 {
	size := alloc.baseFraction

	if confidence > 0 {
		if confidence > 1 {
			confidence = 1
		}

		size = alloc.baseFraction * confidence
	}

	if cap > 0 && size > cap {
		return cap
	}

	return size
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
it, which is exactly the sudden-pump case the reserved slots exist to catch.

With a single positive-scored candidate there is no distribution to stand out
from — but a lone candidate that already failed to win a normal slot IS the
sudden-pump-with-slots-full case the reserved slots exist for, so the bar drops
below it (every positive score clears) rather than locking it out. With zero
candidates the bar is +Inf (nothing to admit).
*/
func eliteBar(entries []rankedEntry) float64 {
	// No candidates: nothing can clear the bar.
	if len(entries) == 0 {
		return math.Inf(1)
	}

	// A lone candidate has no peers to be "elite" against. Admit it to a
	// reserved slot on its own positive score rather than locking it out: the
	// reserved slots exist precisely so a single high-conviction pump is not
	// missed when the normal slots are full. The score > bar test in admit
	// requires bar strictly below the score, so go just under zero (scores that
	// reach admit are already gated positive in choose).
	if len(entries) == 1 {
		return math.Nextafter(0, math.Inf(-1))
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
	held := alloc.heldCount(balances)
	remaining := max(alloc.maxPositions-held, 0)
	if remaining == 0 {
		return nil
	}

	normalAvailable := min(max(alloc.normalSlots-held, 0), remaining)
	reservedAvailable := min(alloc.reservedSlots, max(remaining-normalAvailable, 0))

	bar := eliteBar(entries)
	reservedUsed := 0
	admitted := make([]rankedEntry, 0, len(entries))

	for index, entry := range entries {
		switch {
		case index < normalAvailable:
		case reservedUsed < reservedAvailable && entry.score > bar:
			reservedUsed++
		default:
			continue
		}

		// The playbook fraction already on the action is a cap, not the size:
		// the wallet policy sizes by confidence and the desk applies that,
		// clamped to the playbook's ceiling. Read the cap before overwriting.
		playbookCap := datura.Peek[float64](entry.action, "fraction")
		riskFraction := alloc.fraction(entry.confidence, playbookCap)
		entry.action.Merge("fraction", riskFraction)
		entry.action.Merge("cl_ord_id", uuid.NewString())

		if datura.Peek[float64](entry.action, "offset") <= 0 {
			entry.action.Merge("offset", alloc.stopOffset)
		}

		admitted = append(admitted, entry)
	}

	return admitted
}
