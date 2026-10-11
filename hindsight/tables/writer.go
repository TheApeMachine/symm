package tables

import (
	"context"
	"fmt"
	"sync"

	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

const defaultTargetCommitBytes = 16 * 1024 * 1024 // 16 MB on-disk batch target

/*
TargetCommitBytes returns the configured target size in bytes for Iceberg data files,
falling back to defaultTargetCommitBytes (16 MB).
*/
func TargetCommitBytes() int64 {
	if configured := viper.GetInt64("storage.iceberg.append_bytes"); configured > 0 {
		return configured
	}

	if configured := viper.GetInt64("hindsight.capture.commit_bytes"); configured > 0 {
		return configured
	}

	return defaultTargetCommitBytes
}

/*
Writer buffers incoming measurements and commits them to Iceberg.
*/
type Writer struct {
	catalog *Catalog
	table   string
	epoch   int64

	mu            sync.Mutex
	measurements  []*data.Measurement
	bufferedBytes int64
}

func (writer *Writer) lock() {
	writer.mu.Lock()
}

func (writer *Writer) unlock() {
	writer.mu.Unlock()
}

/*
NewWriter constructs a Writer writing into the given catalog for a stable run epoch.
*/
func NewWriter(catalog *Catalog, epoch int64) *Writer {
	return &Writer{
		catalog: catalog,
		table:   Measurements,
		epoch:   epoch,
	}
}

/*
Add routes an incoming publication to its canonical table family buffer.
*/
func (writer *Writer) Add(channel string, measurement *data.Measurement) {
	if measurement == nil {
		return
	}

	writer.lock()
	defer writer.unlock()

	writer.measurements = append(writer.measurements, measurement)
	writer.bufferedBytes += measurement.ApproximateBytes()
}

/*
CommitReady commits buffered measurements to Iceberg once the buffer reaches
the target byte size estimated via measurement.ApproximateBytes(), or flushes
all buffered measurements if forceAll is true.
*/
func (writer *Writer) CommitReady(ctx context.Context, forceAll bool) error {
	targetBytes := TargetCommitBytes()

	writer.lock()

	if !forceAll && writer.bufferedBytes < targetBytes {
		writer.unlock()

		return nil
	}

	if len(writer.measurements) == 0 {
		writer.unlock()

		return nil
	}

	var batches [][]*data.Measurement
	var currentBatch []*data.Measurement
	var currentBatchBytes int64

	measurements := writer.measurements
	writer.measurements = nil
	writer.bufferedBytes = 0

	for _, measurement := range measurements {
		measBytes := measurement.ApproximateBytes()

		if len(currentBatch) > 0 && currentBatchBytes+measBytes > targetBytes {
			batches = append(batches, currentBatch)
			currentBatch = nil
			currentBatchBytes = 0
		}

		currentBatch = append(currentBatch, measurement)
		currentBatchBytes += measBytes
	}

	if len(currentBatch) > 0 {
		if !forceAll && currentBatchBytes < targetBytes {
			writer.measurements = currentBatch
			writer.bufferedBytes = currentBatchBytes
		}

		if forceAll || currentBatchBytes >= targetBytes {
			batches = append(batches, currentBatch)
		}
	}

	writer.unlock()

	for batchIndex, batch := range batches {
		if err := writer.commitBatch(ctx, writer.table, batch); err != nil {
			writer.lock()
			var uncommitted []*data.Measurement
			var uncommittedBytes int64

			for remainingIndex := batchIndex; remainingIndex < len(batches); remainingIndex++ {
				for _, meas := range batches[remainingIndex] {
					uncommitted = append(uncommitted, meas)
					uncommittedBytes += meas.ApproximateBytes()
				}
			}

			writer.measurements = append(uncommitted, writer.measurements...)
			writer.bufferedBytes += uncommittedBytes
			writer.unlock()

			return err
		}
	}

	return nil
}

func (writer *Writer) commitBatch(
	ctx context.Context,
	tableName string,
	batch []*data.Measurement,
) error {
	if len(batch) == 0 {
		return nil
	}

	tbl, err := writer.catalog.Load(ctx, tableName)

	if err != nil {
		return err
	}

	reader, err := measurementRecords(tbl.Schema(), batch, writer.epoch)

	if err != nil {
		return err
	}

	defer reader.Release()

	var appendErr error
	if err := writer.catalog.WithCommit(ctx, func() error {
		_, appendErr = tbl.Append(writer.catalog.context(ctx), reader, nil)
		return appendErr
	}); err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to append to "+tableName,
			err,
		))
	}

	return nil
}

func (writer *Writer) ReleaseRemaining() {
	writer.lock()
	defer writer.unlock()

	writer.measurements = nil
	writer.bufferedBytes = 0
}

/*
BufferedRows returns the current count of buffered measurements waiting to be committed.
*/
func (writer *Writer) BufferedRows() int {
	writer.lock()
	defer writer.unlock()

	return len(writer.measurements)
}

/*
BufferedBytes returns the approximate memory footprint in bytes of buffered measurements.
*/
func (writer *Writer) BufferedBytes() int64 {
	writer.lock()
	defer writer.unlock()

	return writer.bufferedBytes
}

/*
Append commits measurements for one epoch to the named table as snapshots
sized to TargetCommitBytes() estimated via measurement.ApproximateBytes().
Nothing is retained on success; on failure the error is returned.
*/
func (catalog *Catalog) Append(
	ctx context.Context, tableName string, epoch int64, measurements []*data.Measurement,
) error {
	if catalog == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[catalog] catalog is required", nil,
		))
	}

	if epoch <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[catalog] valid positive epoch is required", nil,
		))
	}

	if len(measurements) == 0 {
		return nil
	}

	writer := NewWriter(catalog, epoch)
	writer.table = tableName
	defer writer.ReleaseRemaining()

	for _, measurement := range measurements {
		if measurement == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation, "[catalog] cannot append a nil measurement", nil,
			))
		}

		if measurement.Epoch != epoch {
			return errnie.Error(errnie.Err(
				errnie.Validation, "[catalog] measurement epoch does not match append epoch", nil,
			))
		}

		writer.Add(tableName, measurement)
	}

	return errnie.Error(writer.CommitReady(ctx, true))
}

/*
Compact consolidates fragmented Parquet data files for the given epoch in tableName
into properly sized Parquet files based on targetBytes estimated via
measurement.ApproximateBytes().
If epoch <= 0, it compacts all historical epochs found in the table.
*/
func (catalog *Catalog) Compact(
	ctx context.Context,
	tableName string,
	epoch int64,
) error {
	if catalog == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[catalog] catalog is required", nil,
		))
	}

	ctx = catalog.context(ctx)

	tbl, err := catalog.Load(ctx, tableName)

	if err != nil {
		return err
	}

	var targetEpochs []int64

	if epoch > 0 {
		targetEpochs = []int64{epoch}
	}

	if epoch <= 0 {
		runs, runErr := catalog.Runs(ctx)

		if runErr != nil {
			return runErr
		}

		for _, run := range runs {
			if run.Epoch > 0 {
				targetEpochs = append(targetEpochs, run.Epoch)
			}
		}
	}

	targetBytes := TargetCommitBytes()

	for _, currentEpoch := range targetEpochs {
		if err := catalog.compactEpoch(ctx, tbl, tableName, currentEpoch, targetBytes); err != nil {
			return err
		}
	}

	return nil
}

func (catalog *Catalog) compactEpoch(
	ctx context.Context,
	tbl *icetable.Table,
	tableName string,
	epoch int64,
	targetBytes int64,
) error {
	predicate := iceberg.EqualTo(iceberg.Reference("epoch"), epoch)
	tasks, err := tbl.Scan(icetable.WithRowFilter(predicate)).PlanFiles(ctx)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			fmt.Sprintf("[iceberg] failed to plan files for compaction in %s epoch %d", tableName, epoch),
			err,
		))
	}

	if len(tasks) <= 1 {
		return nil
	}

	errnie.Info(fmt.Sprintf(
		"[iceberg] compacting %s epoch %d: %d files into target size (~%d MB)",
		tableName, epoch, len(tasks), targetBytes/(1024*1024),
	))

	var measurements []*data.Measurement

	for measurement, scanErr := range catalog.scan(ctx, tableName, epoch, nil, 0) {
		if scanErr != nil {
			return scanErr
		}

		measurements = append(measurements, measurement)
	}

	if len(measurements) == 0 {
		return nil
	}

	var batches [][]*data.Measurement
	var currentBatch []*data.Measurement
	var currentBatchBytes int64

	for _, measurement := range measurements {
		measBytes := measurement.ApproximateBytes()

		if len(currentBatch) > 0 && currentBatchBytes+measBytes > targetBytes {
			batches = append(batches, currentBatch)
			currentBatch = nil
			currentBatchBytes = 0
		}

		currentBatch = append(currentBatch, measurement)
		currentBatchBytes += measBytes
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	firstReader, err := measurementRecords(tbl.Schema(), batches[0], epoch)

	if err != nil {
		return err
	}

	defer firstReader.Release()

	var overwriteErr error
	err = catalog.WithCommit(ctx, func() error {
		tbl, overwriteErr = tbl.Overwrite(
			catalog.context(ctx),
			firstReader,
			nil,
			icetable.WithOverwriteFilter(predicate),
		)
		return overwriteErr
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			fmt.Sprintf("[iceberg] failed to overwrite during compaction in %s epoch %d", tableName, epoch),
			err,
		))
	}

	for batchIndex := 1; batchIndex < len(batches); batchIndex++ {
		batchReader, batchErr := measurementRecords(tbl.Schema(), batches[batchIndex], epoch)

		if batchErr != nil {
			return batchErr
		}

		var appendErr error
		err = catalog.WithCommit(ctx, func() error {
			tbl, appendErr = tbl.Append(catalog.context(ctx), batchReader, nil)
			return appendErr
		})

		batchReader.Release()

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.BadGateway,
				fmt.Sprintf("[iceberg] failed to append batch %d during compaction in %s epoch %d", batchIndex, tableName, epoch),
				err,
			))
		}
	}

	errnie.Info(fmt.Sprintf(
		"[iceberg] compacted %s epoch %d: %d files replaced by %d files (%d measurements)",
		tableName, epoch, len(tasks), len(batches), len(measurements),
	))

	return nil
}
