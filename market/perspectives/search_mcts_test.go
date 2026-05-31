package perspectives

import (
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDocumentSearchNext(t *testing.T) {
	Convey("Given a Monte Carlo document search", t, func() {
		search, err := NewDocumentSearch(testSearchProfile(), rand.New(rand.NewSource(13)))
		So(err, ShouldBeNil)

		document, pendingID := search.Next()
		_, err = BuildStrategies(document)
		So(err, ShouldBeNil)

		search.Observe(document, 4.2, pendingID)
		next, _ := search.Next()
		_, err = BuildStrategies(next)

		Convey("It should backpropagate rewards and keep searching valid trees", func() {
			So(err, ShouldBeNil)
			So(search.BestReward(), ShouldEqual, 4.2)
		})
	})
}

func TestDocumentSearchObserve(t *testing.T) {
	Convey("Given an untracked baseline document", t, func() {
		random := rand.New(rand.NewSource(17))
		profile := testSearchProfile()
		search, err := NewDocumentSearch(profile, random)
		So(err, ShouldBeNil)
		baseline := GenerateDocument(profile, random)

		search.Observe(baseline, 0, 0)

		Convey("It should seed the root before random generation", func() {
			So(search.root, ShouldNotBeNil)
			So(documentSearchKey(search.root.document), ShouldEqual, documentSearchKey(baseline))
		})
	})
}
