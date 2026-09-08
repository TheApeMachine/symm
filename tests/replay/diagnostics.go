package replay

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/pion/webrtc/v4"
	"github.com/theapemachine/errnie"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func (observer *Observer) readDiagnostics(ctx context.Context, url string) error {
	connection, err := webrtc.NewPeerConnection(webrtc.Configuration{})

	if err != nil {
		return errnie.Error(err)
	}
	defer func() {
		if err := connection.Close(); err != nil {
			errnie.Error(err)
		}
	}()
	ordered, retransmits := false, uint16(0)
	channel, err := connection.CreateDataChannel("diagnostics", &webrtc.DataChannelInit{Ordered: &ordered, MaxRetransmits: &retransmits})

	if err != nil {
		return errnie.Error(err)
	}
	failed := make(chan error, 1)
	var record diagnosticRecord
	channel.OnMessage(func(message webrtc.DataChannelMessage) {
		payload, err := record.Accept(message.Data)

		if err != nil {
			select {
			case failed <- err:
			default:
				errnie.Error(err)
			}
			return
		}

		if payload == nil {
			return
		}
		envelope := wire.GetRootAsEnvelope(payload, 0)
		var table flatbuffers.Table

		if !envelope.Frame(&table) || envelope.FrameType() != wire.FrameEnvelopeStateFrame {
			failed <- errnie.Error(errnie.Err(errnie.Validation, "replay: invalid diagnostics envelope", nil))
			return
		}
		var frame wire.EnvelopeStateFrame
		frame.Init(table.Bytes, table.Pos)
		state := frame.State(nil)

		for index := range state.BoundariesLength() {
			var boundary wire.EnvelopeBoundaryStamp
			state.Boundaries(&boundary, index)

			if string(boundary.Label()) == "learning" && boundary.SeqCount() > observer.Completed.Load() {
				observer.Completed.Store(boundary.SeqCount())
				observer.First.CompareAndSwap(0, boundary.AtNs())
				observer.Last.Store(boundary.AtNs())
				observer.Backlog.Store(boundary.Backlog())
			}
		}
	})
	offer, err := connection.CreateOffer(nil)

	if err != nil {
		return errnie.Error(err)
	}
	gathered := webrtc.GatheringCompletePromise(connection)

	if err := connection.SetLocalDescription(offer); err != nil {
		return errnie.Error(err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-gathered:
	}
	payload, err := json.Marshal(connection.LocalDescription())

	if err != nil {
		return errnie.Error(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))

	if err != nil {
		return errnie.Error(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)

	if err != nil {
		return errnie.Error(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			errnie.Error(err)
		}
	}()

	if response.StatusCode != http.StatusOK {
		return errnie.Error(fmt.Errorf("replay: diagnostics signaling returned %s", response.Status))
	}
	var answer webrtc.SessionDescription

	if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
		return errnie.Error(err)
	}

	if err := connection.SetRemoteDescription(answer); err != nil {
		return errnie.Error(err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-failed:
		return err
	}
}

// diagnosticRecord implements the existing SFD1 segmented, latest-wins wire
// contract used by the dashboard. Partial superseded records are replaceable.
type diagnosticRecord struct {
	identity uint32
	parts    [][]byte
	received int
}

func (record *diagnosticRecord) Accept(segment []byte) ([]byte, error) {
	if len(segment) < 16 || string(segment[:4]) != "SFD1" {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "replay: invalid diagnostics segment", nil))
	}
	identity := binary.LittleEndian.Uint32(segment[4:8])
	index := binary.LittleEndian.Uint32(segment[8:12])
	count := binary.LittleEndian.Uint32(segment[12:16])

	if count == 0 || index >= count {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "replay: invalid diagnostics segment index", nil))
	}

	if int32(identity-record.identity) < 0 {
		return nil, nil
	}

	if identity != record.identity || record.parts == nil {
		record.identity, record.parts, record.received = identity, make([][]byte, count), 0
	}

	if len(record.parts) != int(count) {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "replay: inconsistent diagnostics segment count", nil))
	}

	if record.parts[index] != nil {
		return nil, nil
	}
	record.parts[index] = bytes.Clone(segment[16:])
	record.received++

	if record.received != len(record.parts) {
		return nil, nil
	}
	return bytes.Join(record.parts, nil), nil
}
