package market

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/client"
	kmarket "github.com/theapemachine/symm/kraken/market"
)

/*
Pairs watches the Kraken v2 instrument channel and exposes filtered tradeable pairs.
*/
type Pairs struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	public  *client.PublicClient
	quote   string
	mu      sync.RWMutex
	index   map[string]kmarket.Instrument
	symbols []asset.Pair
	names   []string
	ready   bool
	loaded  chan struct{}
}

/*
NewPairs subscribes on the shared public websocket and registers the instrument handler.
*/
func NewPairs(
	parent context.Context,
	quote string,
	publicClient *client.PublicClient,
) (*Pairs, error) {
	ctx, cancel := context.WithCancel(parent)

	if publicClient == nil {
		cancel()
		return nil, fmt.Errorf("public websocket client is nil")
	}

	params := kmarket.SubscribeParams{}.Instrument()

	if err := publicClient.SubscribeTo(params); err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe instrument channel: %w", err)
	}

	pairs := &Pairs{
		ctx:    ctx,
		cancel: cancel,
		public: publicClient,
		quote:  quote,
		index:  make(map[string]kmarket.Instrument),
		loaded: make(chan struct{}),
	}

	publicClient.OnFrame(pairs.handleFrame)

	return pairs, nil
}

/*
GetAll waits for the instrument snapshot and returns filtered pairs.
*/
func (pairs *Pairs) GetAll(ctx context.Context) ([]asset.Pair, error) {
	if err := pairs.wait(ctx); err != nil {
		return nil, err
	}

	pairs.mu.RLock()
	defer pairs.mu.RUnlock()

	return pairs.symbols, nil
}

/*
Names waits for the instrument snapshot and returns websocket symbols.
*/
func (pairs *Pairs) Names(ctx context.Context) ([]string, error) {
	if err := pairs.wait(ctx); err != nil {
		return nil, err
	}

	pairs.mu.RLock()
	defer pairs.mu.RUnlock()

	return pairs.names, nil
}

/*
Observe returns the loaded pair set for signal composition.
*/
func (pairs *Pairs) Observe(_ context.Context) (engine.Observation, error) {
	pairs.mu.RLock()
	defer pairs.mu.RUnlock()

	if !pairs.ready {
		return engine.Observation{}, fmt.Errorf("pairs not loaded")
	}

	return engine.Observation{
		Pairs:     pairs.symbols,
		Timestamp: time.Now().UnixNano(),
	}, nil
}

func (pairs *Pairs) handleFrame(_ context.Context, payload []byte) error {
	var instrumentMessage kmarket.InstrumentMessage

	if err := instrumentMessage.Parse(payload); err != nil {
		if errors.Is(err, kmarket.ErrNotInstrument) {
			return nil
		}

		pairs.err = err
		return err
	}

	pairs.apply(instrumentMessage)

	return nil
}

func (pairs *Pairs) apply(instrumentMessage kmarket.InstrumentMessage) {
	pairs.mu.Lock()
	defer pairs.mu.Unlock()

	if instrumentMessage.Type == kmarket.InstrumentUpdateTypeSnapshot {
		pairs.index = make(map[string]kmarket.Instrument)
	}

	for _, instrument := range instrumentMessage.Data.Pairs {
		if !pairs.keep(instrument) {
			delete(pairs.index, instrument.Symbol)
			continue
		}

		pairs.index[instrument.Symbol] = instrument
	}

	pairs.rebuild()

	if pairs.ready {
		return
	}

	if instrumentMessage.Type != kmarket.InstrumentUpdateTypeSnapshot {
		return
	}

	pairs.ready = true
	close(pairs.loaded)
}

func (pairs *Pairs) keep(instrument kmarket.Instrument) bool {
	if instrument.Status != kmarket.PairStatusOnline {
		return false
	}

	if pairs.quote != "" && instrument.Quote != pairs.quote {
		return false
	}

	return true
}

func (pairs *Pairs) rebuild() {
	pairs.symbols = make([]asset.Pair, 0, len(pairs.index))
	pairs.names = make([]string, 0, len(pairs.index))

	for _, instrument := range pairs.index {
		pairs.symbols = append(pairs.symbols, instrument.AssetPair())
		pairs.names = append(pairs.names, instrument.Symbol)
	}

	sort.Strings(pairs.names)
	sort.Slice(pairs.symbols, func(left, right int) bool {
		return pairs.symbols[left].Wsname < pairs.symbols[right].Wsname
	})
}

func (pairs *Pairs) wait(ctx context.Context) error {
	select {
	case <-pairs.loaded:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-pairs.ctx.Done():
		return pairs.ctx.Err()
	}
}
