package tokenizer

import (
	"context"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/store"
)

const GrammarSymbolDir = "model/grammar/symbol/"

func GrammarObjectKey(symbol string) string {
	return GrammarSymbolDir + strings.ReplaceAll(symbol, "/", "-") + ".bin"
}

func (sequencer *Sequencer) Persist(ctx context.Context, client *store.Client, symbol string) {
	if sequencer == nil || client == nil || symbol == "" {
		return
	}

	value := data.NewValue("model", "grammar", symbol)
	value.Append(sequencer.State(), "state")

	if _, err := client.WriteAtPrefix(GrammarObjectKey(symbol), value.Bytes()); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "[sequencer] persist "+symbol, err))
	}
}

func (sequencer *Sequencer) RestoreFromStore(ctx context.Context, client *store.Client, symbol string) bool {
	if sequencer == nil || client == nil || symbol == "" {
		return false
	}

	value := client.Download(ctx, GrammarObjectKey(symbol))
	if value == nil || value.Err != nil {
		return false
	}

	var state SymbolState
	value.Lookup(&state, "state")
	if value.Err != nil {
		return false
	}

	sequencer.Restore(state)
	return true
}

func RestoreCatalog(
	ctx context.Context,
	client *store.Client,
	apply func(symbol string, state SymbolState),
) {
	if client == nil || apply == nil {
		return
	}

	for key := range client.ListKeys(GrammarSymbolDir) {
		if !strings.HasSuffix(key, ".bin") {
			continue
		}

		value := client.Download(ctx, key)
		if value == nil || value.Err != nil {
			continue
		}

		var state SymbolState
		value.Lookup(&state, "state")
		if value.Err != nil {
			continue
		}

		symbol := value.Scope
		if symbol == "" {
			name := strings.TrimSuffix(strings.TrimPrefix(key, GrammarSymbolDir), ".bin")
			symbol = strings.ReplaceAll(name, "-", "/")
		}
		apply(symbol, state)
	}
}
