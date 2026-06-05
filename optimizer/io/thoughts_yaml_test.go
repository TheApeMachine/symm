package io

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestWriteThoughts(t *testing.T) {
	convey.Convey("Given a reasoning forest", t, func() {
		path := filepath.Join(t.TempDir(), "perspectives.yaml")
		thoughts := []perspectives.Thought{
			{
				When: perspectives.Predicate{
					Subject:   perspectives.SubjectPosition,
					Op:        perspectives.ComparisonEquals,
					Lifecycle: perspectives.ObservationNotHolding,
				},
				Do: perspectives.Act{Type: perspectives.ActionMarket},
			},
		}

		convey.Convey("It should write a parseable playbook file", func() {
			convey.So(WriteThoughts(path, thoughts), convey.ShouldBeNil)

			raw, readErr := os.ReadFile(path)

			convey.So(readErr, convey.ShouldBeNil)

			reparsed, parseErr := perspectives.ParseThoughts(raw)

			convey.So(parseErr, convey.ShouldBeNil)
			convey.So(reparsed, convey.ShouldResemble, thoughts)
		})
	})
}

func BenchmarkWriteThoughts(b *testing.B) {
	path := filepath.Join(b.TempDir(), "perspectives.yaml")
	thoughts := []perspectives.Thought{
		{
			When: perspectives.Predicate{
				Subject:   perspectives.SubjectPosition,
				Op:        perspectives.ComparisonEquals,
				Lifecycle: perspectives.ObservationNotHolding,
			},
			Do: perspectives.Act{Type: perspectives.ActionMarket},
		},
		{
			When: perspectives.Predicate{
				Subject:   perspectives.SubjectPosition,
				Op:        perspectives.ComparisonEquals,
				Lifecycle: perspectives.ObservationHolding,
			},
			Do: perspectives.Act{Type: perspectives.ActionTrailingStop, Offset: 0.02},
		},
	}

	for b.Loop() {
		if err := WriteThoughts(path, thoughts); err != nil {
			b.Fatal(err)
		}
	}
}
