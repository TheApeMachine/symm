package perspectives

import (
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDocumentActionApply(t *testing.T) {
	Convey("Given a generated playbook document", t, func() {
		random := rand.New(rand.NewSource(11))
		profile := testSearchProfile()
		document := GenerateDocument(profile, random)
		branch := randomRoute(branchSectionEntry, profile, random)
		action := documentAction{
			kind:          documentActionAddBranch,
			playbookIndex: 0,
			section:       branchSectionEntry,
			branch:        branch,
		}

		next := action.Apply(document, profile, random)
		_, err := BuildStrategies(next)

		Convey("It should keep the candidate buildable", func() {
			So(err, ShouldBeNil)
			So(len(next.Playbooks[0].Entry), ShouldBeGreaterThan, len(document.Playbooks[0].Entry))
		})
	})
}
