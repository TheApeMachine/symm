package ui

import (
	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/types"
)

/*
unwrapWirePayload accepts versioned envelopes or legacy flat frames so retain
and Publish keep working while producers migrate to WireEnvelope.
*/
func unwrapWirePayload(msg []byte) (map[string]sonic.NoCopyRawMessage, error) {
	var envelope types.WireEnvelope

	if err := sonic.Unmarshal(msg, &envelope); err == nil && envelope.Version != 0 {
		if !envelope.Compatible() {
			return nil, types.VersionError{Want: types.WireVersion, Got: envelope.Version}
		}

		encoded, err := sonic.Marshal(envelope.Payload)

		if err != nil {
			return nil, err
		}

		var frame map[string]sonic.NoCopyRawMessage

		if err := sonic.Unmarshal(encoded, &frame); err != nil {
			return nil, err
		}

		return frame, nil
	}

	var frame map[string]sonic.NoCopyRawMessage

	if err := sonic.Unmarshal(msg, &frame); err != nil {
		return nil, err
	}

	return frame, nil
}

/*
wireFrame wraps a cached payload under the wire envelope so clients can enforce
generation-aware ordering against reconnect replay.
*/
func wireFrame(frame cachedFrame) ([]byte, error) {
	var payload map[string]any

	if err := sonic.Unmarshal(frame.payload, &payload); err != nil {
		return nil, err
	}

	return sonic.Marshal(types.NewWireEnvelope(frame.generation, payload))
}

/*
WirePublish wraps a payload under the current wire version for typed ingress.
*/
func WirePublish(generation uint64, payload map[string]any) ([]byte, error) {
	envelope := types.NewWireEnvelope(generation, payload)

	if !envelope.Compatible() {
		return nil, types.VersionError{Want: types.WireVersion, Got: envelope.Version}
	}

	return sonic.Marshal(envelope)
}
