package system

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewStorage(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewStorage is called", func() {
			storage := NewStorage()

			Convey("S3 defaults land from viper", func() {
				So(storage.S3.Bucket, ShouldEqual, "symm")
				So(storage.S3.Region, ShouldEqual, "us-east-1")
				So(storage.S3.Endpoint, ShouldEqual, "http://s3.seaweed.home.arpa")
				So(storage.S3.Anonymous, ShouldBeTrue)
				So(storage.S3.MaxIdleConns, ShouldEqual, 128)
				So(storage.S3.IdleConnTimeout, ShouldEqual, 90*time.Second)
				So(storage.S3.RequestTimeout, ShouldEqual, 10*time.Second)
			})
		})
	})
}
