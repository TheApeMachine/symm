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
harness lease. PeekBook and Books delegate here so conn.go stays focused on
public/private routing.
*/
type Level3Registry struct {
	conns *sync.Map
}

/*
NewLevel3Registry constructs an empty Level3 transport index.
*/
func NewLevel3Registry() *Level3Registry {
	return &Level3Registry{conns: &sync.Map{}}
}

/*
Attach registers one Level3 Live transport under key.
*/
func (registry *Level3Registry) Attach(key string, live *Live) {
	if registry == nil || key == "" || live == nil || live.books == nil {
		return
	}

	registry.conns.Store(key, live)
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
		live.Close()
	}

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
PeekBook invokes fn under the Level3 read lease for symbol.
*/
func (registry *Level3Registry) PeekBook(
	symbol string,
	fn func(*book.Book),
) bool {
	if registry == nil || fn == nil || symbol == "" {
		return false
	}

	found := false

	registry.conns.Range(func(_, value any) bool {
		live := value.(*Live)

		if !live.peekBook(symbol, fn) {
			return true
		}

		found = true

		return false
	})

	return found
}
