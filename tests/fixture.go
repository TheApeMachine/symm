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
)

type Fixture interface {
	Generate() iter.Seq[[]byte]
	Frames() iter.Seq[Frame]
}

type Frame struct {
	Channel string
	Type    string
	Payload []byte
}

func FrameSequence(sequence iter.Seq[[]byte]) iter.Seq[Frame] {
	return func(yield func(Frame) bool) {
		for payload := range sequence {
			envelope := struct {
				Channel string `json:"channel"`
				Type    string `json:"type"`
			}{}

			if err := json.Unmarshal(payload, &envelope); err != nil {
				panic(err)
			}

			frame := Frame{
				Channel: envelope.Channel,
				Type:    envelope.Type,
				Payload: append([]byte(nil), payload...),
			}

			if !yield(frame) {
				return
			}
		}
	}
}
