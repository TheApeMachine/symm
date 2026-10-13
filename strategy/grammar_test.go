package strategy

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGrammar(t *testing.T) {
	Convey("Sequitur Grammar Engine", t, func() {
		Convey("NewGrammar creates an empty grammar with root R0", func() {
			grammar := NewGrammar()
			So(grammar, ShouldNotBeNil)
			So(grammar.StartRule(), ShouldNotBeNil)
			So(grammar.StartRule().ID(), ShouldEqual, 0)
			So(grammar.StartRule().Length(), ShouldEqual, 0)
			So(len(grammar.Rules()), ShouldEqual, 0)
		})

		Convey("Distinct sequence induces no rules", func() {
			grammar := NewGrammar()

			rule1 := grammar.Append("A")
			rule2 := grammar.Append("B")
			rule3 := grammar.Append("C")
			rule4 := grammar.Append("D")

			So(rule1, ShouldBeNil)
			So(rule2, ShouldBeNil)
			So(rule3, ShouldBeNil)
			So(rule4, ShouldBeNil)

			So(len(grammar.Rules()), ShouldEqual, 0)
			So(grammar.StartRule().Length(), ShouldEqual, 4)
		})

		Convey("Repeated digram induces a length-2 rule", func() {
			grammar := NewGrammar()

			grammar.Append("A")
			grammar.Append("B")
			grammar.Append("A")
			activeRule := grammar.Append("B")

			So(activeRule, ShouldNotBeNil)
			So(activeRule.ID(), ShouldEqual, 1)
			So(activeRule.Count(), ShouldEqual, 2)
			So(activeRule.Length(), ShouldEqual, 2)
			So(activeRule.Depth(), ShouldEqual, 1)
			So(activeRule.TerminalNames(grammar), ShouldResemble, []string{"A", "B"})

			rules := grammar.Rules()
			So(len(rules), ShouldEqual, 1)
			So(rules[0].ID(), ShouldEqual, 1)
			So(grammar.StartRule().Length(), ShouldEqual, 2)
		})

		Convey("Hierarchical pattern induces sentences and inlines single-use rules", func() {
			grammar := NewGrammar()

			// Tape sequence: A B C A B C
			grammar.Append("A")
			grammar.Append("B")
			grammar.Append("C")
			grammar.Append("A")
			grammar.Append("B")
			activeRule := grammar.Append("C")

			So(activeRule, ShouldNotBeNil)
			So(activeRule.TerminalNames(grammar), ShouldResemble, []string{"A", "B", "C"})
			So(activeRule.Length(), ShouldEqual, 3)
			So(activeRule.Count(), ShouldEqual, 2)
			So(grammar.StartRule().Length(), ShouldEqual, 2)
			So(len(grammar.Rules()), ShouldEqual, 1)
		})

		Convey("User example: word discovery and nested sentence discovery", func() {
			grammar := NewGrammar()

			// A B A B C A B C
			// 1. A B -> first occurrence
			// 2. A B -> second occurrence -> R1 -> [A, B]
			// 3. C -> R1 C
			// 4. A B C -> triggers R1 then C -> R2 -> [R1, C]
			grammar.Append("A")
			grammar.Append("B")
			grammar.Append("A")
			ruleWord := grammar.Append("B")
			So(ruleWord, ShouldNotBeNil)
			So(ruleWord.TerminalNames(grammar), ShouldResemble, []string{"A", "B"})

			grammar.Append("C")
			grammar.Append("A")
			grammar.Append("B")
			ruleSentence := grammar.Append("C")

			So(ruleSentence, ShouldNotBeNil)
			So(ruleSentence.Depth(), ShouldEqual, 2)
			So(ruleSentence.TerminalNames(grammar), ShouldResemble, []string{"A", "B", "C"})
			So(ruleSentence.String(grammar), ShouldEqual, "R2 -> [R1, C]")
		})

		Convey("Overlapping digrams do not trigger false replacements", func() {
			grammar := NewGrammar()

			// A A A -> overlapping digram (A, A)
			grammar.Append("A")
			grammar.Append("A")
			ruleOverlap := grammar.Append("A")

			So(ruleOverlap, ShouldBeNil)
			So(len(grammar.Rules()), ShouldEqual, 0)
			So(grammar.StartRule().Length(), ShouldEqual, 3)

			// Fourth A -> non-overlapping pair triggers R1 -> [A, A]
			rulePair := grammar.Append("A")
			So(rulePair, ShouldNotBeNil)
			So(rulePair.TerminalNames(grammar), ShouldResemble, []string{"A", "A"})
			So(grammar.StartRule().Length(), ShouldEqual, 2)
		})

		Convey("JSON serialization and deserialization preserves grammar state", func() {
			grammar := NewGrammar()

			grammar.Append("IgnitionBuy")
			grammar.Append("AskVacuum")
			grammar.Append("VolumeSurge")
			grammar.Append("IgnitionBuy")
			grammar.Append("AskVacuum")
			grammar.Append("VolumeSurge")

			So(len(grammar.Rules()), ShouldEqual, 1)
			ruleOriginal := grammar.Rules()[0]
			So(ruleOriginal.TerminalNames(grammar), ShouldResemble, []string{
				"IgnitionBuy", "AskVacuum", "VolumeSurge",
			})

			serialized, err := json.Marshal(grammar)
			So(err, ShouldBeNil)
			So(len(serialized), ShouldBeGreaterThan, 0)

			restored := NewGrammar()
			err = json.Unmarshal(serialized, restored)
			So(err, ShouldBeNil)

			So(len(restored.Rules()), ShouldEqual, 1)
			ruleRestored := restored.Rule(ruleOriginal.ID())
			So(ruleRestored, ShouldNotBeNil)
			So(ruleRestored.TerminalNames(restored), ShouldResemble, []string{
				"IgnitionBuy", "AskVacuum", "VolumeSurge",
			})
			So(restored.StartRule().Length(), ShouldEqual, 2)

			// Restored grammar continues streaming and inducing new rules seamlessly
			restored.Append("IgnitionBuy")
			restored.Append("AskVacuum")
			restoredActive := restored.Append("VolumeSurge")
			So(restoredActive, ShouldNotBeNil)
			So(restoredActive.ID(), ShouldEqual, ruleOriginal.ID())
		})

		Convey("Root rule R0 is never substituted when length is 2", func() {
			grammar := NewGrammar()

			// Start with length-2 sequence in R0: A B
			grammar.Append("A")
			grammar.Append("B")
			So(grammar.StartRule().Length(), ShouldEqual, 2)

			// Append A B again: matches the exact length-2 digram of R0
			grammar.Append("A")
			rule := grammar.Append("B")

			So(rule, ShouldNotBeNil)
			So(rule.ID(), ShouldEqual, 1)
			So(rule.TerminalNames(grammar), ShouldResemble, []string{"A", "B"})
			So(grammar.StartRule().Length(), ShouldEqual, 2)
			So(grammar.StartRule().ID(), ShouldEqual, 0)
		})

		Convey("Chained reduction and recursive rule utility across repetitions", func() {
			grammar := NewGrammar()

			// Sequence: A B C D E A B C D E A B C D E
			for range 3 {
				grammar.Append("A")
				grammar.Append("B")
				grammar.Append("C")
				grammar.Append("D")
				grammar.Append("E")
			}

			rules := grammar.Rules()
			So(len(rules), ShouldBeGreaterThanOrEqualTo, 1)

			// Verify all rules have count >= 2 (Rule Utility invariant)
			for _, rule := range rules {
				So(rule.Count(), ShouldBeGreaterThanOrEqualTo, 2)
				So(rule.Length(), ShouldBeGreaterThanOrEqualTo, 2)
			}
		})
	})
}
