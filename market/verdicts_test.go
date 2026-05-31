package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestEntryVerdictsIncludesDenies(t *testing.T) {
	convey.Convey("Given measurements that trigger a deny gate", t, func() {
		measurements := []perspectives.Measurement{
			{Category: perspectives.CategorySystemicSlump, SNR: 2.0},
		}

		verdicts := EntryVerdicts(measurements, nil)
		defer ReleaseEntryVerdicts(verdicts)

		convey.Convey("It should return blocked playbook verdicts with traces", func() {
			found := false

			for _, verdict := range verdicts {
				if verdict.Action != perspectives.ActionWait &&
					verdict.Action != perspectives.ActionDeny {
					continue
				}

				found = true
				convey.So(verdict.Trace, convey.ShouldNotBeNil)
			}

			convey.So(found, convey.ShouldBeTrue)
		})
	})
}
