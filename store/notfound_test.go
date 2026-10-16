package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	. "github.com/smartystreets/goconvey/convey"
)

func TestIsNotFound(t *testing.T) {
	Convey("IsNotFound separates a missing key from every other failure", t, func() {
		So(IsNotFound(fmt.Errorf("download: %w", &types.NoSuchKey{})), ShouldBeTrue)
		So(IsNotFound(errors.New("connection reset")), ShouldBeFalse)
		So(IsNotFound(nil), ShouldBeFalse)
	})
}
