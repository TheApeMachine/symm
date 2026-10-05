package tables

import (
	"context"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

const (
	defaultTargetCommitBytes  = 16 * 1024 * 1024 // 16 MB on-disk batch target
	measurementBatchThreshold = 20000
)

/*
Writer buffers incoming measurements and commits them to Iceberg.
*/
type Writer struct {
	catalog *Catalog
	epoch   int64

	mu            sync.Mutex
	measurements  []data.Publication
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
		epoch:   epoch,
	}
}

/*
Add routes an incoming publication to its canonical table family buffer.
*/
func (writer *Writer) Add(channel string, pub data.Publication) {
	if pub.Measurement == nil {
		return
	}

	size := measurementSize(pub.Measurement)

	writer.lock()
	defer writer.unlock()

	writer.measurements = append(writer.measurements, pub)
	writer.bufferedBytes += size
}

/*
Pending returns total buffered measurements.
*/
func (writer *Writer) Pending() int {
	writer.lock()
	defer writer.unlock()

	return len(writer.measurements)
}

/*
BufferedBytes returns total estimated bytes of buffered measurements.
*/
func (writer *Writer) BufferedBytes() int64 {
	writer.lock()
	defer writer.unlock()

	return writer.bufferedBytes
}

/*
CommitReady commits any family whose buffer meets its volume or size threshold, or all families if forceAll is true.
*/
func (writer *Writer) CommitReady(ctx context.Context, forceAll bool) error {
	targetBytes := int64(defaultTargetCommitBytes)

	if configured := viper.GetInt64("hindsight.capture.commit_bytes"); configured > 0 {
		targetBytes = configured
	}

	threshold := measurementBatchThreshold

	if viper.GetInt("hindsight.capture.commit_rows") > 0 {
		threshold = viper.GetInt("hindsight.capture.commit_rows")
	}

	if err := writer.commitFamily(ctx, Measurements, targetBytes, threshold, forceAll, func() ([]data.Publication, int64) {
		rows := writer.measurements
		bytes := writer.bufferedBytes
		writer.measurements = nil
		writer.bufferedBytes = 0

		return rows, bytes
	}, func(remaining []data.Publication, remainingBytes int64) {
		writer.measurements = append(remaining, writer.measurements...)
		writer.bufferedBytes += remainingBytes
	}); err != nil {
		return err
	}

	return nil
}

func (writer *Writer) commitFamily(
	ctx context.Context,
	tableName string,
	targetBytes int64,
	thresholdRows int,
	forceAll bool,
	takeRows func() ([]data.Publication, int64),
	putRows func([]data.Publication, int64),
) error {
	writer.lock()

	rowsToCommit, totalBytes := takeRows()

	if len(rowsToCommit) == 0 {
		writer.unlock()

		return nil
	}

	if !forceAll && totalBytes < targetBytes && len(rowsToCommit) < thresholdRows {
		putRows(rowsToCommit, totalBytes)
		writer.unlock()

		return nil
	}

	writer.unlock()

	tbl, err := writer.catalog.Load(ctx, tableName)

	if err != nil {
		writer.lock()
		putRows(rowsToCommit, totalBytes)
		writer.unlock()

		return err
	}

	measList := make([]*data.Measurement, len(rowsToCommit))
	for idx, pub := range rowsToCommit {
		measList[idx] = pub.Measurement
	}

	reader, err := measurementRecords(tbl.Schema(), measList, writer.epoch)
	if err != nil {
		writer.lock()
		putRows(rowsToCommit, totalBytes)
		writer.unlock()

		return err
	}

	defer reader.Release()

	var appendErr error
	if err := writer.catalog.WithCommit(ctx, func() error {
		_, appendErr = tbl.Append(writer.catalog.context(ctx), reader, nil)
		return appendErr
	}); err != nil {
		writer.lock()
		putRows(rowsToCommit, totalBytes)
		writer.unlock()

		return errnie.Error(errnie.Err(
			errnie.BadGateway,
			"[iceberg] failed to append to "+tableName,
			err,
		))
	}

	// Release generations now that storage has committed the batch
	for _, pub := range rowsToCommit {
		pub.Release()
	}

	return nil
}

func (writer *Writer) ReleaseRemaining() {
	writer.lock()
	defer writer.unlock()

	for _, p := range writer.measurements {
		p.Release()
	}
	writer.measurements = nil
	writer.bufferedBytes = 0
}

func measurementSize(measurement *data.Measurement) int64 {
	if measurement == nil {
		return 0
	}

	size := int64(128 + len(measurement.Label) + len(measurement.Source))

	for index := range measurement.Metrics {
		size += int64(32 + len(measurement.Metrics[index].Key))
	}

	for index := range measurement.Metadata {
		size += int64(16 + len(measurement.Metadata[index].Key) + len(measurement.Metadata[index].Value))
	}

	for index := range measurement.Provenance {
		size += int64(16 + len(measurement.Provenance[index].Key) + len(measurement.Provenance[index].Value))
	}

	return size
}
