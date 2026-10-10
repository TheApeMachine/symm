package audit

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestCovarianceScoreContract(t *testing.T) {
	Convey("Covariance scores are unbounded; only their magnitudes must be non-negative", t, func() {
		So(contractViolation(data.UnitCovarianceScore, -3.4, "covariance_score"), ShouldEqual, "")
		So(contractViolation(data.UnitCovarianceScore, 3.4, "absolute_covariance_score"), ShouldEqual, "")
		So(contractViolation(data.UnitCovarianceScore, 2.5, "cohort_absolute_covariance_score"), ShouldEqual, "")
		So(contractViolation(data.UnitCovarianceScore, -0.1, "absolute_covariance_score"), ShouldEqual, "negative_value_for_non_negative_unit")
		So(declaredDomain(data.UnitCovarianceScore, "absolute_covariance_score"), ShouldEqual, "[0, +inf)")
		So(declaredDomain(data.UnitCovarianceScore, "covariance_score"), ShouldEqual, "finite")
	})
}
