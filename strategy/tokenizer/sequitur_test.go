package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAnalyze(t *testing.T) {
	Convey("Given a Sequitur", t, func() {
		seq := NewSequitur()

		Convey("When analyzing tokens that form a repeated digram", func() {
			seq.Analyze([]string{"R01"})
			seq.Analyze([]string{"R02"})
			seq.Analyze([]string{"R01"})
			rule, isBoundary := seq.Analyze([]string{"R02"})

			Convey("It should detect a boundary and return the new rule", func() {
				So(isBoundary, ShouldBeTrue)
				So(rule, ShouldNotBeNil)
				So(seq.RuleCount(), ShouldBeGreaterThan, 0)
				So(seq.TerminalNames(rule), ShouldResemble, []string{"R01", "R02"})
			})
		})

		Convey("When analyzing tokens without repetition", func() {
			_, isBoundary := seq.Analyze([]string{"R08"})
			So(isBoundary, ShouldBeFalse)

			_, isBoundary = seq.Analyze([]string{"R09"})
			So(isBoundary, ShouldBeFalse)
		})

		Convey("When analyzing a chord in one call", func() {
			seq := NewSequitur()
			seq.Analyze([]string{"R01", "R02"})
			rule, isBoundary := seq.Analyze([]string{"R01", "R02"})

			So(isBoundary, ShouldBeTrue)
			So(rule, ShouldNotBeNil)
		})
	})
}

func TestFlush(t *testing.T) {
	Convey("Given a Sequitur with pending tokens", t, func() {
		seq := NewSequitur()
		seq.Analyze([]string{"R01"})
		seq.Analyze([]string{"R02"})

		Convey("When flushing", func() {
			hasPending := seq.Flush()

			Convey("It should report pending tokens", func() {
				So(hasPending, ShouldBeTrue)
			})
		})
	})

	Convey("Given a Sequitur with no pending tokens after a boundary", t, func() {
		seq := NewSequitur()
		seq.Analyze([]string{"R01", "R02", "R01", "R02"})

		Convey("When flushing", func() {
			hasPending := seq.Flush()

			Convey("It should return no pending", func() {
				So(hasPending, ShouldBeFalse)
			})
		})
	})
}

func TestRuleCount(t *testing.T) {
	Convey("Given a Sequitur", t, func() {
		seq := NewSequitur()

		Convey("When no rules have been created", func() {
			count := seq.RuleCount()

			Convey("It should report zero rules", func() {
				So(count, ShouldEqual, 0)
			})
		})

		Convey("When a repeated digram creates a rule", func() {
			seq.Analyze([]string{"R01", "R02", "R01", "R02"})

			count := seq.RuleCount()

			Convey("It should report at least one rule", func() {
				So(count, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func BenchmarkAnalyze(b *testing.B) {
	seq := NewSequitur()
	chord := []string{"R01", "R02", "R03", "R04"}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		for range 4 {
			seq.Analyze(chord)
		}
	}
}

func BenchmarkFlush(b *testing.B) {
	seq := NewSequitur()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		seq.Analyze([]string{"R01", "R02"})
		seq.Flush()
	}
}

func BenchmarkRuleCount(b *testing.B) {
	seq := NewSequitur()

	for range 8 {
		seq.Analyze([]string{"R01", "R02"})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = seq.RuleCount()
	}
}
