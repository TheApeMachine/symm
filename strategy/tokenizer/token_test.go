package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewToken(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewToken wraps composite symbols", func() {
			token := NewToken([]string{"R09", "R03", "R13", "R06"}, 0.8)

			Convey("String joins symbols with dashes", func() {
				So(token.String(), ShouldEqual, "[R09-R03-R13-R06]")
				So(token.Brightness, ShouldEqual, 0.8)
			})
		})
	})
}

func TestNewTokens(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewTokens builds an empty cut", func() {
			cut := NewTokens()

			Convey("Len is zero and String is empty", func() {
				So(cut.Len(), ShouldEqual, 0)
				So(cut.String(), ShouldEqual, "")
			})
		})
	})
}

func BenchmarkNewToken(b *testing.B) {
	syms := []string{"R09", "R03", "R13", "R06"}

	b.ReportAllocs()

	for b.Loop() {
		tok := NewToken(syms, 0.8)
		_ = tok.String()
	}
}

func BenchmarkNewTokens(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		cut := NewTokens()
		_ = cut.Len()
	}
}
