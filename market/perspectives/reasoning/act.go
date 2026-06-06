package reasoning

import "github.com/theapemachine/symm/kraken/trading"

/*
Act is the decision at a node. Offset lets a single node override the global
protective threshold for the trade it manages — a short scalp can carry a tight
stop and a long accumulation a wide one, which a global ReplayCosts.StopLossPct
cannot express (this is the per-action parameter the analysis correctly flagged).
The YAML accepts a bare action for the no-parameter case ("do: iceberg") or the
object form ("do: { type: stop_loss, offset: 0.015 }").
*/
type Act struct {
	Type     ActionType   `yaml:"type"`
	Side     trading.Side `yaml:"side,omitempty"`     // sell opens a short entry; buy is the default for entries
	Offset   float64      `yaml:"offset,omitempty"`   // overrides the global stop/take/trail fraction for this node (0 = use global)
	Fraction float64      `yaml:"fraction,omitempty"` // multiplier on trading.position_fraction for this entry (0 = deploy the global fraction)
}

/*
IsShortAct reports whether act opens a short position.
*/
func IsShortAct(act Act) bool {
	return IsEntryAction(act.Type) && act.Side == trading.Sell
}
