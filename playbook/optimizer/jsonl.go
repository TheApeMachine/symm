package optimizer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ReadJSONL reads optimizer samples from audit-style JSONL.
func ReadJSONL(reader io.Reader) ([]Sample, error) {
	scanner := bufio.NewScanner(reader)
	var samples []Sample
	var lineNo int
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parsed, err := parseJSONLine([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		samples = append(samples, parsed...)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

func parseJSONLine(line []byte) ([]Sample, error) {
	var row map[string]any
	if err := json.Unmarshal(line, &row); err != nil {
		return nil, err
	}
	if decisions, ok := row["decisions"].([]any); ok {
		samples := make([]Sample, 0, len(decisions))
		for _, decision := range decisions {
			child, ok := decision.(map[string]any)
			if !ok {
				continue
			}
			samples = append(samples, sampleFromMap(child))
		}
		return samples, nil
	}
	return []Sample{sampleFromMap(row)}, nil
}

func sampleFromMap(row map[string]any) Sample {
	sample := Sample{
		Symbol:          stringValue(row, "symbol"),
		Type:            stringValue(row, "type"),
		Source:          stringValue(row, "source"),
		Category:        stringValue(row, "category"),
		Verdict:         stringValue(row, "verdict"),
		Confidence:      numberValue(row, "confidence"),
		Edge:            firstNumber(row, "edge", "expected_edge"),
		Reward:          firstNumber(row, "reward", "realized_reward", "net_reward"),
		Hurdle:          firstNumber(row, "hurdle", "fee_hurdle"),
		Friction:        firstNumber(row, "friction", "round_trip_friction"),
		FillProbability: firstNumber(row, "fill_probability", "fill_prob"),
	}
	if economicPriced, ok := boolValue(row, "economic_priced"); ok {
		sample.EconomicPriced = economicPriced
	}
	if filled, ok := boolValue(row, "filled"); ok {
		sample.Filled = &filled
	}
	return sample
}

func stringValue(row map[string]any, key string) string {
	value, ok := row[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func firstNumber(row map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value := numberValue(row, key); value != 0 {
			return value
		}
	}
	return 0
}

func numberValue(row map[string]any, key string) float64 {
	value, ok := row[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		number, _ := typed.Float64()
		return number
	case string:
		number, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return number
	default:
		return 0
	}
}

func boolValue(row map[string]any, key string) (bool, bool) {
	value, ok := row[key]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return false, false
	}
}
