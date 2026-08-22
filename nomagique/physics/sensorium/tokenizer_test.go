package sensorium

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTokenizerMakeBatch(t *testing.T) {
	Convey("Given one byte at sequence 0", t, func() {
		tokenizer, err := NewTokenizer(64, 64, 64, 64)
		So(err, ShouldBeNil)
		state, err := tokenizer.MakeBatch([]int64{0}, []int64{0})
		So(err, ShouldBeNil)

		Convey("It should emit the original ω, token_id, and grid pose", func() {
			So(state.N, ShouldEqual, 1)
			So(state.TokenIDs[0], ShouldEqual, int64(0))
			So(state.Omega[0], ShouldEqual, float32(-4))
			So(state.Energy[0], ShouldEqual, float32(1))
			So(state.Mass[0], ShouldEqual, float32(1))
			So(state.Heat[0], ShouldEqual, float32(0))
			So(state.Pos[0], ShouldEqual, float32(0))
			So(state.Phase[0], ShouldEqual, float32(0))
		})
	})

	Convey("Given byte 255 at sequence 256", t, func() {
		tokenizer, err := NewTokenizer(64, 64, 64, 256)
		So(err, ShouldBeNil)
		state, err := tokenizer.MakeBatch([]int64{255}, []int64{256})
		So(err, ShouldBeNil)

		Convey("Relative wrap should share token_id 255 with a 1/32 beat", func() {
			So(state.N, ShouldEqual, 1)
			So(state.TokenIDs[0], ShouldEqual, int64(255))
			So(float64(state.Omega[0]), ShouldAlmostEqual, 4.0, 1e-5)
			So(float64(state.Phase[0]), ShouldAlmostEqual, math.Pi/32, 1e-5)
		})
	})

	Convey("Given duplicate (byte, seq) pairs", t, func() {
		tokenizer, err := NewTokenizer(64, 64, 64, 64)
		So(err, ShouldBeNil)
		state, err := tokenizer.MakeBatch(
			[]int64{1, 1, 2},
			[]int64{0, 0, 0},
		)
		So(err, ShouldBeNil)

		Convey("Collision-is-compression should fold mass into one particle", func() {
			So(state.N, ShouldEqual, 2)
			So(state.Bytes[0], ShouldEqual, int64(1))
			So(state.Mass[0], ShouldEqual, float32(2))
			So(state.Bytes[1], ShouldEqual, int64(2))
			So(state.Mass[1], ShouldEqual, float32(1))
		})
	})
}

func TestTokenizerMakeDarkBatch(t *testing.T) {
	Convey("Given a vacuum superposition at sequence 3", t, func() {
		tokenizer, err := NewTokenizer(64, 64, 64, 64)
		So(err, ShouldBeNil)
		state := tokenizer.MakeDarkBatch(3, 1e-6)

		Convey("It should seed every byte without compressing", func() {
			So(state.N, ShouldEqual, 256)
			So(state.Bytes[0], ShouldEqual, int64(0))
			So(state.Bytes[255], ShouldEqual, int64(255))
			So(state.Mass[0], ShouldEqual, float32(1e-6))
			So(state.Energy[0], ShouldEqual, float32(1e-6))
			So(state.Heat[0], ShouldEqual, float32(1))
			So(state.TokenIDs[3], ShouldEqual, int64((3<<8)|3))
			So(state.Dark[0], ShouldBeTrue)
		})
	})
}

func TestCompressorFilter(t *testing.T) {
	Convey("Compressor.Filter should return nothing for empty input", t, func() {
		tokenizer, err := NewTokenizer(64, 64, 64, 64)
		So(err, ShouldBeNil)
		bytes, seqs, counts := tokenizer.Compressor.Filter(nil, nil)

		So(bytes, ShouldBeEmpty)
		So(seqs, ShouldBeEmpty)
		So(counts, ShouldBeEmpty)
	})
}

func TestNewTokenizer(t *testing.T) {
	Convey("Given a zero grid", t, func() {
		_, err := NewTokenizer(0, 64, 64, 64)

		Convey("It should refuse to build", func() {
			So(err, ShouldNotBeNil)
		})
	})
}
