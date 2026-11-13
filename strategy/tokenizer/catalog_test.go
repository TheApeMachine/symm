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

func TestNewGrammarCatalog(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewGrammarCatalog is called", func() {
			catalog := NewGrammarCatalog()

			Convey("ObjectKey round-trips symbols", func() {
				So(catalog.ObjectKey("ETH/USD"), ShouldEqual, grammarSymbolDir+"ETH-USD.bin")
				So(catalog.Symbol(grammarSymbolDir+"BTC-USD.bin"), ShouldEqual, "BTC/USD")
			})
		})
	})
}

func TestObjectKey(t *testing.T) {
	Convey("Setup", t, func() {
		catalog := NewGrammarCatalog()

		Convey("When ObjectKey encodes a symbol", func() {
			Convey("Key uses grammar prefix and escaped slash", func() {
				So(catalog.ObjectKey("BTC/USD"), ShouldEqual, grammarSymbolDir+"BTC-USD.bin")
			})
		})
	})
}

func TestSymbol(t *testing.T) {
	Convey("Setup", t, func() {
		catalog := NewGrammarCatalog()

		Convey("When Symbol decodes an object key", func() {
			Convey("Symbol restores the trading pair", func() {
				So(catalog.Symbol(grammarSymbolDir+"BTC-USD.bin"), ShouldEqual, "BTC/USD")
			})
		})
	})
}

func TestPersist(t *testing.T) {
	Convey("Setup", t, func() {
		catalog := NewGrammarCatalog()
		state := NewSequencer().State()

		Convey("When client or symbol is missing", func() {
			Convey("Persist is a no-op", func() {
				So(catalog.Persist(t.Context(), nil, "BTC/USD", state), ShouldBeNil)
				So(catalog.Persist(t.Context(), nil, "", state), ShouldBeNil)
			})
		})

		Convey("When grammar is written to a mem store", func() {
			backing := &memGrammarStore{}
			seq := NewSequencer()
			_, mutated := seq.Step("R03", 0.7)
			So(mutated, ShouldBeTrue)

			err := catalog.Persist(t.Context(), backing, "BTC/USD", seq.State())

			key := catalog.ObjectKey("BTC/USD")
			raw, dlErr := backing.Download(t.Context(), key)
			restored, attrErr := data.AttributeOk[SequencerState](raw, "state")

			Convey("JSON round-trips root length", func() {
				So(err, ShouldBeNil)
				So(dlErr, ShouldBeNil)
				So(attrErr, ShouldBeNil)
				So(len(restored.Root), ShouldEqual, len(seq.State().Root))
			})
		})
	})
}

func TestRestoreCatalog(t *testing.T) {
	Convey("Setup", t, func() {
		backing := &memGrammarStore{}
		catalog := NewGrammarCatalog()

		btc := NewSequencer()
		_, _ = btc.Step("R02", 0.5)

		eth := NewSequencer()
		_, _ = eth.Step("R04", 0.6)

		So(catalog.Persist(t.Context(), backing, "BTC/USD", btc.State()), ShouldBeNil)
		So(catalog.Persist(t.Context(), backing, "ETH/USD", eth.State()), ShouldBeNil)

		seen := make(map[string]SequencerState)

		Convey("When RestoreCatalog lists persisted keys", func() {
			catalog.RestoreCatalog(t.Context(), backing, func(symbol string, state SequencerState) {
				seen[symbol] = state
			})

			Convey("Every symbol is replayed", func() {
				So(len(seen), ShouldEqual, 2)
				So(len(seen["BTC/USD"].Root), ShouldEqual, len(btc.State().Root))
				So(len(seen["ETH/USD"].Root), ShouldEqual, len(eth.State().Root))
			})
		})
	})
}
