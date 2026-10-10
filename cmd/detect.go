package cmd

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/cobra"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
)

var (
	detectEpochs    []int64
	detectSymbols   []string
	detectFeePct    string
	detectCensus    bool
	detectCensusTop int
	detectDryRun    bool
	detectMinMove   time.Duration
)

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Label stored spot:trade tape with excursion detections (source=detector)",
	Long: `detect runs the excursion Detector over the stored spot:trade tape of each
requested epoch and symbol, under an explicit taker fee (percent per side), and
appends the resulting source=detector rows to the detections table
(hindsight.detections), never to the measurements table.

--min-move-duration is required: a detection whose B->C venue time is shorter
is dropped as a sweep artifact (one aggressive order printing through several
levels is not a move the system can enter and exit) and reported per class.

It is idempotent: detections already stored for an epoch are recognised by
their full coordinates (class, B/C ticks and indices, prices) and are not
written again. A stored detection for a requested symbol that this run does not
reproduce (different fee, different tape) is a conflict and aborts the epoch
before anything is written.

--census only reads: it prints the spot:trade row count per symbol.
--dry-run detects and prints every detection without appending.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		system.NewConfig()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tables.Open(ctx)

		if catalog == nil {
			return errnie.Error(errnie.Err(errnie.Validation, "[detect] unable to open iceberg catalog", nil))
		}

		if len(detectEpochs) == 0 {
			runs, err := catalog.Runs(ctx)

			if err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO,
					"[detect] failed to read recorded runs from catalog",
					err,
				))
			}

			for _, run := range runs {
				if run.Epoch > 0 && !slices.Contains(detectEpochs, run.Epoch) {
					detectEpochs = append(detectEpochs, run.Epoch)
				}
			}

			if len(detectEpochs) == 0 {
				return errnie.Error(errnie.Err(
					errnie.NotFound,
					"[detect] no completed epochs found in catalog",
					nil,
				))
			}

			slices.Sort(detectEpochs)
		}

		if detectCensus {
			return detectCensusReport(ctx, catalog)
		}

		if !cmd.Flags().Changed("min-move-duration") || detectMinMove < 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[detect] --min-move-duration is required (e.g. 1s; 0s keeps every detection)",
				nil,
			))
		}

		if len(detectSymbols) == 0 {
			symbolSet := make(map[string]struct{})
			epochsWithTrades := make([]int64, 0, len(detectEpochs))

			for _, epoch := range detectEpochs {
				counts, err := catalog.TradeCounts(ctx, epoch)

				if err != nil {
					return errnie.Error(err)
				}

				if len(counts) > 0 {
					epochsWithTrades = append(epochsWithTrades, epoch)

					for symbol := range counts {
						symbolSet[symbol] = struct{}{}
					}
				}
			}

			for symbol := range symbolSet {
				detectSymbols = append(detectSymbols, symbol)
			}

			slices.Sort(detectSymbols)

			if len(detectSymbols) == 0 {
				return errnie.Error(errnie.Err(
					errnie.NotFound,
					"[detect] no symbols found with stored trades in the specified epochs",
					nil,
				))
			}

			detectEpochs = epochsWithTrades
		}

		price, err := offlinePrice(ctx, detectFeePct, detectSymbols)

		if err != nil {
			return err
		}

		if !detectDryRun {
			if err := catalog.EnsureTable(ctx, tables.Detections); err != nil {
				return errnie.Error(err)
			}
		}

		for _, epoch := range detectEpochs {
			if err := detectEpoch(ctx, catalog, price, epoch); err != nil {
				return err
			}
		}

		return nil
	},
}

/*
offlinePrice builds the offline friction model shared by detect and train:
Kraken's public pair facts for lot normalization, the explicit taker fee
(percent per side) for every requested symbol, and the configured account
balance as reference cash for allocation sizing.
*/
func offlinePrice(ctx context.Context, feePercent string, symbols []string) (*broker.Price, error) {
	fee, err := decimal.NewFromString(feePercent)

	if err != nil || fee == nil || fee.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"--taker-fee-percent must be a positive decimal percent per side (e.g. 0.26)",
			err,
		))
	}

	if system.Cfg.Market.Balance <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "market.balance must be positive", nil))
	}

	normalizer := spot.NewNormalizer()

	if err := broker.SeedNormalizer(normalizer); err != nil {
		return nil, err
	}

	price := broker.NewPrice(ctx, nil, nil, nil, normalizer)

	for _, symbol := range symbols {
		price.SetFee(symbol, kraken.TradeVolumeFee{Fee: fee})

		if price.Fee(symbol) == nil {
			return nil, errnie.Error(errnie.Err(errnie.NotFound, "fee did not register for "+symbol, nil))
		}
	}

	price.SetReferenceCash(decimal.NewFromInt64(int64(system.Cfg.Market.Balance)))

	return price, nil
}

func detectEpoch(ctx context.Context, catalog *tables.Catalog, price *broker.Price, epoch int64) error {
	if epoch <= 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "[detect] epoch must be positive", nil))
	}

	existing := make(map[string]string)

	stored, err := catalog.HasTable(ctx, tables.Detections)

	if err != nil {
		return errnie.Error(err)
	}

	if !stored && !detectDryRun {
		return errnie.Error(errnie.Err(errnie.NotFound, "[detect] detections table does not exist", nil))
	}

	if stored {
		for measurement, err := range catalog.Detections(ctx, epoch) {
			if err != nil {
				return errnie.Error(err)
			}

			key, err := detectionKey(measurement)

			if err != nil {
				return err
			}

			existing[key] = measurement.Label
		}
	}

	tee := hindsight.NewStoreTee(ctx, "detect")
	tee.Transition(nmruntime.READY)

	if tee.Status() != nmruntime.READY {
		return errnie.Error(errnie.Err(errnie.Internal, "[detect] detection tee is not ready", nil))
	}

	detector := strategy.NewDetector(ctx, tee, price)

	trades := 0
	counting := func(yield func(*data.Measurement, error) bool) {
		for measurement, err := range catalog.Trades(ctx, epoch, detectSymbols...) {
			if err == nil && measurement != nil {
				trades++
			}

			if !yield(measurement, err) {
				return
			}
		}
	}

	if err := detector.Scan(counting); err != nil {
		return errnie.Error(err)
	}

	if trades == 0 {
		fmt.Printf("epoch=%d trades_scanned=0 (no trades to detect)\n", epoch)
		return nil
	}

	if dropped := tee.Dropped(); dropped != 0 {
		return errnie.Error(errnie.Err(
			errnie.Internal, fmt.Sprintf("[detect] epoch %d: %d detections were dropped by the tee", epoch, dropped), nil,
		))
	}

	detected := make([]*data.Measurement, 0, tee.Pending())
	sweeps := make(map[string]int)

	for _, measurement := range tee.Take() {
		if move := measurement.At.Sub(measurement.From); move < detectMinMove {
			sweeps[measurement.Meta("type")]++

			if detectDryRun {
				key, err := detectionKey(measurement)

				if err != nil {
					return err
				}

				fmt.Printf("sweep %s|move=%s\n", key, move)
			}

			continue
		}

		detected = append(detected, measurement)
	}

	reproduced := make(map[string]struct{}, len(detected))
	fresh := make([]*data.Measurement, 0, len(detected))

	for _, measurement := range detected {
		key, err := detectionKey(measurement)

		if err != nil {
			return err
		}

		reproduced[key] = struct{}{}

		if _, stored := existing[key]; !stored {
			fresh = append(fresh, measurement)
		}
	}

	for key, label := range existing {
		if !slices.Contains(detectSymbols, label) {
			continue
		}

		if _, ok := reproduced[key]; !ok {
			return errnie.Error(errnie.Err(
				errnie.Conflict,
				fmt.Sprintf("[detect] epoch %d: stored detection not reproduced by this run: %s", epoch, key),
				nil,
			))
		}
	}

	if detectDryRun {
		for _, measurement := range detected {
			key, err := detectionKey(measurement)

			if err != nil {
				return err
			}

			fmt.Printf("detection %s|move=%s\n", key, measurement.At.Sub(measurement.From))
		}

		fmt.Printf(
			"epoch=%d trades_scanned=%d detections=%d sweeps_removed=%v already_stored=%d would_append=%d (dry run)\n",
			epoch, trades, len(detected), sweeps, len(detected)-len(fresh), len(fresh),
		)

		return nil
	}

	if err := catalog.Append(ctx, tables.Detections, epoch, fresh); err != nil {
		return errnie.Error(err)
	}

	fmt.Printf(
		"epoch=%d trades_scanned=%d detections=%d sweeps_removed=%v already_stored=%d appended=%d\n",
		epoch, trades, len(detected), sweeps, len(detected)-len(fresh), len(fresh),
	)

	return nil
}

/*
detectionKey identifies a detection by every coordinate that defines it, so a
re-run recognises its own rows. Detector rows carry no fee, so a run under
another fee shows up as stored detections it does not reproduce (a conflict).
*/
func detectionKey(measurement *data.Measurement) (string, error) {
	class := measurement.Meta("type")
	startTick, bTick, cTick, err := tables.DetectionTicks(measurement)

	if err != nil {
		return "", err
	}

	bPrice, cPrice, err := tables.DetectionPrices(measurement)

	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"%s|%s|start=%d|b=%d|c=%d|bp=%s|cp=%s",
		measurement.Label, class, startTick, bTick, cTick,
		bPrice.String(), cPrice.String(),
	), nil
}

func detectCensusReport(ctx context.Context, catalog *tables.Catalog) error {
	for _, epoch := range detectEpochs {
		counts, err := catalog.TradeCounts(ctx, epoch)

		if err != nil {
			return errnie.Error(err)
		}

		type row struct {
			label string
			count int
		}

		rows := make([]row, 0, len(counts))
		total := 0

		for label, count := range counts {
			rows = append(rows, row{label, count})
			total += count
		}

		slices.SortFunc(rows, func(left, right row) int {
			if c := cmp.Compare(right.count, left.count); c != 0 {
				return c
			}

			return strings.Compare(left.label, right.label)
		})

		fmt.Printf("epoch=%d symbols=%d trades=%d\n", epoch, len(rows), total)

		for index, r := range rows {
			if detectCensusTop > 0 && index >= detectCensusTop {
				break
			}

			fmt.Printf("  %4d  %-14s %d\n", index+1, r.label, r.count)
		}
	}

	return nil
}

func init() {
	detectCmd.Flags().Int64SliceVar(&detectEpochs, "epoch", nil, "Run epoch to label (repeatable; default: all completed epochs)")
	detectCmd.Flags().StringSliceVar(&detectSymbols, "symbols", nil, "Symbols to label, e.g. BTC/USD,ETH/USD (default: all symbols observed in epochs)")
	detectCmd.Flags().StringVar(&detectFeePct, "taker-fee-percent", "", "Taker fee in percent per side that defines friction, e.g. 0.26 (required)")
	detectCmd.Flags().BoolVar(&detectCensus, "census", false, "Only print spot:trade counts per symbol for the epochs")
	detectCmd.Flags().IntVar(&detectCensusTop, "top", 0, "With --census, print only the N symbols with most trades (0 = all)")
	detectCmd.Flags().BoolVar(&detectDryRun, "dry-run", false, "Detect and print, but append nothing")
	detectCmd.Flags().DurationVar(&detectMinMove, "min-move-duration", 0, "Drop detections whose B->C venue time is shorter (sweep artifacts); required")
	rootCmd.AddCommand(detectCmd)
}
