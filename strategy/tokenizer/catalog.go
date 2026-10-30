package tokenizer

import (
	"context"
	"strings"

	"github.com/theapemachine/symm/store"
)

const grammarSymbolDir = "model/grammar/symbol/"

// GrammarCatalog reads and writes persisted sequencer state in object storage.
type GrammarCatalog struct{}

func (catalog *GrammarCatalog) ObjectKey(symbol string) string {
	return grammarSymbolDir + strings.ReplaceAll(symbol, "/", "-") + ".bin"
}

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

		value := client.Download(ctx, key)
		if value == nil || value.Err != nil {
			continue
		}

		var state SequencerState
		value.Lookup(&state, "state")
		if value.Err != nil {
			continue
		}

		symbol := value.Scope
		if symbol == "" {
			name := strings.TrimSuffix(strings.TrimPrefix(key, grammarSymbolDir), ".bin")
			symbol = strings.ReplaceAll(name, "-", "/")
		}

		apply(symbol, state)
	}
}
