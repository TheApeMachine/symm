package tables

import (
	"context"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewTableSink creates a streaming persistence closure that routes incoming measurements into the catalog table writer.
*/
func NewTableSink(writer *Writer, channel string) types.Value[*data.Measurement[float64], *data.Measurement[float64]] {
	return func(measurement *data.Measurement[float64]) *data.Measurement[float64] {
		if writer == nil || measurement == nil {
			return measurement
		}

		writer.Add(channel, measurement)
		return measurement
	}
}

/*
NewReplaySource creates a streaming replayer that reads measurements from a catalog historical run.
*/
func NewReplaySource(catalog *Catalog, epoch int64) types.Value[context.Context, <-chan *data.Measurement[float64]] {
	return func(ctx context.Context) <-chan *data.Measurement[float64] {
		out := make(chan *data.Measurement[float64], 128)
		if catalog == nil {
			close(out)
			return out
		}

		go func() {
			defer close(out)
			_, seq, err := catalog.Replay(ctx, epoch)
			if err != nil || seq == nil {
				return
			}

			for measurement, seqErr := range seq {
				if seqErr != nil || measurement == nil {
					continue
				}

				select {
				case <-ctx.Done():
					return
				case out <- measurement:
				}
			}
		}()

		return out
	}
}
