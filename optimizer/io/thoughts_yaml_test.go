package io

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestWriteThoughts(t *testing.T) {
	convey.Convey("Given a reasoning forest", t, func() {
		path := filepath.Join(t.TempDir(), "perspectives.yaml")
		thoughts := []reasoning.Thought{
			{
				When: reasoning.Predicate{
					Subject:   reasoning.SubjectPosition,
					Op:        reasoning.ComparisonEquals,
					Lifecycle: types.ObservationNotHolding,
				},
				Do: reasoning.Act{Type: reasoning.ActionMarket},
			},
		}

		convey.Convey("It should write a parseable playbook file", func() {
			convey.So(WriteThoughts(path, thoughts), convey.ShouldBeNil)

			raw, readErr := os.ReadFile(path)

			convey.So(readErr, convey.ShouldBeNil)

			reparsed, parseErr := reasoning.ParseThoughts(raw)

			convey.So(parseErr, convey.ShouldBeNil)
			convey.So(reparsed, convey.ShouldResemble, thoughts)
		})
	})
}

func BenchmarkWriteThoughts(b *testing.B) {
	path := filepath.Join(b.TempDir(), "perspectives.yaml")
	thoughts := []reasoning.Thought{
		{
			When: reasoning.Predicate{
				Subject:   reasoning.SubjectPosition,
				Op:        reasoning.ComparisonEquals,
				Lifecycle: types.ObservationNotHolding,
			},
			Do: reasoning.Act{Type: reasoning.ActionMarket},
		},
		{
			When: reasoning.Predicate{
				Subject:   reasoning.SubjectPosition,
				Op:        reasoning.ComparisonEquals,
				Lifecycle: types.ObservationHolding,
			},
			Do: reasoning.Act{Type: reasoning.ActionTrailingStop, Offset: 0.02},
		},
	}

	for b.Loop() {
		if err := WriteThoughts(path, thoughts); err != nil {
			b.Fatal(err)
		}
	}
}
