package websocket

import (
	"context"
	"iter"
	"slices"
	"strings"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

const level3MaxSymbolsPerConnection = 200

/*
Level3Registry owns SDK BookManager transports keyed by subscription batch or
injected transport. PeekBook and Books delegate here so conn.go stays focused on
public/private routing. A symbol index maps each subscribed symbol to its owning
transport so PeekBook resolves in O(1) instead of ranging every connection.
*/
type Level3Registry struct {
	conns *sync.Map
	index *sync.Map
}

/*
NewLevel3Registry constructs an empty Level3 transport index.
*/
func NewLevel3Registry() *Level3Registry {
	return &Level3Registry{conns: &sync.Map{}, index: &sync.Map{}}
}

/*
Attach registers one Level3 Live transport under key and indexes its symbols so
subsequent PeekBook reads resolve directly to this transport. A prior Live on
the same key is unindexed so registry.index cannot retain stale owners.
*/
func (registry *Level3Registry) Attach(key string, live *Live) {
	if registry == nil || key == "" || live == nil || live.books == nil {
		return
	}

	if previous, loaded := registry.conns.Load(key); loaded {
		if old, ok := previous.(*Live); ok && old != live {
			registry.unindexSymbols(old)
		}
	}

	registry.conns.Store(key, live)
	registry.indexSymbols(live)
}

/*
Detach removes one Level3 transport, clears its symbol index entries, and closes
the Live so PeekBook cannot resolve books through a dead connection.
*/
func (registry *Level3Registry) Detach(key string) {
	if registry == nil || key == "" {
		return
	}

	value, ok := registry.conns.LoadAndDelete(key)

	if !ok {
		return
	}

	live, ok := value.(*Live)

	if !ok || live == nil {
		return
	}

	registry.unindexSymbols(live)
	live.Close()
}

/*
Close detaches every Level3 transport owned by the registry.
*/
func (registry *Level3Registry) Close() {
	if registry == nil {
		return
	}

	registry.conns.Range(func(key, _ any) bool {
		if text, ok := key.(string); ok {
			registry.Detach(text)
		}

		return true
	})
}

/*
indexSymbols records each of a transport's subscribed symbols in the O(1) lookup
index. Books created after subscription resolve lazily through scanBook.
*/
func (registry *Level3Registry) indexSymbols(live *Live) {
	if registry == nil || live == nil {
		return
	}

	for _, symbol := range live.symbols {
		if symbol == "" {
			continue
		}

		registry.index.Store(symbol, live)
	}
}

/*
unindexSymbols drops index entries that still point at live so a closed or
replaced transport cannot keep the map growing with stale owners.
*/
func (registry *Level3Registry) unindexSymbols(live *Live) {
	if registry == nil || live == nil {
		return
	}

	for _, symbol := range live.symbols {
		if symbol == "" {
			continue
		}

		if current, ok := registry.index.Load(symbol); ok && current == live {
			registry.index.Delete(symbol)
		}
	}
}

/*
Subscribe registers one Level3 transport for key when absent and initializes it.
*/
func (registry *Level3Registry) Subscribe(
	ctx context.Context,
	key string,
	symbols []string,
) error {
	if registry == nil || key == "" {
		return nil
	}

	if _, loaded := registry.conns.Load(key); loaded {
		return nil
	}

	live := New(ctx, nil, true, Level3WebSocketURL)
	live.symbols = symbols

	if err := live.Initialize(); err != nil {
		live.Close()
		return err
	}

	_, loaded := registry.conns.LoadOrStore(key, live)

	if loaded {
		// Never indexed; close the unused transport without touching the index.
		live.Close()
		return nil
	}

	registry.indexSymbols(live)

	return nil
}

/*
SubscribeAll assigns each symbol batch its own authenticated book transport.
*/
func (registry *Level3Registry) SubscribeAll(ctx context.Context, pairs []string) error {
	if registry == nil {
		return nil
	}

	batchSize, err := level3BatchSize()

	if err != nil {
		return err
	}

	for batch := range slices.Chunk(pairs, batchSize) {
		key := strings.Join(batch, "|")

		if err := registry.Subscribe(ctx, key, batch); err != nil {
			return err
		}
	}

	return nil
}

/*
level3BatchSize derives the number of symbols that fit in one Kraken L3
subscription request from the configured client-tier budget and book depth.
*/
func level3BatchSize() (int, error) {
	depth := viper.GetInt("market.l3_depth")
	rateLimit := viper.GetInt("market.l3_rate_limit")
	rateCost := map[int]int{
		10:   5,
		100:  25,
		1000: 100,
	}[depth]

	if rateCost == 0 || rateLimit < rateCost {
		return 0, errnie.Err(
			errnie.Validation,
			"websocket: L3 depth and rate limit cannot admit one symbol",
			nil,
		)
	}

	return min(rateLimit/rateCost, level3MaxSymbolsPerConnection), nil
}

/*
Books yields the SDK BookManager owned by each Level3 transport.
*/
func (registry *Level3Registry) Books() iter.Seq[*spot.BookManager] {
	return func(yield func(*spot.BookManager) bool) {
		if registry == nil {
			return
		}

		registry.conns.Range(func(key, value any) bool {
			live := value.(*Live)
			keepGoing := yield(live.books)

			return keepGoing
		})
	}
}

/*
PeekBook invokes fn under the Level3 read lease for symbol, resolving the owning
transport through the symbol index first and falling back to a full scan only
when the symbol was never indexed (book created after subscription). An indexed
Live that simply has no book yet must not be deleted or scanned — that thrash
turns every touchReady miss into an O(conns) walk and collapses tick rate.
Stale owners are removed by Detach/unindexSymbols on close or replacement.
*/
func (registry *Level3Registry) PeekBook(
	symbol string,
	fn func(*book.Book),
) bool {
	if registry == nil || fn == nil || symbol == "" {
		return false
	}

	if value, ok := registry.index.Load(symbol); ok {
		live, valid := value.(*Live)

		if !valid || live == nil {
			registry.index.Delete(symbol)
		} else {
			return live.peekBook(symbol, fn)
		}
	}

	return registry.scanBook(symbol, fn)
}

/*
scanBook ranges every transport for one symbol's book and caches the resolving
transport in the symbol index so later reads take the O(1) path.
*/
func (registry *Level3Registry) scanBook(
	symbol string,
	fn func(*book.Book),
) bool {
	found := false

	registry.conns.Range(func(_, value any) bool {
		live := value.(*Live)

		if !live.peekBook(symbol, fn) {
			return true
		}

		registry.index.Store(symbol, live)
		found = true

		return false
	})

	return found
}
