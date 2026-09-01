package main

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReadCatalog(t *testing.T) {
	Convey("Given a metric CSV row", t, func() {
		source := strings.Join([]string{
			"source,metric,identity,metric_class,semantic_role,purpose,quality_definedness,current_named_use,normative_destinations,forbidden_use,review_note,normative_status,baseline_commit",
			"flow,rate,flow/rate,rate,FLOW_STATE,purpose,defined,use,destination,forbidden,,KEEP_NAMED_USE,commit",
		}, "\n")

		catalog, err := readCatalog(strings.NewReader(source))

		Convey("it preserves every semantic field under the declared identity", func() {
			So(err, ShouldBeNil)
			So(catalog.BaselineCommit, ShouldEqual, "commit")
			So(catalog.Metrics, ShouldHaveLength, 1)
			So(catalog.Metrics[0].Identity, ShouldEqual, "flow/rate")
			So(catalog.Metrics[0].SemanticRole, ShouldEqual, "FLOW_STATE")
		})
	})
}
