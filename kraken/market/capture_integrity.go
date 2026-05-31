package market

import (
	"context"

	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/replay"
)

/*
CountCaptureBookDivergences scans a JSONL capture and returns how many symbols
first lose Kraken checksum alignment during sequential book playback.
*/
func CountCaptureBookDivergences(ctx context.Context, capturePath string) (int, error) {
	inbound, err := replay.ScanWSRows[BookUpdate](ctx, capturePath, public.BookChannel)

	if err != nil {
		return 0, err
	}

	books := map[string]*BookFeedState{}
	divergences := 0

	for update := range inbound {
		if update == nil {
			continue
		}

		state, ok := books[update.Symbol]

		if !ok {
			state = NewBookFeedState(update.Symbol, "capture-integrity", 25)
			books[update.Symbol] = state
		}

		wasDiverged := state.Diverged()

		if !state.Apply(*update) {
			continue
		}

		if state.Diverged() && !wasDiverged {
			divergences++
		}
	}

	return divergences, nil
}
