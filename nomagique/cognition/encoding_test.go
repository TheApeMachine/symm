package cognition

import (
	"bytes"
	"encoding/gob"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/transport"
)

func TestEngineSnapshot(t *testing.T) {
	Convey("A packed trie round trip preserves inference and subsequent feedback", t, func() {
		engine := NewEngine(Config{})
		sequence := []byte("BTC\x00precursor\x00ignition\x00")

		observing(t, engine, sequence, []byte("enter"), 0.02)
		observing(t, engine, sequence, []byte("wait"), -0.01)

		encoded := drive(t, engine, &Command{Snapshot: &Snapshot{}}).Model
		So(len(encoded), ShouldBeGreaterThan, 0)

		restored := NewEngine(Config{})
		So(drive(t, restored, &Command{Restore: encoded}).Tree.Len(), ShouldEqual,
			drive(t, engine, &Command{Root: &Root{}}).Tree.Len())
		So(evaluating(t, restored, sequence), ShouldResemble, evaluating(t, engine, sequence))

		observing(t, engine, sequence, []byte("enter"), -0.03)
		observing(t, restored, sequence, []byte("enter"), -0.03)
		So(evaluating(t, restored, sequence), ShouldResemble, evaluating(t, engine, sequence))

		invalid := transport.NewEvaluate(restored)
		command := Command{Restore: encoded}

		for range invalid.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)) {
			t.Fatal("restoring over a used engine must not yield")
		}

		So(invalid.Error(), ShouldNotBeNil)
	})
}

func TestEngineRestore(t *testing.T) {
	Convey("Invalid persistence cannot publish partial model state", t, func() {
		engine := NewEngine(Config{})
		observing(t, engine, []byte("context"), []byte("enter"))
		encoded := drive(t, engine, &Command{Snapshot: &Snapshot{}}).Model

		for _, corrupt := range [][]byte{nil, []byte("retired model"), encoded[:len(encoded)-1]} {
			fresh := NewEngine(Config{})
			So(drive(t, fresh, &Command{Root: &Root{}}).Tree.Len(), ShouldEqual, 0)

			attempt := transport.NewEvaluate(fresh)
			command := Command{Restore: corrupt}

			for range attempt.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)) {
			}

			So(attempt.Error(), ShouldNotBeNil)
		}

		Convey("Outcome namespace records are rejected rather than treated as packed basins", func() {
			var buffer bytes.Buffer
			encoder := gob.NewEncoder(&buffer)

			for _, field := range []any{
				"cognition/packed-weight/1",
				defaultBounds(),
				uint64(1),
				1,
				[]byte("o/enter/context"),
				make([]byte, WeightSize),
			} {
				So(encoder.Encode(field), ShouldBeNil)
			}

			fresh := NewEngine(Config{})
			So(drive(t, fresh, &Command{Root: &Root{}}).Tree.Len(), ShouldEqual, 0)

			attempt := transport.NewEvaluate(fresh)
			command := Command{Restore: buffer.Bytes()}

			for range attempt.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)) {
			}

			So(attempt.Error(), ShouldNotBeNil)
		})
	})
}

func BenchmarkEngineSnapshot(b *testing.B) {
	engine := NewEngine(Config{}).(*Engine)

	for index := range 256 {
		engine.observe(Association{
			Context:  []byte{byte(index), 0},
			Class:    []byte("enter"),
			Feedback: 0.01,
			Graded:   true,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := engine.snapshot(); err != nil {
			b.Fatal(err)
		}
	}
}
