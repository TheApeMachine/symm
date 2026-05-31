package trader

import (
	"fmt"

	"github.com/theapemachine/symm/kraken/market"
)

type bookHealthSink struct {
	crypto *Crypto
}

func (sink *bookHealthSink) BookHealth(event market.BookHealthEvent) {
	if sink.crypto == nil {
		return
	}

	if event.Recovered {
		sink.crypto.publishAudit(
			"book_recovered",
			event.Symbol,
			fmt.Sprintf("%s book resynced with Kraken", event.Signal),
			map[string]any{
				"signal":         event.Signal,
				"diverged_total": event.TotalDiverged,
			},
		)

		return
	}

	sink.crypto.publishAudit(
		"book_diverged",
		event.Symbol,
		fmt.Sprintf("%s book checksum diverged — signal blind until resync", event.Signal),
		map[string]any{
			"signal":         event.Signal,
			"diverged_total": event.TotalDiverged,
		},
	)
}
