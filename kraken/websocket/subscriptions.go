package websocket

import (
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

/*
Reconnector is implemented by Live so API can replay subscription intent after
the SDK auto-reconnects. Mock and paper transports omit it.
*/
type Reconnector interface {
	OnReconnect(fn func())
}

type subscriptionIntent struct {
	mu          sync.Mutex
	instruments bool
	tickers     []string
	trades      []string
	books       []string
	balances    bool
	executions  bool
}

func (api *API) bindReconnect() {
	if api == nil {
		return
	}

	if reconnector, ok := api.public.(Reconnector); ok {
		reconnector.OnReconnect(api.replayPublic)
	}

	if reconnector, ok := api.private.(Reconnector); ok {
		reconnector.OnReconnect(api.replayPrivate)
	}
}

func (api *API) rememberSymbols(dest *[]string, pairs []string) {
	api.subs.mu.Lock()
	defer api.subs.mu.Unlock()

	seen := make(map[string]struct{}, len(*dest)+len(pairs))

	for _, symbol := range *dest {
		seen[symbol] = struct{}{}
	}

	for _, symbol := range pairs {
		if symbol == "" {
			continue
		}

		if _, ok := seen[symbol]; ok {
			continue
		}

		seen[symbol] = struct{}{}
		*dest = append(*dest, symbol)
	}
}

func (api *API) replayPublic() {
	api.subs.mu.Lock()
	instruments := api.subs.instruments
	tickers := append([]string(nil), api.subs.tickers...)
	trades := append([]string(nil), api.subs.trades...)
	books := append([]string(nil), api.subs.books...)
	api.subs.mu.Unlock()

	if instruments {
		errnie.Error(api.public.Client().SubInstruments())
	}

	if len(tickers) > 0 {
		errnie.Error(api.public.Client().SubTicker(tickers))
	}

	if len(trades) > 0 {
		errnie.Error(api.public.Client().SubTrades(
			trades,
			map[string]any{"params": map[string]any{"snapshot": true}},
		))
	}

	if len(books) > 0 {
		errnie.Error(api.public.Client().SubBook(
			books, viper.GetInt("market.book.depth"), nil,
		))
	}
}

func (api *API) replayPrivate() {
	api.subs.mu.Lock()
	balances := api.subs.balances
	executions := api.subs.executions
	api.subs.mu.Unlock()

	if !api.live {
		return
	}

	if balances {
		errnie.Error(api.private.Client().SubBalances())
	}

	if executions {
		errnie.Error(api.private.Client().SubExecutions(map[string]any{
			"params": map[string]any{
				"snap_orders": true,
				"snap_trades": true,
			},
		}))
	}
}

/*
ResyncBook forces a public book resnapshot for symbols after a local checksum
failure by unsubscribing then subscribing again.
*/
func (api *API) ResyncBook(pairs []string) error {
	if api == nil || api.public == nil || len(pairs) == 0 {
		return nil
	}

	if err := api.public.Write(kraken.NewBookUnsubscription(pairs)); err != nil {
		return err
	}

	return api.SubscribeBook(pairs)
}
