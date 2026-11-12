package tokenizer

import (
	"context"
	"errors"
	"iter"
	"strings"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/data"
)

type memGrammarStore struct {
	mu   sync.Mutex
	blob map[string][]byte
}

func (store *memGrammarStore) WriteAtPrefix(key string, value []byte) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.blob == nil {
		store.blob = make(map[string][]byte)
	}

	store.blob[key] = append([]byte(nil), value...)

	return len(value), nil
}

func (store *memGrammarStore) Download(_ context.Context, key string) ([]byte, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	raw, ok := store.blob[key]

	if !ok {
		return nil, errors.New("missing key")
	}

	return append([]byte(nil), raw...), nil
}

func (store *memGrammarStore) ListKeys(prefix string) iter.Seq[string] {
	return func(yield func(string) bool) {
		store.mu.Lock()
		keys := make([]string, 0, len(store.blob))

		for key := range store.blob {
			if strings.HasPrefix(key, prefix) {
				keys = append(keys, key)
			}
		}

		store.mu.Unlock()

		for _, key := range keys {
			if !yield(key) {
				return
			}
		}
	}
}

func TestGrammarCatalogSymbol(t *testing.T) {
	Convey("The symbol is read from the object key", t, func() {
		catalog := &GrammarCatalog{}

		So(catalog.Symbol(grammarSymbolDir+"BTC-USD.bin"), ShouldEqual, "BTC/USD")
		So(catalog.ObjectKey("ETH/USD"), ShouldEqual, grammarSymbolDir+"ETH-USD.bin")
	})
}

func TestGrammarCatalogPersistNoOp(t *testing.T) {
	Convey("Persist skips when client or symbol is missing", t, func() {
		catalog := NewGrammarCatalog()
		state := NewSequencer().State()

		So(catalog.Persist(t.Context(), nil, "BTC/USD", state), ShouldBeNil)
		So(catalog.Persist(t.Context(), nil, "", state), ShouldBeNil)
	})
}

func TestGrammarCatalogPersistRoundTrip(t *testing.T) {
	Convey("Persist writes grammar JSON the store can reload", t, func() {
		backing := &memGrammarStore{}
		catalog := NewGrammarCatalog()
		seq := NewSequencer()
		_, mutated := seq.Step("R03", 0.7)
		So(mutated, ShouldBeTrue)

		err := catalog.Persist(t.Context(), backing, "BTC/USD", seq.State())
		So(err, ShouldBeNil)

		key := catalog.ObjectKey("BTC/USD")
		raw, err := backing.Download(t.Context(), key)
		So(err, ShouldBeNil)

		restored, err := data.AttributeOk[SequencerState](raw, "state")
		So(err, ShouldBeNil)
		So(len(restored.Root), ShouldEqual, len(seq.State().Root))
	})
}

func TestGrammarCatalogRestoreCatalog(t *testing.T) {
	Convey("RestoreCatalog applies every grammar object under the prefix", t, func() {
		backing := &memGrammarStore{}
		catalog := NewGrammarCatalog()

		btc := NewSequencer()
		_, _ = btc.Step("R02", 0.5)

		eth := NewSequencer()
		_, _ = eth.Step("R04", 0.6)

		So(catalog.Persist(t.Context(), backing, "BTC/USD", btc.State()), ShouldBeNil)
		So(catalog.Persist(t.Context(), backing, "ETH/USD", eth.State()), ShouldBeNil)

		seen := make(map[string]SequencerState)

		catalog.RestoreCatalog(t.Context(), backing, func(symbol string, state SequencerState) {
			seen[symbol] = state
		})

		So(len(seen), ShouldEqual, 2)
		So(len(seen["BTC/USD"].Root), ShouldEqual, len(btc.State().Root))
		So(len(seen["ETH/USD"].Root), ShouldEqual, len(eth.State().Root))
	})
}
