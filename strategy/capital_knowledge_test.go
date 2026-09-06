package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestCapitalKnowledgeReading(t *testing.T) {
	Convey("Allocation knowledge transfers within sources without pooling correlated teachers", t, func() {
		knowledge := NewCapitalKnowledge()
		context := []uint64{11, 22, 0, 1}
		action := CapitalAction{Symbol: "A/USD", Kind: types.ActionEnter, Power: 2}
		for _, target := range []float64{0.01, 0.02, 0.03} {
			So(knowledge.Observe("capital_virtual", context, action, target, 1), ShouldBeNil)
		}

		Convey("An unobserved symbol uses transferable allocation structure and power", func() {
			action.Symbol = "B/USD"
			reading := knowledge.Reading(context, action)
			So(reading.Source, ShouldEqual, "capital_virtual")
			So(reading.Scope, ShouldEqual, "global")
			So(reading.Selected.Samples, ShouldEqual, 3)
			So(reading.Selected.Mean, ShouldAlmostEqual, 0.02, 0.0001)
			So(reading.Selected.ContextLength, ShouldEqual, len(context))
			So(reading.Virtual.Symbol.Defined, ShouldBeFalse)
		})
		Convey("Correlated actual and virtual outcomes remain three samples each, never six", func() {
			for _, target := range []float64{0.01, 0.02, 0.03} {
				So(knowledge.Observe("capital_account", context, action, target, 1), ShouldBeNil)
			}
			reading := knowledge.Reading(context, action)
			So(reading.Virtual.Selected.Samples, ShouldEqual, 3)
			So(reading.Actual.Selected.Samples, ShouldEqual, 3)
			So(reading.Selected.Samples, ShouldEqual, 3)
			So(reading.Selected.Support, ShouldBeLessThanOrEqualTo, 3)
			So(reading.Source, ShouldEqual, "capital_account")
		})
		Convey("A fresh persistent symbol exception can overcome older broader outcomes", func() {
			action.Symbol = "B/USD"
			for range 256 {
				So(knowledge.Observe("capital_virtual", context, action, -0.1, 1), ShouldBeNil)
			}
			reading := knowledge.Reading(context, action)
			So(reading.Scope, ShouldEqual, "symbol")
			So(reading.Selected.Mean, ShouldAlmostEqual, -0.1)
		})
	})
}

func TestCapitalKnowledgeIssue(t *testing.T) {
	Convey("A capital experience binds two symbol-specificity levels inside one source", t, func() {
		knowledge := NewCapitalKnowledge()
		action := CapitalAction{Symbol: "A/USD", Kind: types.ActionEnter}
		ticket, err := knowledge.Issue("capital_account", nil, action, 1)
		So(err, ShouldBeNil)
		_, err = knowledge.Model.Resolve(ticket, 0.1)
		So(err, ShouldBeNil)
		reading := knowledge.Reading(nil, action)
		So(reading.Actual.Global.Samples, ShouldEqual, 1)
		So(reading.Actual.Symbol.Samples, ShouldEqual, 1)
		So(reading.Virtual.Global.Samples, ShouldEqual, 0)
		So(reading.Selected.Samples, ShouldEqual, 1)
	})
}

func BenchmarkCapitalKnowledgeReading(b *testing.B) {
	knowledge := NewCapitalKnowledge()
	action := CapitalAction{Symbol: "A/USD", Kind: types.ActionEnter}
	context := []uint64{11, 22, 0, 1}
	for range 3 {
		if err := knowledge.Observe("capital_virtual", context, action, 0.1, 1); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		knowledge.Reading(context, action)
	}
}

func BenchmarkCapitalKnowledgeIssue(b *testing.B) {
	knowledge := NewCapitalKnowledge()
	action := CapitalAction{Symbol: "A/USD", Kind: types.ActionEnter}
	context := []uint64{11, 22, 0, 1}
	b.ReportAllocs()
	for b.Loop() {
		ticket, err := knowledge.Issue("capital_account", context, action, 1)
		if err != nil {
			b.Fatal(err)
		}
		if err := knowledge.Model.Abort(ticket); err != nil {
			b.Fatal(err)
		}
	}
}
