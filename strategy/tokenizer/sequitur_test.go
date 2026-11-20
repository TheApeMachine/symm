package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRuleRef(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When RuleRef formats rule ids", func() {
			Convey("Ref is M-prefixed", func() {
				So(RuleRef(1), ShouldEqual, "M1")
				So(RuleRef(12), ShouldEqual, "M12")
			})
		})
	})
}

func TestNewSequitur(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewSequitur is called", func() {
			seq := NewSequitur()

			Convey("RuleCount starts at zero", func() {
				So(seq, ShouldNotBeNil)
				So(seq.RuleCount(), ShouldEqual, 0)
				So(seq.RootSequence(), ShouldResemble, []string{})
			})
		})
	})
}

func TestAnalyze(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequitur()

		Convey("When a repeated digram appears", func() {
			seq.Analyze([]string{"R01"})
			seq.Analyze([]string{"R02"})
			seq.Analyze([]string{"R01"})
			rule, isBoundary := seq.Analyze([]string{"R02"})

			Convey("Boundary yields a rule for R01 R02", func() {
				So(isBoundary, ShouldBeTrue)
				So(rule, ShouldNotBeNil)
				So(seq.RuleCount(), ShouldEqual, 1)
				So(seq.TerminalNames(rule), ShouldResemble, []string{"R01", "R02"})
			})
		})

		Convey("When tokens do not repeat", func() {
			_, first := seq.Analyze([]string{"R08"})
			_, second := seq.Analyze([]string{"R09"})

			Convey("Analyze returns no boundary", func() {
				So(first, ShouldBeFalse)
				So(second, ShouldBeFalse)
			})
		})
	})
}

func TestTail(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequitur()
		seq.Analyze([]string{"R01", "R02", "R03"})

		Convey("When Tail requests the last two symbols", func() {
			tail := seq.Tail(2)

			Convey("Tail returns the suffix", func() {
				So(tail, ShouldResemble, []string{"R02", "R03"})
			})
		})
	})
}

func TestFlush(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When pending tokens remain", func() {
			seq := NewSequitur()
			seq.Analyze([]string{"R01"})
			seq.Analyze([]string{"R02"})

			Convey("Flush reports pending", func() {
				So(seq.Flush(), ShouldBeTrue)
			})
		})

		Convey("When no pending tokens remain after a boundary", func() {
			seq := NewSequitur()
			seq.Analyze([]string{"R01", "R02", "R01", "R02"})

			Convey("Flush returns false", func() {
				So(seq.Flush(), ShouldBeFalse)
			})
		})
	})
}

func TestRuleCount(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequitur()

		Convey("When a digram rule is created", func() {
			seq.Analyze([]string{"R01", "R02", "R01", "R02"})

			Convey("RuleCount is one and LastKind is create", func() {
				So(seq.RuleCount(), ShouldEqual, 1)
				So(seq.LastKind(), ShouldEqual, "create")
			})
		})

		Convey("When the same digram repeats after the rule exists", func() {
			seq.Analyze([]string{"R01", "R02", "R01", "R02"})
			rules := seq.RuleCount()
			seq.Analyze([]string{"R09", "R01", "R02"})

			Convey("Rule is reused without minting another", func() {
				So(seq.LastKind(), ShouldEqual, "reuse")
				So(seq.RuleCount(), ShouldEqual, rules)
				root := seq.RootSequence()
				So(root[len(root)-1], ShouldEqual, "M1")
			})
		})
	})
}

func TestRootSequence(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequitur()
		seq.Analyze([]string{"R05"})
		seq.Analyze([]string{"R06"})

		Convey("When RootSequence is read", func() {
			Convey("Root lists terminal symbols", func() {
				So(seq.RootSequence(), ShouldResemble, []string{"R05", "R06"})
			})
		})
	})
}

func TestTerminalNames(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequitur()
		rule, ok := seq.Analyze([]string{"R01", "R02", "R01", "R02"})
		So(ok, ShouldBeTrue)

		Convey("When TerminalNames expands a rule", func() {
			Convey("Names match the digram terminals", func() {
				So(seq.TerminalNames(rule), ShouldResemble, []string{"R01", "R02"})
			})
		})
	})
}

func TestTerminalsForRef(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequitur()
		seq.Analyze([]string{"R01", "R02", "R01", "R02"})

		Convey("When TerminalsForRef looks up M1", func() {
			Convey("Ref resolves to terminals", func() {
				So(seq.TerminalsForRef("M1"), ShouldResemble, []string{"R01", "R02"})
			})
		})
	})
}

func TestRuleViews(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequitur()
		seq.Analyze([]string{"R01", "R02", "R01", "R02"})

		Convey("When RuleViews exports rules", func() {
			views := seq.RuleViews()

			Convey("One rule view carries the digram", func() {
				So(len(views), ShouldEqual, 1)
				So(len(views[0].Symbols), ShouldEqual, 2)
			})
		})
	})
}

func TestSymbols(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequitur()
		seq.Analyze([]string{"R01", "R02", "R01", "R02"})

		Convey("When Symbols counts rules plus one", func() {
			Convey("Count is rule count plus the terminal slot", func() {
				So(seq.Symbols(), ShouldEqual, seq.RuleCount()+1)
			})
		})
	})
}

func TestRules(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequitur()
		seq.Analyze([]string{"R01", "R02", "R01", "R02"})

		Convey("When Rules lists productions", func() {
			rules := seq.Rules()

			Convey("One production lists two terminals", func() {
				So(len(rules), ShouldEqual, 1)
				So(rules[0], ShouldResemble, []string{"R01", "R02"})
			})
		})
	})
}

func TestSentence(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequitur()
		seq.Analyze([]string{"R01", "R02"})

		Convey("When Sentence starts a new phrase", func() {
			seq.Sentence()

			Convey("Root sequence clears", func() {
				So(len(seq.RootSequence()), ShouldEqual, 0)
			})
		})
	})
}

func TestLearn(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequitur()

		Convey("When Learn restores a production", func() {
			seq.Learn(1, 2, []string{"R01", "R02"})

			Convey("RuleViews contains the production", func() {
				views := seq.RuleViews()
				So(len(views), ShouldEqual, 1)
				So(views[0].Symbols, ShouldResemble, []string{"R01", "R02"})
			})
		})
	})
}

func TestRestoreRoot(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequitur()
		seq.Learn(1, 1, []string{"R01", "R02"})

		Convey("When RestoreRoot installs a sentence", func() {
			seq.RestoreRoot([]string{"M1", "R03"})

			Convey("Root matches the restored sequence", func() {
				So(seq.RootSequence(), ShouldResemble, []string{"M1", "R03"})
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

func BenchmarkRuleRef(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = RuleRef(12)
	}
}

func BenchmarkNewSequitur(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewSequitur()
	}
}

func BenchmarkTail(b *testing.B) {
	seq := NewSequitur()
	seq.Analyze([]string{"R01", "R02", "R03"})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = seq.Tail(2)
	}
}

func BenchmarkRootSequence(b *testing.B) {
	seq := NewSequitur()
	seq.Analyze([]string{"R05", "R06"})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = seq.RootSequence()
	}
}

func BenchmarkTerminalNames(b *testing.B) {
	seq := NewSequitur()
	rule, _ := seq.Analyze([]string{"R01", "R02", "R01", "R02"})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = seq.TerminalNames(rule)
	}
}

func BenchmarkTerminalsForRef(b *testing.B) {
	seq := NewSequitur()
	seq.Analyze([]string{"R01", "R02", "R01", "R02"})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = seq.TerminalsForRef("M1")
	}
}

func BenchmarkRuleViews(b *testing.B) {
	seq := NewSequitur()
	seq.Analyze([]string{"R01", "R02", "R01", "R02"})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = seq.RuleViews()
	}
}

func BenchmarkSymbols(b *testing.B) {
	seq := NewSequitur()
	seq.Analyze([]string{"R01", "R02", "R01", "R02"})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = seq.Symbols()
	}
}

func BenchmarkRules(b *testing.B) {
	seq := NewSequitur()
	seq.Analyze([]string{"R01", "R02", "R01", "R02"})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = seq.Rules()
	}
}

func BenchmarkSentence(b *testing.B) {
	seq := NewSequitur()
	seq.Analyze([]string{"R01", "R02"})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		seq.Sentence()
	}
}

func BenchmarkLearn(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		seq := NewSequitur()
		seq.Learn(1, 2, []string{"R01", "R02"})
	}
}

func BenchmarkRestoreRoot(b *testing.B) {
	seq := NewSequitur()
	seq.Learn(1, 1, []string{"R01", "R02"})
	root := []string{"M1", "R03"}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		seq.RestoreRoot(root)
	}
}
