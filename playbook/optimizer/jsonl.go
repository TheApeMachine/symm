package optimizer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

func ReadReplayJSONL(reader io.Reader) ([]ReplayFrame, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	builder := newFrameBuilder()
	line := 0

	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}

		var frame ReplayFrame
		if err := json.Unmarshal(raw, &frame); err == nil && !frame.Time.IsZero() && len(frame.Artifacts) > 0 {
			builder.addFrame(frame)
			continue
		}

		var artifact ReplayArtifact
		if err := json.Unmarshal(raw, &artifact); err != nil {
			return nil, fmt.Errorf("decode replay JSONL line %d: %w", line, err)
		}
		if artifact.Timestamp == 0 {
			return nil, fmt.Errorf("decode replay JSONL line %d: artifact missing timestamp", line)
		}

		stamp := time.Unix(0, artifact.Timestamp).UTC()
		frameAt := builder.frameAt(stamp)
		frameAt.Artifacts = append(frameAt.Artifacts, artifact)
		if price := priceFromReplayArtifact(artifact); price > 0 {
			frameAt.Prices[artifact.Scope] = price
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return builder.frames(), nil
}

func WriteReplayJSONL(writer io.Writer, frames []ReplayFrame) error {
	encoder := json.NewEncoder(writer)
	for _, frame := range frames {
		if err := encoder.Encode(frame); err != nil {
			return err
		}
	}

	return nil
}

func (builder *frameBuilder) addFrame(frame ReplayFrame) {
	if frame.Time.IsZero() {
		return
	}

	existing := builder.frameAt(frame.Time)
	existing.Artifacts = append(existing.Artifacts, frame.Artifacts...)
	if existing.Prices == nil {
		existing.Prices = make(map[string]float64)
	}
	for symbol, price := range frame.Prices {
		existing.Prices[symbol] = price
	}
	for _, artifact := range frame.Artifacts {
		if price := priceFromReplayArtifact(artifact); price > 0 {
			existing.Prices[artifact.Scope] = price
		}
	}
}

func priceFromReplayArtifact(artifact ReplayArtifact) float64 {
	if artifact.Scope == "" || artifact.Role != "ticker" {
		return 0
	}
	data, ok := artifact.Payload["data"].([]any)
	if !ok || len(data) == 0 {
		rows, ok := artifact.Payload["data"].([]map[string]any)
		if !ok || len(rows) == 0 {
			return 0
		}
		if price, ok := rows[0]["last"].(float64); ok {
			return price
		}
		return 0
	}
	row, ok := data[0].(map[string]any)
	if !ok {
		return 0
	}
	if price, ok := row["last"].(float64); ok {
		return price
	}

	return 0
}

func SortReplayFrames(frames []ReplayFrame) {
	sort.SliceStable(frames, func(first, second int) bool {
		return frames[first].Time.Before(frames[second].Time)
	})
}
