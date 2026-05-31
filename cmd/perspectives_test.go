package cmd

import (
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestConfigurePerspectivesUsesBuiltinWhenFileMissing(t *testing.T) {
	Convey("Given a perspective path that does not exist", t, func() {
		decision.RestoreDefaultPerspectiveRegistry()
		defer decision.RestoreDefaultPerspectiveRegistry()

		missing := filepath.Join(t.TempDir(), "missing-perspectives.yaml")

		Convey("It should keep the Go builtin registry", func() {
			So(configurePerspectives(missing), ShouldBeNil)

			decisions := decision.Decisions(
				[]perspectives.Measurement{
					{Category: perspectives.CategoryRiskOnSurge, SNR: 1.3},
					{Category: perspectives.CategoryEndogenousAlpha, SNR: 1.4},
					{Category: perspectives.CategoryFrenzy, SNR: 1.2},
					{Category: perspectives.CategoryAggressiveDrive, SNR: 1.6},
				},
				nil,
			)

			So(decisions, ShouldNotBeEmpty)
			So(decisions[0].Name, ShouldEqual, "trend")
		})
	})
}

func TestConfigurePerspectivesLoadsYAMLWhenPresent(t *testing.T) {
	Convey("Given a valid perspectives YAML file", t, func() {
		decision.RestoreDefaultPerspectiveRegistry()
		defer decision.RestoreDefaultPerspectiveRegistry()

		path := filepath.Join(t.TempDir(), "perspectives.yaml")
		So(perspectives.SaveDocumentFile(path, perspectives.BuiltinDocument()), ShouldBeNil)

		Convey("It should replace the builtin registry", func() {
			So(configurePerspectives(path), ShouldBeNil)
		})
	})
}
