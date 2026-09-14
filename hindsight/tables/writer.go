package tables

import (
	"context"
	"sync"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

/*
Writer accumulates rows per canonical table family and commits them as Iceberg appends.

Every append is a snapshot plus a metadata write, so rows are buffered rather
than written as they arrive. Callers decide the cadence; this type guarantees
that a family is cleared only after its rows have been appended, that a
payload bound keeps each snapshot inside the catalog's HTTP/object timeout,
and that a family which never reached the catalog is still here afterwards.
*/
type Writer struct {
	catalog *Catalog

	mutex         sync.Mutex
	appendBytes   int
	spotLevel3    []SpotLevel3Row
	spotTicker    []SpotTickerRow
	spotTrade     []SpotTradeRow
	futuresTicker []FuturesTickerRow
	futuresTrade  []FuturesTradeRow
	executions    []ExecutionRow
	measurements  []MeasurementRow
	models        []ModelRow
	grids         []GridRow
	positions     []PositionRow
	decisions     []OutcomeRow
	outcomes      []OutcomeRow
}

// NewWriter returns a Writer appending into the given catalog.
func NewWriter(catalog *Catalog) *Writer {
	return &Writer{
		catalog:     catalog,
		appendBytes: viper.GetInt("storage.iceberg.append_bytes"),
	}
}

func (w *Writer) AddSpotLevel3(row SpotLevel3Row) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.spotLevel3 = append(w.spotLevel3, row)
}

func (w *Writer) AddSpotTicker(row SpotTickerRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.spotTicker = append(w.spotTicker, row)
}

func (w *Writer) AddSpotTrade(row SpotTradeRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.spotTrade = append(w.spotTrade, row)
}

func (w *Writer) AddFuturesTicker(row FuturesTickerRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.futuresTicker = append(w.futuresTicker, row)
}

func (w *Writer) AddFuturesTrade(row FuturesTradeRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.futuresTrade = append(w.futuresTrade, row)
}

func (w *Writer) AddExecution(row ExecutionRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.executions = append(w.executions, row)
}

func (w *Writer) AddMeasurement(row MeasurementRow) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.measurements = append(w.measurements, row)
}

// Pending reports how many rows are buffered across every family.
func (w *Writer) Pending() int {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	return len(w.spotLevel3) + len(w.spotTicker) + len(w.spotTrade) +
		len(w.futuresTicker) + len(w.futuresTrade) + len(w.executions) + len(w.measurements) +
		len(w.models) + len(w.grids) + len(w.positions) +
		len(w.decisions) + len(w.outcomes)
}

/*
Commit appends every buffered family, one family at a time.
*/
func (w *Writer) Commit(ctx context.Context) error {
	if w.appendBytes < 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[iceberg] append_bytes must be nonnegative",
			nil,
		))
	}

	if err := commitFamily(w, ctx, SpotLevel3,
		func() []SpotLevel3Row { rows := w.spotLevel3; w.spotLevel3 = nil; return rows },
		func(rows []SpotLevel3Row) { w.spotLevel3 = append(rows, w.spotLevel3...) },
		nil, fillSpotLevel3,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, SpotTicker,
		func() []SpotTickerRow { rows := w.spotTicker; w.spotTicker = nil; return rows },
		func(rows []SpotTickerRow) { w.spotTicker = append(rows, w.spotTicker...) },
		nil, fillSpotTicker,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, SpotTrade,
		func() []SpotTradeRow { rows := w.spotTrade; w.spotTrade = nil; return rows },
		func(rows []SpotTradeRow) { w.spotTrade = append(rows, w.spotTrade...) },
		nil, fillSpotTrade,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, FuturesTicker,
		func() []FuturesTickerRow { rows := w.futuresTicker; w.futuresTicker = nil; return rows },
		func(rows []FuturesTickerRow) { w.futuresTicker = append(rows, w.futuresTicker...) },
		nil, fillFuturesTicker,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, FuturesTrade,
		func() []FuturesTradeRow { rows := w.futuresTrade; w.futuresTrade = nil; return rows },
		func(rows []FuturesTradeRow) { w.futuresTrade = append(rows, w.futuresTrade...) },
		nil, fillFuturesTrade,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, Executions,
		func() []ExecutionRow { rows := w.executions; w.executions = nil; return rows },
		func(rows []ExecutionRow) { w.executions = append(rows, w.executions...) },
		nil, fillExecutions,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, Measurements,
		func() []MeasurementRow { rows := w.measurements; w.measurements = nil; return rows },
		func(rows []MeasurementRow) { w.measurements = append(rows, w.measurements...) },
		func(row MeasurementRow) int { return len(row.Payload) },
		fillMeasurements,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, Models,
		func() []ModelRow { rows := w.models; w.models = nil; return rows },
		func(rows []ModelRow) { w.models = append(rows, w.models...) },
		func(row ModelRow) int { return len(row.Payload) },
		fillModels,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, Grids,
		func() []GridRow { rows := w.grids; w.grids = nil; return rows },
		func(rows []GridRow) { w.grids = append(rows, w.grids...) },
		func(row GridRow) int { return len(row.Payload) },
		fillGrids,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, Positions,
		func() []PositionRow { rows := w.positions; w.positions = nil; return rows },
		func(rows []PositionRow) { w.positions = append(rows, w.positions...) },
		nil, fillPositions,
	); err != nil {
		return err
	}

	if err := commitFamily(w, ctx, Decisions,
		func() []OutcomeRow { rows := w.decisions; w.decisions = nil; return rows },
		func(rows []OutcomeRow) { w.decisions = append(rows, w.decisions...) },
		nil, fillOutcomes,
	); err != nil {
		return err
	}

	return commitFamily(w, ctx, Outcomes,
		func() []OutcomeRow { rows := w.outcomes; w.outcomes = nil; return rows },
		func(rows []OutcomeRow) { w.outcomes = append(rows, w.outcomes...) },
		nil, fillOutcomes,
	)
}

func commitFamily[T any](
	w *Writer,
	ctx context.Context,
	name string,
	take func() []T,
	put func([]T),
	payload func(T) int,
	fill func(*array.RecordBuilder, []T),
) error {
	w.mutex.Lock()
	rows := take()
	w.mutex.Unlock()

	if len(rows) == 0 {
		return nil
	}

	var sizeOf func(int) int

	if payload != nil {
		sizeOf = func(index int) int { return payload(rows[index]) }
	}

	committed, err := w.append(ctx, name, sizeOf, len(rows), func(builder *array.RecordBuilder, start, end int) {
		fill(builder, rows[start:end])
	})

	if committed < len(rows) {
		w.mutex.Lock()
		put(rows[committed:])
		w.mutex.Unlock()
	}

	return err
}

func (w *Writer) append(
	ctx context.Context,
	name string,
	payloadSize func(int) int,
	count int,
	fill func(*array.RecordBuilder, int, int),
) (int, error) {
	committed := 0

	for committed < count {
		end, _, err := span(committed, count, w.appendBytes, payloadSize)

		if err != nil {
			return committed, err
		}

		tbl, err := w.catalog.Load(ctx, name)

		if err != nil {
			return committed, err
		}

		reader, err := records(tbl.Schema(), end-committed, payloadSize, func(builder *array.RecordBuilder, s, e int) {
			fill(builder, committed+s, committed+e)
		})

		if err != nil {
			return committed, err
		}

		_, appendErr := tbl.Append(ctx, reader, nil)
		reader.Release()

		if appendErr != nil {
			return committed, errnie.Error(errnie.Err(
				errnie.BadGateway,
				"[iceberg] failed to commit append to "+name,
				appendErr,
			))
		}

		committed = end
	}

	return committed, nil
}
