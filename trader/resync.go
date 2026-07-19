package trader

import (
	"time"

	"github.com/theapemachine/errnie"
)

const (
	bookResyncCooldown = time.Second
	bookResyncAttempts = 3
)

/*
scheduleBookResync enqueues one symbol for coalesced book resnapshot work.
*/
func (market *Market) scheduleBookResync(symbol string) {
	if market == nil || market.api == nil || market.resyncIn == nil || symbol == "" {
		return
	}

	select {
	case market.resyncIn <- symbol:
	case <-market.ctx.Done():
	default:
	}
}

/*
resync coalesces failed book symbols and retries ResyncBook with cooldown.
*/
func (market *Market) resync() {
	ctx := market.ctx

	for {
		select {
		case <-ctx.Done():
			return
		case symbol := <-market.resyncIn:
			pending := map[string]struct{}{symbol: {}}

		drain:
			for {
				select {
				case next := <-market.resyncIn:
					pending[next] = struct{}{}
				default:
					break drain
				}
			}

			symbols := make([]string, 0, len(pending))

			for pendingSymbol := range pending {
				symbols = append(symbols, pendingSymbol)
			}

			for attempt := 0; attempt < bookResyncAttempts; attempt++ {
				resyncErr := market.api.ResyncBook(symbols)

				if resyncErr == nil {
					break
				}

				errnie.Error(errnie.Err(
					errnie.Internal,
					"market: resync book snapshot",
					resyncErr,
				))

				select {
				case <-ctx.Done():
					return
				case <-time.After(bookResyncCooldown):
				}
			}
		}
	}
}
