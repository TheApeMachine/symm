package matrix_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestTransposeNext(t *testing.T) {
	Convey("Transpose changes addressing without mutating the source", t, func() {
		node := matrix.NewTranspose[string]()
		original := [][]string{{"a", "b", "c"}, {"d", "e", "f"}}
		want := [][]string{{"a", "d"}, {"b", "e"}, {"c", "f"}}
		out := tests.CollectSeq(node.Next(transport.Values(original)))
		So(node.Error(), ShouldBeNil)
		So(out[0], ShouldResemble, want)
		So(node.Read(), ShouldResemble, want)
		out[0][0][0] = "changed"
		So(original[0][0], ShouldEqual, "a")
	})

	Convey("Each arrival is transposed independently", t, func() {
		node := matrix.NewTranspose[string]()
		original := [][]string{{"a", "b", "c"}, {"d", "e", "f"}}
		out := tests.CollectSeq(node.Next(transport.Values(original, [][]string{{"last"}})))
		So(out[0], ShouldResemble, [][]string{{"a", "d"}, {"b", "e"}, {"c", "f"}})
		So(out[1], ShouldResemble, [][]string{{"last"}})
	})

	Convey("Ragged rows are a shape error", t, func() {
		bad := matrix.NewTranspose[float64]()
		tests.CollectSeq(bad.Next(transport.Values([][]float64{{1, 2}, {3}})))
		So(errors.Is(bad.Error(), core.ErrShape), ShouldBeTrue)
	})
}
