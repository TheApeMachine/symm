package replay

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/theapemachine/errnie"
)

/*
ScanWSRows reads JSONL capture path in file order and emits decoded rows for one
WebSocket channel. Unlike Hub playback, each call starts at the beginning of the
file with no shared global state.
*/
func ScanWSRows[T any](
	ctx context.Context, path string, channel string,
) (<-chan *T, error) {
	file, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	outbound := make(chan *T, 256)

	go func() {
		defer close(outbound)
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			var line Line

			if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
				continue
			}

			if line.Transport != TransportWS {
				continue
			}

			if line.Direction != "" && line.Direction != DirectionIn {
				continue
			}

			if line.Channel != channel {
				continue
			}

			rows, _, err := DecodeWSRows[T](line.Payload)

			if err != nil {
				continue
			}

			for index := range rows {
				row := rows[index]

				select {
				case <-ctx.Done():
					return
				case outbound <- &row:
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errnie.Error(fmt.Errorf("replay scan %s channel=%s: %w", path, channel, err))
		}
	}()

	return outbound, nil
}
