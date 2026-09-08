package replay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
)

// captureFixture exercises the persisted transport format, not market behaviour.
func captureFixture(t testing.TB, count int) (Tape, []hindsight.RawFrame) {
	t.Helper()
	tape := Tape{Directory: t.TempDir()}
	frames := make([]hindsight.RawFrame, count)

	for index := range frames {
		payload := []byte(`{"channel":"heartbeat"}`)
		digest := sha256.Sum256(payload)
		frames[index] = hindsight.RawFrame{
			Identity:   hindsight.CaptureIdentity{Run: "recorded", Sequence: hindsight.CaptureSequence(index + 1)},
			ReceivedAt: time.Unix(100+int64(index), 0), Kind: "heartbeat", Endpoint: "wss://venue",
			Payload: payload, PayloadHash: hex.EncodeToString(digest[:]),
		}
	}
	return tape, frames
}

func writeCaptureFixture(t testing.TB, tape Tape, frames []hindsight.RawFrame) {
	t.Helper()
	file, err := os.Create(filepath.Join(tape.Directory, "00000000000000000001.jsonl"))

	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	encoder := json.NewEncoder(file)

	for _, frame := range frames {
		if err := encoder.Encode(frame); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTapeRead(t *testing.T) {
	Convey("Original capture identities and bytes are verified before delivery", t, func() {
		tape, frames := captureFixture(t, 3)
		Convey("Capture order survives timestamp reversal", func() {
			frames[1].ReceivedAt = frames[0].ReceivedAt.Add(-time.Second)
			writeCaptureFixture(t, tape, frames)
			var received []hindsight.RawFrame
			So(tape.Read(t.Context(), func(frame hindsight.RawFrame) error {
				received = append(received, frame)
				return nil
			}), ShouldBeNil)
			So(received, ShouldResemble, frames)
		})
		Convey("An explicit endpoint ends a contiguous capture prefix", func() {
			writeCaptureFixture(t, tape, frames)
			tape.Through = frames[1].ReceivedAt
			var count int
			So(tape.Read(t.Context(), func(hindsight.RawFrame) error { count++; return nil }), ShouldBeNil)
			So(count, ShouldEqual, 2)
		})
		Convey("Missing records are rejected", func() {
			frames[1].Identity.Sequence++
			writeCaptureFixture(t, tape, frames)
			So(tape.Read(t.Context(), func(hindsight.RawFrame) error { return nil }), ShouldNotBeNil)
		})
		Convey("Altered payloads are rejected", func() {
			frames[0].Payload = []byte(`{}`)
			writeCaptureFixture(t, tape, frames)
			So(tape.Read(t.Context(), func(hindsight.RawFrame) error { return nil }), ShouldNotBeNil)
		})
		Convey("Cancellation stops reading", func() {
			writeCaptureFixture(t, tape, frames)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			So(tape.Read(ctx, func(hindsight.RawFrame) error { return nil }), ShouldNotBeNil)
		})
	})
}

func BenchmarkTapeRead(b *testing.B) {
	directory := os.Getenv("SYMM_REPLAY_CAPTURE_DIR")
	name := "recorded-capture"
	var tape Tape

	if directory == "" {
		// 256 is the repository's configured capture batch size.
		var frames []hindsight.RawFrame
		tape, frames = captureFixture(b, 256)
		writeCaptureFixture(b, tape, frames)
		name = "protocol-fixture"
	}

	if directory != "" {
		tape.Directory = directory

		if through := os.Getenv("SYMM_REPLAY_THROUGH"); through != "" {
			var err error
			tape.Through, err = time.Parse(time.RFC3339Nano, through)

			if err != nil {
				b.Fatal(err)
			}
		}
	}
	b.Run(name, func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			var count int64
			var size int64

			if err := tape.Read(b.Context(), func(frame hindsight.RawFrame) error {
				count++
				size += int64(len(frame.Payload))
				return nil
			}); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(size)
			b.ReportMetric(float64(count), "frames/op")
		}
	})
}
