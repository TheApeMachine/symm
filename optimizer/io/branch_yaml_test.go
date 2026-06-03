package io

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestWriteBranchesRejectsEmptyPath(t *testing.T) {
	convey.Convey("Given an empty output path", t, func() {
		err := WriteBranches("", perspectives.BranchList{})

		convey.Convey("It should reject the write", func() {
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "empty perspectives output path")
		})
	})
}

func TestBranchDocumentsFromBranches(t *testing.T) {
	convey.Convey("Given a nested branch list", t, func() {
		value := 2.5
		branches := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       value,
			ValueSet:    true,
			Branches: perspectives.BranchList{{
				Category: perspectives.CategoryExhaustion,
			}},
		}}

		documents := branchDocumentsFromBranches(branches)

		convey.Convey("It should preserve nested branch structure", func() {
			convey.So(len(documents), convey.ShouldEqual, 1)
			convey.So(documents[0].Value, convey.ShouldNotBeNil)
			convey.So(*documents[0].Value, convey.ShouldEqual, value)
			convey.So(len(documents[0].Branches), convey.ShouldEqual, 1)
		})
	})
}

func TestWriteBranchesAtomic(t *testing.T) {
	convey.Convey("Given a valid branch document path", t, func() {
		path := filepath.Join(t.TempDir(), "nested", "perspectives.yaml")
		branches := perspectives.BranchList{{
			Category: perspectives.CategoryLaminar,
			ValueSet: true,
			Value:    1,
		}}

		err := WriteBranches(path, branches)
		raw, readErr := os.ReadFile(path)

		convey.Convey("It should write the file atomically", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(readErr, convey.ShouldBeNil)
			convey.So(string(raw), convey.ShouldContainSubstring, "version: 1")
		})
	})
}
