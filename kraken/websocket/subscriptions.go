package websocket

import (
	"slices"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
)

/*
Reconnector is implemented by Live so API can replay subscription intent after
the SDK auto-reconnects. Mock and paper transports omit it.
*/
type Reconnector interface {
	OnReconnect(fn func() error)
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

/*
subscribeBatchSize returns the configured public subscription chunk size used
for both initial subscribe and reconnect replay.
*/
func (api *API) subscribeBatchSize() int {
	batchSize := viper.GetInt("market.subscribe_batch")

	if batchSize < 1 {
		return 1
	}

	return batchSize
}

/*
replayPublic re-subscribes remembered public channels in the same batch size
used for initial subscriptions and returns the first failure.
*/
func (api *API) replayPublic() error {
	api.subs.mu.Lock()
	instruments := api.subs.instruments
	tickers := append([]string(nil), api.subs.tickers...)
	trades := append([]string(nil), api.subs.trades...)
	books := append([]string(nil), api.subs.books...)
	api.subs.mu.Unlock()

	if instruments {
		if err := api.public.Client().SubInstruments(); err != nil {
			return err
		}
	}

	batchSize := api.subscribeBatchSize()

	for batch := range slices.Chunk(tickers, batchSize) {
		if err := api.public.Client().SubTicker(batch); err != nil {
			return err
		}
	}

	for batch := range slices.Chunk(trades, batchSize) {
		if err := api.public.Client().SubTrades(
			batch,
			map[string]any{"params": map[string]any{"snapshot": true}},
		); err != nil {
			return err
		}
	}

	depth := viper.GetInt("market.book.depth")

	for batch := range slices.Chunk(books, batchSize) {
		if err := api.public.Client().SubBook(batch, depth, nil); err != nil {
			return err
		}
	}

	return nil
}

/*
replayPrivate re-subscribes remembered private channels and returns the first
failure so reconnect cannot mark READY with a partial private surface.
*/
func (api *API) replayPrivate() error {
	api.subs.mu.Lock()
	balances := api.subs.balances
	executions := api.subs.executions
	api.subs.mu.Unlock()

	if !api.live {
		return nil
	}

	if balances {
		if err := api.private.Client().SubBalances(); err != nil {
			return err
		}
	}

	if executions {
		if err := api.private.Client().SubExecutions(map[string]any{
			"params": map[string]any{
				"snap_orders": true,
				"snap_trades": true,
			},
		}); err != nil {
			return err
		}
	}

	return nil
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
