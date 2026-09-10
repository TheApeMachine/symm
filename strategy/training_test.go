package strategy

import (
	"context"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
)

/*
recordedArchive opens the capture catalog the inspection view reads. A run of
these tests without one declared is skipped rather than passed: an absent
archive is unavailable, not empty.
*/
func recordedArchive(t testing.TB) (context.Context, *tables.Catalog) {
	t.Helper()

	if os.Getenv("SYMM_ARCHIVE") == "" {
		t.Skip("set SYMM_ARCHIVE=1 to read the recorded archive")
	}
	viper.SetConfigType("yml")
	viper.SetConfigFile("../cmd/cfg/config.yml")
	So(viper.ReadInConfig(), ShouldBeNil)

	ctx := context.Background()
	catalog := tables.Open(ctx)
	So(catalog, ShouldNotBeNil)

	return ctx, catalog
}

func TestNewTraining(t *testing.T) {
	Convey("Training retrieves the moves the inspection view draws", t, func() {
		ctx, catalog := recordedArchive(t)
		runs, err := catalog.Runs(ctx)
		So(err, ShouldBeNil)
		So(runs, ShouldNotBeEmpty)

		chosen := runs[0].ID

		if named := os.Getenv("SYMM_RUN"); named != "" {
			chosen = named
		}
		training := NewTraining(
			catalog, hindsight.RunID(chosen), hindsight.DefaultDiscoveryPolicy(), 4,
		)
		So(training, ShouldNotBeNil)
	})
}
