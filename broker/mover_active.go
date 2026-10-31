package broker

import (
	"fmt"

	"github.com/theapemachine/symm/system"
)

/*
Active reports whether the symbol clears the mover pre-filter: enough
Ticker 24h quote volume, a day move off the open, and a tight touch spread.
*/
func (price *Price) Active(symbol string) bool {
	return price.ActiveReason(symbol) == ""
}

/*
ActiveReason is empty when Active, otherwise the concrete gate that failed.
*/
func (price *Price) ActiveReason(symbol string) string {
	if price == nil || symbol == "" {
		return "no price"
	}

	volume := 0.0
	change := 0.0

	if price.Mover != nil {
		volume = price.Mover.VolumeUSD(symbol)
		change = price.Mover.ChangePct(symbol)
	}

	floor := system.Cfg.Risk.MinMoverVolumeUSD

	if volume < floor {
		return fmt.Sprintf(
			"24h volume $%.0f < $%.0f",
			volume, floor,
		)
	}

	minChange := system.Cfg.Risk.MinMoverChangePct

	if change < minChange {
		return fmt.Sprintf(
			"day change %.2f%% < %.2f%%",
			change, minChange,
		)
	}

	spread, ok := price.SpreadPct(symbol)
	if !ok || spread == nil {
		return "no touch spread"
	}

	spreadCap := system.Cfg.Risk.MaxEntrySpreadPct

	if spread.Float64() > spreadCap {
		return fmt.Sprintf(
			"spread %.2f%% > %.1f%%",
			spread.Float64(), spreadCap,
		)
	}

	return ""
}
