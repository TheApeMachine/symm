package tokenizer

import (
	"context"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/store"
)

const grammarSymbolDir = "model/grammar/symbol/"

/*
GrammarCatalog reads and writes persisted sequencer state in object storage.
*/
type GrammarCatalog struct{}

/*
NewGrammarCatalog initializes a new grammar catalog.
*/
func NewGrammarCatalog() *GrammarCatalog {
	return &GrammarCatalog{}
}

/*
ObjectKey returns the object key for the given symbol.
*/
func (catalog *GrammarCatalog) ObjectKey(symbol string) string {
	return grammarSymbolDir + strings.ReplaceAll(symbol, "/", "-") + ".bin"
}

/*
Symbol reads the pair out of an object key under the grammar prefix.
*/
func (catalog *GrammarCatalog) Symbol(key string) string {
	name := strings.TrimSuffix(strings.TrimPrefix(key, grammarSymbolDir), ".bin")
	return strings.ReplaceAll(name, "-", "/")
}

/*
Persist writes one sequencer as JSON {"state":...}.
*/
func (catalog *GrammarCatalog) Persist(
	ctx context.Context,
	client *store.Client,
	symbol string,
	state SequencerState,
) error {
	if client == nil || symbol == "" {
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
RestoreCatalog restores the catalog from the given client and applies the given function to each symbol.
*/
func (catalog *GrammarCatalog) RestoreCatalog(
	ctx context.Context,
	client *store.Client,
	apply func(symbol string, state SequencerState),
) {
	if client == nil || apply == nil {
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
