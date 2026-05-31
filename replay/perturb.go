package replay

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/bytedance/sonic"
)

const bookChannel = "book"

/*
PerturbConfig controls synthetic replay noise applied during tuning evals.
Quantity jitter preserves price levels; timestamp jitter preserves ordering.
*/
type PerturbConfig struct {
	Enabled         bool
	Seed            int64
	QtyJitterSigma  float64
	TimestampJitter time.Duration
}

func PerturbConfigFrom(enabled bool, seed int64, qtySigma float64, tsJitter time.Duration) PerturbConfig {
	return PerturbConfig{
		Enabled:         enabled,
		Seed:            seed,
		QtyJitterSigma:  qtySigma,
		TimestampJitter: tsJitter,
	}
}

type wireBookLevel struct {
	Price json.Number `json:"price"`
	Qty   json.Number `json:"qty"`
}

type wireBookRow struct {
	Symbol    string          `json:"symbol"`
	Bids      []wireBookLevel `json:"bids"`
	Asks      []wireBookLevel `json:"asks"`
	Checksum  int64           `json:"checksum"`
	Timestamp string          `json:"timestamp"`
}

/*
PerturbLine applies configured replay noise to one JSONL record.
*/
func PerturbLine(line Line, config PerturbConfig, random *rand.Rand) (Line, error) {
	if !config.Enabled || random == nil {
		return line, nil
	}

	if config.TimestampJitter > 0 && !line.Timestamp.IsZero() {
		jitterSpan := int64(config.TimestampJitter)
		offset := time.Duration(random.Int63n(2*jitterSpan+1) - jitterSpan)
		line.Timestamp = line.Timestamp.Add(offset)
	}

	if line.Transport != TransportWS || len(line.Payload) == 0 {
		return line, nil
	}

	if line.Channel != bookChannel {
		return line, nil
	}

	if config.QtyJitterSigma <= 0 {
		return line, nil
	}

	payload, err := perturbBookPayload(line.Payload, random, config.QtyJitterSigma)

	if err != nil {
		return line, fmt.Errorf("perturb book payload: %w", err)
	}

	line.Payload = payload

	return line, nil
}

func perturbBookPayload(payload json.RawMessage, random *rand.Rand, sigma float64) (json.RawMessage, error) {
	var envelope socketEnvelope

	if err := sonic.Unmarshal(payload, &envelope); err != nil {
		return payload, nil
	}

	if envelope.Type == "snapshot" {
		row, err := decodeWireBookRow(envelope.Data)

		if err != nil {
			return payload, err
		}

		jitterBookRow(row, random, sigma)

		return encodeBookEnvelope(envelope.Type, row)
	}

	rows, err := decodeWireBookRows(envelope.Data)

	if err != nil {
		return payload, err
	}

	for index := range rows {
		jitterBookRow(&rows[index], random, sigma)
	}

	return encodeBookEnvelope(envelope.Type, rows)
}

func decodeWireBookRow(data json.RawMessage) (*wireBookRow, error) {
	var row wireBookRow

	if err := sonic.Unmarshal(data, &row); err != nil {
		return nil, err
	}

	return &row, nil
}

func decodeWireBookRows(data json.RawMessage) ([]wireBookRow, error) {
	var rows []wireBookRow

	if err := sonic.Unmarshal(data, &rows); err != nil {
		return nil, err
	}

	return rows, nil
}

func jitterBookRow(row *wireBookRow, random *rand.Rand, sigma float64) {
	row.Checksum = 0
	jitterBookSide(row.Bids, random, sigma)
	jitterBookSide(row.Asks, random, sigma)
}

func jitterBookSide(levels []wireBookLevel, random *rand.Rand, sigma float64) {
	for index := range levels {
		qty, err := levels[index].Qty.Float64()

		if err != nil || qty <= 0 {
			continue
		}

		scaled := qty * (1 + random.NormFloat64()*sigma)

		if scaled < 0 {
			scaled = 0
		}

		levels[index].Qty = json.Number(formatQty(scaled))
	}
}

func formatQty(value float64) string {
	if value == 0 {
		return "0"
	}

	return trimFloat(value)
}

func trimFloat(value float64) string {
	text := fmt.Sprintf("%.8f", value)
	text = trimTrailingZeros(text)

	return text
}

func trimTrailingZeros(text string) string {
	if !containsDot(text) {
		return text
	}

	for len(text) > 0 && text[len(text)-1] == '0' {
		text = text[:len(text)-1]
	}

	if len(text) > 0 && text[len(text)-1] == '.' {
		text = text[:len(text)-1]
	}

	return text
}

func containsDot(text string) bool {
	for index := 0; index < len(text); index++ {
		if text[index] == '.' {
			return true
		}
	}

	return false
}

func encodeBookEnvelope(kind string, payload any) (json.RawMessage, error) {
	data, err := sonic.Marshal(payload)

	if err != nil {
		return nil, err
	}

	envelope := socketEnvelope{
		Channel: bookChannel,
		Type:    kind,
		Data:    data,
	}

	return sonic.Marshal(envelope)
}

func NewPerturbRandom(seed int64) *rand.Rand {
	if seed == 0 {
		return rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	return rand.New(rand.NewSource(seed))
}

func clampSigma(sigma float64) float64 {
	if sigma <= 0 {
		return 0
	}

	return math.Min(sigma, 0.25)
}
