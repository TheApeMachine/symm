package tokenizer

import (
	"context"
	"iter"
	"reflect"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
)

/*
grammarSymbolDir is the object-store prefix for per-symbol sequencer snapshots
written beside model/paths/ leaves — grammar survives independent of tree S3 keys.
*/
const grammarSymbolDir = "model/grammar/symbol/"

/*
grammarObjectStore is the minimal store surface GrammarCatalog needs so unit
tests pass memGrammarStore without linking the full nomagique client.
*/
type grammarObjectStore interface {
	WriteAtPrefix(key string, value []byte) (int, error)
	Download(ctx context.Context, key string) ([]byte, error)
	ListKeys(prefix string) iter.Seq[string]
}

/*
grammarStoreReady treats nil interface and nil pointer differently — a typed nil
client must not panic ListKeys during boot when paper mode has no store wired.
*/
func grammarStoreReady(client grammarObjectStore) bool {
	if client == nil {
		return false
	}

	val := reflect.ValueOf(client)

	return val.Kind() != reflect.Pointer || !val.IsNil()
}

/*
GrammarCatalog owns grammar persistence between Training and object storage;
keys slash-escape symbols because desk pairs look like BTC/USD on disk.
*/
type GrammarCatalog struct{}

/*
NewGrammarCatalog is stateless — one instance shares key rules across all symbols.
*/
func NewGrammarCatalog() *GrammarCatalog {
	return &GrammarCatalog{}
}

/*
ObjectKey maps a desk symbol to a filesystem-safe object name (slashes → dashes).
*/
func (catalog *GrammarCatalog) ObjectKey(symbol string) string {
	return grammarSymbolDir + strings.ReplaceAll(symbol, "/", "-") + ".bin"
}

/*
Symbol inverts ObjectKey after ListKeys so RestoreCatalog can fan sequencers per pair.
*/
func (catalog *GrammarCatalog) Symbol(key string) string {
	name := strings.TrimSuffix(strings.TrimPrefix(key, grammarSymbolDir), ".bin")
	return strings.ReplaceAll(name, "-", "/")
}

/*
Persist wraps SequencerState in a versioned envelope so future fields can grow
without breaking AttributeOk decode on older blobs.
*/
func (catalog *GrammarCatalog) Persist(
	ctx context.Context,
	client grammarObjectStore,
	symbol string,
	state SequencerState,
) error {
	if symbol == "" || !grammarStoreReady(client) {
		return nil
	}

	body, err := sonic.Marshal(map[string]any{"state": state})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "[tokenizer.catalog] marshal grammar", err,
		))
	}

	if _, err = client.WriteAtPrefix(catalog.ObjectKey(symbol), body); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "[tokenizer.catalog] store grammar", err,
		))
	}

	return nil
}

/*
RestoreCatalog walks grammarSymbolDir at boot and invokes apply per decoded state —
Training wires apply to restoreSequencer so cuts resume mid-sentence.
*/
func (catalog *GrammarCatalog) RestoreCatalog(
	ctx context.Context,
	client grammarObjectStore,
	apply func(symbol string, state SequencerState),
) {
	if !grammarStoreReady(client) || apply == nil {
		return
	}

	for key := range client.ListKeys(grammarSymbolDir) {
		if !strings.HasSuffix(key, ".bin") {
			continue
		}

		raw, err := client.Download(ctx, key)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[tokenizer.catalog] download grammar", err,
			))

			continue
		}

		if len(raw) == 0 {
			continue
		}

		state, err := data.AttributeOk[SequencerState](raw, "state")

		if err != nil {
			continue
		}

		apply(catalog.Symbol(key), state)
	}
}
