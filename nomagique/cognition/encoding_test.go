package cognition

import (
	"bytes"
	"encoding/gob"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEngineEncode(t *testing.T) {
	Convey("A packed trie round trip preserves inference and subsequent feedback", t, func() {
		engine := NewEngine(DefaultConfig())
		sequence := []byte("BTC\x00precursor\x00ignition\x00")
		engine.Observe(sequence, []byte("enter"), 0.02)
		engine.Observe(sequence, []byte("wait"), -0.01)
		encoded, err := engine.Encode()
		So(err, ShouldBeNil)
		restored := NewEngine(DefaultConfig())
		So(restored.Decode(encoded), ShouldBeNil)
		So(restored.Evaluate(sequence), ShouldResemble, engine.Evaluate(sequence))
		engine.Observe(sequence, []byte("enter"), -0.03)
		restored.Observe(sequence, []byte("enter"), -0.03)
		So(restored.Evaluate(sequence), ShouldResemble, engine.Evaluate(sequence))
		So(restored.Decode(encoded), ShouldNotBeNil)
	})
}

func TestEngineDecode(t *testing.T) {
	Convey("Invalid persistence cannot publish partial model state", t, func() {
		engine := NewEngine(DefaultConfig())
		engine.Observe([]byte("context"), []byte("enter"))
		encoded, err := engine.Encode()
		So(err, ShouldBeNil)
		for _, corrupt := range [][]byte{nil, []byte("retired model"), encoded[:len(encoded)-1]} {
			fresh := NewEngine(DefaultConfig())
			So(fresh.Decode(corrupt), ShouldNotBeNil)
			So(fresh.root.Load().Len(), ShouldEqual, 0)
			So(fresh.stepCounter.Load(), ShouldEqual, 0)
		}

		Convey("Outcome namespace records are rejected rather than treated as packed basins", func() {
			var buffer bytes.Buffer
			encoder := gob.NewEncoder(&buffer)
			for _, field := range []any{"cognition/packed-weight/1", DefaultConfig(), uint64(1), 1, []byte("o/enter/context"), make([]byte, 40)} {
				So(encoder.Encode(field), ShouldBeNil)
			}
			fresh := NewEngine(DefaultConfig())
			So(fresh.Decode(buffer.Bytes()), ShouldNotBeNil)
			So(fresh.root.Load().Len(), ShouldEqual, 0)
		})
	})
}

func BenchmarkEngineEncode(b *testing.B) {
	engine := NewEngine(DefaultConfig())
	for index := range 256 {
		engine.Observe([]byte{byte(index), 0}, []byte("enter"), 0.01)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := engine.Encode(); err != nil {
			b.Fatal(err)
		}
	}
}
