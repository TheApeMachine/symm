package tables

import (
	"bytes"
	"context"
	"iter"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
scan reads one table, optionally restricted to a single run, and yields its
record batches. Batches are borrowed for the duration of each yield; retained
values must be copied before advancing the iterator.

Iceberg guarantees no row order: a scan returns files in plan order and rows in
file order. Callers that need capture order sort explicitly rather than relying
on the layout, because a compaction or a re-append would silently change it.
*/
func (catalog *Catalog) scan(
	ctx context.Context, name string, fields []string, filters ...iceberg.BooleanExpression,
) (iter.Seq2[arrow.RecordBatch, error], error) {
	loaded, err := catalog.Load(ctx, name)

	if err != nil {
		return nil, err
	}

	predicate := iceberg.BooleanExpression(iceberg.AlwaysTrue{})

	for _, filter := range filters {
		predicate = iceberg.NewAnd(predicate, filter)
	}

	options := []icetable.ScanOption{icetable.WithRowFilter(predicate)}

	if len(fields) > 0 {
		options = append(options, icetable.WithSelectedFields(fields...))
	}
	tasks, err := loaded.Scan(options...).PlanFiles(ctx)

	if err != nil {
		return nil, errnie.Error(err)
	}
	batchSize := int64(loaded.Metadata().Properties().GetInt(
		icetable.ParquetBatchSizeKey, icetable.ParquetBatchSizeDefault,
	))

	if batchSize <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation,
			"[iceberg] Parquet read batch size must be positive", nil))
	}

	for _, task := range tasks {
		if len(fields) > 0 && !slices.Contains(fields, "payload") {
			break
		}
		bound, err := catalog.payloadBound(ctx, loaded, task)

		if err != nil {
			return nil, errnie.Error(err)
		}

		if bound > 0 {
			// A column chunk bounds one flat payload, including dictionary
			// entries. It does not bound a decoded batch of repeated entries.
			batchSize = min(batchSize, max(1, math.MaxInt32/bound))
			break
		}
	}

	// Iceberg 0.6 reads batch size from table metadata, not scan options.
	// Apply it to this scan's metadata copy without committing a table change.
	metadata, err := icetable.MetadataBuilderFromBase(loaded.Metadata(), loaded.MetadataLocation())

	if err != nil {
		return nil, errnie.Error(err)
	}

	if err := metadata.SetProperties(iceberg.Properties{
		icetable.ParquetBatchSizeKey: strconv.FormatInt(batchSize, 10),
	}); err != nil {
		return nil, errnie.Error(err)
	}
	bounded, err := metadata.Build()

	if err != nil {
		return nil, errnie.Error(err)
	}
	reader := icetable.New(loaded.Identifier(), bounded, loaded.MetadataLocation(), loaded.FS, nil)
	_, batches, err := reader.Scan(options...).ReadTasks(ctx, tasks)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to scan "+name,
			err,
		))
	}

	return func(yield func(arrow.RecordBatch, error) bool) {
		for batch, err := range batches {
			more := yield(batch, err)

			if batch != nil {
				batch.Release()
			}

			if !more {
				return
			}
		}
	}, nil
}

// forRun restricts a scan to one run.
func forRun(run string) iceberg.BooleanExpression {
	return iceberg.EqualTo(iceberg.Reference("run"), run)
}

// str reads an optional string column, returning "" for null.
func str(column arrow.Array, row int) string {
	if column.IsNull(row) {
		return ""
	}

	return strings.Clone(column.(*array.String).Value(row))
}

// num reads an optional int64 column, returning 0 for null.
func num(column arrow.Array, row int) int64 {
	if column.IsNull(row) {
		return 0
	}

	return column.(*array.Int64).Value(row)
}

// when reads an optional timestamp column as a UTC time, zero for null.
func when(column arrow.Array, row int) (value arrow.Timestamp, ok bool) {
	if column.IsNull(row) {
		return 0, false
	}

	return column.(*array.Timestamp).Value(row), true
}

// bin reads an optional binary column, returning nil for null.
func bin(column arrow.Array, row int) []byte {
	if column.IsNull(row) {
		return nil
	}

	return bytes.Clone(column.(*array.Binary).Value(row))
}

// rawBin borrows the Arrow binary value. Valid only until the batch is released.
func rawBin(column arrow.Array, row int) []byte {
	if column.IsNull(row) {
		return nil
	}

	return column.(*array.Binary).Value(row)
}

/*
Captures returns a run's captures strictly after the consumed sequence, sorted
in capture order. Sequences start at one; after zero reads the whole run.
Both predicates reach Iceberg before reading payloads, so tail followers avoid
reloading consumed files. Only the selected suffix is collected for sorting.
*/
func (c *Catalog) Captures(ctx context.Context, run string, after int64) ([]CaptureRow, error) {
	return c.captures(ctx, run, iceberg.GreaterThan(iceberg.Reference("sequence"), after))
}

/*
MarketKinds are the capture kinds that carry a market fact. Every other kind —
book and depth frames above all, which are the overwhelming majority of a run —
decodes to no observation at all.
*/
var MarketKinds = []string{"ticker", "trade", "trade_snapshot", "l3_touch"}

func (c *Catalog) captures(ctx context.Context, run string, filters ...iceberg.BooleanExpression) ([]CaptureRow, error) {
	rows := []CaptureRow{}

	if err := c.eachCapture(ctx, run, func(row CaptureRow) error {
		rows = append(rows, row)

		return nil
	}, filters...); err != nil {
		return nil, err
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Sequence < rows[j].Sequence })

	return rows, nil
}

/*
EachMarketCapture hands every capture that can carry a market fact to fn, one
at a time, without ever holding the run.

Collecting first is what a whole-run read cannot afford: a few minutes of one
run is over a million captures, and it is their payloads — book frames above
all — that make the collection gigabytes rather than megabytes. Streaming them
past a decoder keeps only what the decoder kept.

Rows arrive in the order Iceberg planned them, not in capture order. Nothing
here can sort without holding the run, which is precisely the cost being
avoided, so ordering is the caller's to apply to whatever it derives — which
is smaller than the payloads it derived them from.
*/
func (c *Catalog) EachMarketCapture(
	ctx context.Context, run string, after int64, fn func(CaptureRow) error,
) error {
	return c.eachCapture(
		ctx, run, fn,
		iceberg.GreaterThan(iceberg.Reference("sequence"), after),
		iceberg.IsIn(iceberg.Reference("kind"), MarketKinds...),
	)
}

func (c *Catalog) eachCapture(
	ctx context.Context, run string, fn func(CaptureRow) error, filters ...iceberg.BooleanExpression,
) error {
	batches, err := c.scan(ctx, Captures, nil, append(filters, forRun(run))...)

	if err != nil {
		return err
	}

	for batch, err := range batches {
		if err != nil {
			return errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] captures batch", err))
		}

		for index := range int(batch.NumRows()) {
			row := CaptureRow{
				Run:            str(batch.Column(0), index),
				Sequence:       num(batch.Column(1), index),
				Stream:         str(batch.Column(2), index),
				StreamEpoch:    num(batch.Column(3), index),
				StreamSequence: num(batch.Column(4), index),
				Endpoint:       str(batch.Column(6), index),
				Kind:           str(batch.Column(7), index),
				PayloadHash:    str(batch.Column(8), index),
				Payload:        bin(batch.Column(9), index),
			}

			if micros, ok := when(batch.Column(5), index); ok {
				row.ReceivedAt = micros.ToTime(arrow.Microsecond).UTC()
			}

			if err := fn(row); err != nil {
				return err
			}
		}
	}

	return nil
}

// Runs yields every recorded process capture session, newest first.
func (c *Catalog) Runs(ctx context.Context) ([]RunRow, error) {
	batches, err := c.scan(ctx, Runs, nil)

	if err != nil {
		return nil, err
	}

	rows := []RunRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] runs batch", err))
		}

		for index := range int(batch.NumRows()) {
			row := RunRow{
				ID:           str(batch.Column(0), index),
				CodeCommit:   str(batch.Column(2), index),
				BuildID:      str(batch.Column(3), index),
				ConfigDigest: str(batch.Column(4), index),
				Integrity:    str(batch.Column(5), index),
			}

			if !batch.Column(6).IsNull(index) {
				row.Positions = batch.Column(6).(*array.Int32).Value(index)
			}

			versions := batch.Column(7).(*array.Map)

			if !versions.IsNull(index) {
				start, end := versions.ValueOffsets(index)
				row.SchemaVersions = make(map[string]string, end-start)

				for element := int(start); element < int(end); element++ {
					row.SchemaVersions[str(versions.Keys(), element)] = str(versions.Items(), element)
				}
			}

			if micros, ok := when(batch.Column(1), index); ok {
				row.StartedAt = micros.ToTime(arrow.Microsecond).UTC()
			}

			rows = append(rows, row)
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].StartedAt.After(rows[j].StartedAt) })

	return rows, nil
}

// ref reads a nested envelope reference column.
func ref(column arrow.Array, row int) EnvelopeRefRow {
	if column.IsNull(row) {
		return EnvelopeRefRow{}
	}

	fields := column.(*array.Struct)

	return EnvelopeRefRow{
		Run:      str(fields.Field(0), row),
		Sequence: num(fields.Field(1), row),
		Ordinal:  num(fields.Field(2), row),
	}
}

// listBounds returns the half-open element range one list row occupies in the
// list's flattened value array.
func listBounds(column *array.List, row int) (start, end int) {
	if column.IsNull(row) {
		return 0, 0
	}

	offsets := column.Offsets()

	return int(offsets[row]), int(offsets[row+1])
}

/*
Witnesses yields the artifact witnesses of one run.

Passing a non-empty kind selects a single artifact family; "state" reproduces
what the old states/ key prefix held, which is now a predicate rather than a
separate table.
*/
func (c *Catalog) Witnesses(ctx context.Context, run, kind string) ([]WitnessRow, error) {
	return c.witnesses(ctx, run, kind, nil, nil, nil)
}

func (c *Catalog) witnesses(ctx context.Context, run, kind string, sequence, ordinal *int64, seen map[EnvelopeRefRow]bool) ([]WitnessRow, error) {
	filters := []iceberg.BooleanExpression{forRun(run)}

	if kind != "" {
		filters = append(filters, iceberg.EqualTo(iceberg.Reference("artifact_kind"), kind))
	}

	batches, err := c.scan(ctx, Witnesses, nil, filters...)

	if err != nil {
		return nil, err
	}

	rows := []WitnessRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] witnesses batch", err))
		}

		parents, _ := batch.Column(9).(*array.List)
		semantic, _ := batch.Column(10).(*array.List)

		for index := range int(batch.NumRows()) {
			reference := ref(batch.Column(1), index)

			if seen[reference] {
				continue
			}

			if sequence != nil && reference.Sequence != *sequence {
				continue
			}
			if ordinal != nil && reference.Ordinal != *ordinal {
				continue
			}

			row := WitnessRow{
				Run:                   str(batch.Column(0), index),
				Envelope:              ref(batch.Column(1), index),
				Boundary:              str(batch.Column(2), index),
				ArtifactKind:          str(batch.Column(3), index),
				ArtifactIdentity:      str(batch.Column(4), index),
				ArtifactKindLabel:     str(batch.Column(5), index),
				Component:             str(batch.Column(7), index),
				ComponentStateVersion: num(batch.Column(8), index),
				Payload:               bin(batch.Column(11), index),
			}

			if micros, ok := when(batch.Column(6), index); ok {
				row.ProducedAt = micros.ToTime(arrow.Microsecond).UTC()
			}

			if parents != nil {
				start, end := listBounds(parents, index)
				values := parents.ListValues()

				for element := start; element < end; element++ {
					row.ImmediateParents = append(row.ImmediateParents, ref(values, element))
				}
			}

			if semantic != nil {
				start, end := listBounds(semantic, index)
				values := semantic.ListValues()

				for element := start; element < end; element++ {
					row.SemanticParents = append(row.SemanticParents, str(values, element))
				}
			}

			rows = append(rows, row)
		}
	}

	return rows, nil
}

// Manifests yields the envelope manifests of one run.
func (c *Catalog) Manifests(ctx context.Context, run string) ([]ManifestRow, error) {
	return c.manifests(ctx, run, nil)
}

func (c *Catalog) manifests(ctx context.Context, run string, sequence *int64) ([]ManifestRow, error) {
	batches, err := c.scan(ctx, Manifests, nil, forRun(run))

	if err != nil {
		return nil, err
	}

	rows := []ManifestRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] manifests batch", err))
		}

		for index := range int(batch.NumRows()) {
			if sequence != nil && ref(batch.Column(1), index).Sequence != *sequence {
				continue
			}

			row := ManifestRow{
				Run:           str(batch.Column(0), index),
				Envelope:      ref(batch.Column(1), index),
				Workload:      str(batch.Column(2), index),
				DomainKind:    str(batch.Column(3), index),
				Symbol:        str(batch.Column(4), index),
				VenueSequence: str(batch.Column(6), index),
			}

			if micros, ok := when(batch.Column(5), index); ok {
				row.VenueAt = micros.ToTime(arrow.Microsecond).UTC()
			}

			rows = append(rows, row)
		}
	}

	return rows, nil
}

// Lifecycle yields the position and order transitions of one run, in time order.
func (c *Catalog) Lifecycle(ctx context.Context, run string) ([]LifecycleRow, error) {
	batches, err := c.scan(ctx, Lifecycle, nil, forRun(run))

	if err != nil {
		return nil, err
	}

	rows := []LifecycleRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] lifecycle batch", err))
		}

		for index := range int(batch.NumRows()) {
			row := LifecycleRow{
				Run:                 str(batch.Column(0), index),
				DecisionID:          str(batch.Column(1), index),
				ActionCorrelationID: str(batch.Column(2), index),
				Symbol:              str(batch.Column(3), index),
				Kind:                str(batch.Column(4), index),
				Action:              str(batch.Column(5), index),
				CaptureSeq:          num(batch.Column(7), index),
			}

			if micros, ok := when(batch.Column(6), index); ok {
				row.At = micros.ToTime(arrow.Microsecond).UTC()
			}

			// All execution columns are null for position-only transitions.
			if hasExecution(batch, index) {
				row.Exec = &ExecutionRow{
					OrderID: str(batch.Column(8), index), ClientOrderID: str(batch.Column(9), index),
					ExecID: str(batch.Column(10), index), ExecType: str(batch.Column(11), index),
					TradeID: num(batch.Column(12), index), Side: str(batch.Column(13), index),
					OrderType: str(batch.Column(14), index), OrderStatus: str(batch.Column(15), index),
					LiquidityInd: str(batch.Column(16), index), LastQty: dec(batch.Column(18), index),
					LastPrice: dec(batch.Column(19), index), Cost: dec(batch.Column(20), index),
					CumQty: dec(batch.Column(21), index), CumCost: dec(batch.Column(22), index),
					AvgPrice: dec(batch.Column(23), index), FeeUsdEquiv: dec(batch.Column(24), index),
					Fees: str(batch.Column(25), index),
				}

				if micros, ok := when(batch.Column(17), index); ok {
					row.Exec.At = micros.ToTime(arrow.Microsecond).UTC()
				}
			}

			rows = append(rows, row)
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].At.Before(rows[j].At) })

	return rows, nil
}

// Gaps yields the capture-integrity gaps recorded for one run.
func (c *Catalog) Gaps(ctx context.Context, run string) ([]GapRow, error) {
	batches, err := c.scan(ctx, Gaps, nil, forRun(run))

	if err != nil {
		return nil, err
	}

	rows := []GapRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] gaps batch", err))
		}

		for index := range int(batch.NumRows()) {
			rows = append(rows, GapRow{
				Run:      str(batch.Column(0), index),
				Sequence: num(batch.Column(1), index),
				Encoding: str(batch.Column(2), index),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Sequence < rows[j].Sequence })

	return rows, nil
}

/*
dec reads an optional decimal column back into a Kraken decimal.

The value is reconstructed through its decimal text rather than through
NewFromBigInt, which treats its argument as a whole number and would rescale
it: what Arrow hands back is an unscaled integer at DecimalScale, so the point
has to be placed explicitly.
*/
func dec(column arrow.Array, row int) *decimal.Decimal {
	if column.IsNull(row) {
		return nil
	}

	unscaled := column.(*array.Decimal128).Value(row).BigInt()
	digits := unscaled.String()
	sign := ""

	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}

	for int64(len(digits)) <= DecimalScale {
		digits = "0" + digits
	}

	point := int64(len(digits)) - DecimalScale
	value, err := decimal.NewFromString(sign + digits[:point] + "." + digits[point:])

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "[iceberg] decode decimal", err))

		return nil
	}

	return value
}

// Outcomes yields the graded decisions of one run.
func (c *Catalog) Outcomes(ctx context.Context, run string) ([]OutcomeRow, error) {
	batches, err := c.scan(ctx, Outcomes, nil, forRun(run))

	if err != nil {
		return nil, err
	}

	rows := []OutcomeRow{}

	for batch, err := range batches {
		if err != nil {
			return nil, errnie.Error(errnie.Err(errnie.BadGateway, "[iceberg] outcomes batch", err))
		}

		context, _ := batch.Column(10).(*array.List)

		for index := range int(batch.NumRows()) {
			row := OutcomeRow{
				Run:          str(batch.Column(0), index),
				DecisionID:   num(batch.Column(1), index),
				Label:        str(batch.Column(3), index),
				ActionKind:   str(batch.Column(5), index),
				ActionReduce: batch.Column(7).(*array.Boolean).Value(index),
				Authority:    batch.Column(8).(*array.Float64).Value(index),
				Value:        batch.Column(12).(*array.Float64).Value(index),
				Complete:     batch.Column(13).(*array.Boolean).Value(index),
				Forced:       batch.Column(14).(*array.Boolean).Value(index),
				Initial:      dec(batch.Column(15), index),
				Reference:    dec(batch.Column(16), index),
				Quantity:     dec(batch.Column(17), index),
				Cost:         dec(batch.Column(18), index),
				Fee:          dec(batch.Column(19), index),
				Opportunity:  dec(batch.Column(20), index),
			}

			row.Trader = batch.Column(2).(*array.Int32).Value(index)
			row.ActionPower = batch.Column(6).(*array.Int32).Value(index)

			if !batch.Column(9).IsNull(index) {
				outcome := batch.Column(9).(*array.Float64).Value(index)
				row.Outcome = &outcome
			}

			if micros, ok := when(batch.Column(4), index); ok {
				row.At = micros.ToTime(arrow.Microsecond).UTC()
			}

			if micros, ok := when(batch.Column(11), index); ok {
				row.Through = micros.ToTime(arrow.Microsecond).UTC()
			}

			if context != nil {
				start, end := listBounds(context, index)
				values := context.ListValues()

				for element := start; element < end; element++ {
					row.Context = append(row.Context, num(values, element))
				}
			}

			rows = append(rows, row)
		}
	}

	return rows, nil
}

// hasExecution distinguishes absent execution facts from present facts whose
// optional order ID, trade ID or quantity was not supplied by the venue.
func hasExecution(batch arrow.RecordBatch, row int) bool {
	// Lifecycle schema columns 8 through 25 are the execution fact.
	for column := 8; column <= 25; column++ {
		if !batch.Column(column).IsNull(row) {
			return true
		}
	}
	return false
}
