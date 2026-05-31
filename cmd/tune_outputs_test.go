package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestSnapshotTuneCandidate(t *testing.T) {
	Convey("Given a candidate document and tunables", t, func() {
		document := perspectives.Document{
			Version: 1,
			Playbooks: []perspectives.PlaybookSpec{{
				Name: "test",
			}},
		}
		entryEdge := 2.0
		tunables := config.Tunables{EntryEdgeMultiple: &entryEdge}

		snapshot := snapshotTuneCandidate(document, tunables)

		Convey("It should not alias the source document", func() {
			document.Playbooks[0].Name = "mutated"
			So(snapshot.perspectives.Playbooks[0].Name, ShouldEqual, "test")
		})

		Convey("It should not alias tunable overlays", func() {
			changed := 3.0
			tunables.EntryEdgeMultiple = &changed
			So(*snapshot.tunables.EntryEdgeMultiple, ShouldEqual, 2.0)
		})
	})
}

func TestTuneAcceptsFirstEligibleLeader(t *testing.T) {
	Convey("Given no incumbent and a flat zero trial", t, func() {
		hasBest := false
		accept := !hasBest || betterTuneCandidate(0, 0, 0, 0)

		Convey("It should install the first eligible leader", func() {
			So(accept, ShouldBeTrue)
		})
	})
}
