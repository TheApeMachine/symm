package cmd

import (
	"io"
	"os"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/cobra"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
)

var (
	trainEpochs  []int64
	trainSymbols []string
	trainFeePct  string
)

var trainCmd = &cobra.Command{
	Use:   "train",
	Short: "Train the action tree and publish its prefixes",
	Long: `train reads only detections/ (committed labels), never draft/ or
detector/, plus workspace ticks for each excursion. Precursor A→B and holding
B→C are replayed separately: each tick in the window through its own grid
and Sequitur. Regions at or above the brightness midpoint become the chord.
When the up round trip from B to C clears friction, the precursor phrase is
enter.bin and the holding phrase is exit.bin. Anything else is left
unmatched. An exit is never published without that excursion's enter. A
live sentence ends when two regimes describe its metric energy in a
shorter code than one regime.
The previous policy/current tree is moved to policy/archive/ first.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		normalizer := spot.NewNormalizer()
		books := broker.NewBook(ctx, normalizer)
		instrument := broker.NewInstrument(ctx, nil)

		price := broker.NewPrice(
			ctx, books, nil, instrument, normalizer,
		)

		balance := broker.NewBalance(ctx, nil)
		desk := broker.NewDesk(ctx, nil, price, balance)
		client := store.NewClient(ctx, "symm")
		hub := io.Writer(os.Stdout)

		training := strategy.NewTraining(
			ctx, price, desk, client, hub,
		)

		return strategy.NewTrainer(
			ctx, training, client, price, desk,
		).Generate()
	},
}
