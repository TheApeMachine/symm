package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

type captureFrame struct {
	Endpoint   string          `json:"endpoint"`
	Payload    json.RawMessage `json:"payload"`
	ReceivedAt time.Time       `json:"received_at"`
}

type captureCursor struct {
	decoder  *json.Decoder
	previous time.Time
	frame    *captureFrame
}

/*
ReplayCapture merges untouched Kraken endpoint captures by arrival time and
replays only frames carrying the requested symbol.
*/
func (market *Market) ReplayCapture(symbols []string, readers ...io.Reader) error {
	if len(symbols) == 0 || len(readers) == 0 {
		return fmt.Errorf("market: replay capture requires symbols and readers")
	}
	symbolSet := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		if symbol == "" {
			return fmt.Errorf("market: replay capture requires named symbols")
		}

		symbolSet[symbol] = struct{}{}
	}

	cursors := make([]*captureCursor, len(readers))

	for index, reader := range readers {
		if reader == nil {
			return fmt.Errorf("market: replay capture reader %d is nil", index)
		}

		cursors[index] = &captureCursor{decoder: json.NewDecoder(reader)}

		if err := cursors[index].advance(symbolSet); err != nil {
			return fmt.Errorf("market: initialize capture reader %d: %w", index, err)
		}
	}

	previousAt := time.Time{}
	replayed := 0

	for {
		selected := nextCaptureCursor(cursors)

		if selected < 0 {
			if replayed == 0 {
				return fmt.Errorf("market: capture has no frames for requested symbols")
			}

			return nil
		}

		frame := cursors[selected].frame

		if !previousAt.IsZero() && frame.ReceivedAt.Before(previousAt) {
			return fmt.Errorf("market: merged capture chronology regressed")
		}

		if err := market.replayFrame(
			frame.Endpoint,
			frame.Payload,
			frame.ReceivedAt,
		); err != nil {
			return fmt.Errorf("market: replay capture frame %d: %w", replayed+1, err)
		}

		previousAt = frame.ReceivedAt
		replayed++

		if err := cursors[selected].advance(symbolSet); err != nil {
			return fmt.Errorf("market: advance capture reader %d: %w", selected, err)
		}
	}
}

func (cursor *captureCursor) advance(symbols map[string]struct{}) error {
	for record := 1; ; record++ {
		frame := &captureFrame{}
		err := cursor.decoder.Decode(frame)

		if errors.Is(err, io.EOF) {
			cursor.frame = nil
			return nil
		}

		if err != nil {
			return fmt.Errorf("decode record %d: %w", record, err)
		}

		if frame.ReceivedAt.IsZero() ||
			(!cursor.previous.IsZero() && frame.ReceivedAt.Before(cursor.previous)) {
			return fmt.Errorf("capture record %d has invalid arrival time", record)
		}

		cursor.previous = frame.ReceivedAt
		carries, err := frameCarriesSymbols(frame.Payload, symbols)

		if err != nil {
			return fmt.Errorf("capture record %d: %w", record, err)
		}

		if !carries {
			continue
		}

		cursor.frame = frame
		return nil
	}
}

func frameCarriesSymbols(
	payload []byte,
	symbols map[string]struct{},
) (bool, error) {
	header := struct {
		Channel string `json:"channel"`
		Data    []struct {
			Symbol string `json:"symbol"`
		} `json:"data"`
	}{}

	if err := json.Unmarshal(payload, &header); err != nil {
		return false, fmt.Errorf("decode payload header: %w", err)
	}

	if header.Channel != "ticker" && header.Channel != "trade" &&
		header.Channel != "level3" {
		return false, nil
	}

	for _, data := range header.Data {
		if _, requested := symbols[data.Symbol]; requested {
			return true, nil
		}
	}

	return false, nil
}

func nextCaptureCursor(cursors []*captureCursor) int {
	selected := -1

	for index, cursor := range cursors {
		if cursor.frame == nil {
			continue
		}

		if selected < 0 || cursor.frame.ReceivedAt.Before(
			cursors[selected].frame.ReceivedAt,
		) {
			selected = index
		}
	}

	return selected
}
