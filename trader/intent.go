package trader

import (
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
orderIntent captures decision-time context for one live cl_ord_id until the
exchange reports a trade fill.
*/
type orderIntent struct {
	kind           string
	symbol         string
	playbook       string
	notional       float64
	quote          broker.Quote
	feePct         float64
	spreadBPS      float64
	score          float64
	names          []string
	trigger        perspectives.Measurement
	entryPrice     float64
	exitReason     string
	lotDecimals    int
	hasLotDecimals bool
	predictedAt    time.Time
}
