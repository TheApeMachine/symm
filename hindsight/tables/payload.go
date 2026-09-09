package tables

import (
	"context"
	"errors"

	"github.com/apache/arrow-go/v18/parquet/file"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/theapemachine/errnie"
)

// payloadBound reads only Parquet metadata. The uncompressed column chunk
// includes every dictionary entry and bounds the size of any one flat payload;
// compressed file size and average row size cannot provide that bound.
func (catalog *Catalog) payloadBound(
	ctx context.Context, loaded *icetable.Table, task icetable.FileScanTask,
) (bound int64, err error) {
	if _, exists := loaded.Schema().FindFieldByName("payload"); !exists {
		return 0, nil
	}
	storage, err := loaded.FS(ctx)

	if err != nil {
		return 0, errnie.Error(err)
	}
	input, err := storage.Open(task.File.FilePath())

	if err != nil {
		return 0, errnie.Error(err)
	}
	reader, err := file.NewParquetReader(input)

	if err != nil {
		return 0, errnie.Error(errors.Join(err, input.Close()))
	}
	defer func() { err = errnie.Error(errors.Join(err, reader.Close())) }()
	metadata := reader.MetaData()

	for column := range metadata.NumColumns() {
		if metadata.Schema.Column(column).Path() != "payload" {
			continue
		}

		for group := range reader.NumRowGroups() {
			chunk, err := metadata.RowGroup(group).ColumnChunk(column)

			if err != nil {
				return 0, errnie.Error(err)
			}
			bound = max(bound, chunk.TotalUncompressedSize())
		}
	}

	return bound, nil
}
