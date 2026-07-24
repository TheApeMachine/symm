package types

import (
	"time"

	"github.com/bytedance/sonic"
)

/*
WireVersion is the negotiated UI and actor envelope schema generation.
Bump when keys, shapes, or semantics change incompatibly.
*/
const WireVersion uint16 = 1

/*
WireEnvelope is the versioned frame root for hub and actor ingress. Payload
keys remain stream names; Version rejects incompatible clients at the boundary.
*/
type WireEnvelope struct {
	Version    uint16         `json:"v"`
	Generation uint64         `json:"g,omitempty"`
	At         time.Time      `json:"at,omitempty"`
	Payload    map[string]any `json:"payload"`
}

/*
NewWireEnvelope wraps a payload map under the current WireVersion.
*/
func NewWireEnvelope(generation uint64, payload map[string]any) WireEnvelope {
	return WireEnvelope{
		Version:    WireVersion,
		Generation: generation,
		At:         time.Now().UTC(),
		Payload:    payload,
	}
}

/*
Compatible reports whether the peer version can be decoded by this process.
*/
func (envelope WireEnvelope) Compatible() bool {
	return envelope.Version == WireVersion
}

/*
WireMeasurements publishes a non-blocking UI frame for one signal's measurement
batch. A full channel drops the frame rather than stalling measure.
*/
func WireMeasurements(rows []*Measurement, ui chan []byte) {
	if len(rows) == 0 || ui == nil {
		return
	}

	frame, err := sonic.Marshal(map[string]any{
		"measurements": ForPublish(rows),
	})

	if err != nil {
		return
	}

	select {
	case ui <- frame:
	default:
	}
}
