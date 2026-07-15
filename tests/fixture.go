package tests

import (
	"encoding/json"
	"iter"
)

type FixtureType string

const (
	FixtureTypeTicker     FixtureType = "ticker"
	FixtureTypeBook       FixtureType = "book"
	FixtureTypeCandles    FixtureType = "candles"
	FixtureTypeTrade      FixtureType = "trade"
	FixtureTypeInstrument FixtureType = "instrument"
	FixtureTypeStatus     FixtureType = "status"
	FixtureTypeHeartbeat  FixtureType = "heartbeat"
	FixtureTypeExecution  FixtureType = "execution"
	FixtureTypeOrderAck   FixtureType = "orderack"
)

type Fixture interface {
	Generate() iter.Seq[[]byte]
	Frames() iter.Seq[Frame]
}

/*
PayloadFixture yields Kraken payloads that are not routed by channel name, such
as private add_order acknowledgements used by broker position tests.
*/
type PayloadFixture interface {
	Generate() iter.Seq[[]byte]
}

/*
StaticSequence replays one fixed payload list in order for scripted broker tests.
*/
type StaticSequence struct {
	payloads [][]byte
}

/*
NewStaticSequence builds an ordered payload fixture from explicit test inputs.
*/
func NewStaticSequence(payloads ...[]byte) *StaticSequence {
	copied := make([][]byte, len(payloads))

	for index, payload := range payloads {
		copied[index] = append([]byte(nil), payload...)
	}

	return &StaticSequence{payloads: copied}
}

func (sequence *StaticSequence) Generate() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for _, payload := range sequence.payloads {
			if !yield(payload) {
				return
			}
		}
	}
}

type Frame struct {
	Channel string
	Type    string
	Payload []byte
}

func FrameSequence(sequence iter.Seq[[]byte]) iter.Seq[Frame] {
	return func(yield func(Frame) bool) {
		for payload := range sequence {
			frame, ok := frameFromPayload(payload)

			if !ok {
				continue
			}

			if !yield(frame) {
				return
			}
		}
	}
}

func frameFromPayload(payload []byte) (Frame, bool) {
	envelope := struct {
		Channel string `json:"channel"`
		Type    string `json:"type"`
	}{}

	if err := json.Unmarshal(payload, &envelope); err != nil {
		return Frame{}, false
	}

	if envelope.Channel == "" {
		return Frame{}, false
	}

	return Frame{
		Channel: envelope.Channel,
		Type:    envelope.Type,
		Payload: append([]byte(nil), payload...),
	}, true
}
