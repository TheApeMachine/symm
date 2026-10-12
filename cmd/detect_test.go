package cmd

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func makeTestDetection(class string, timestamp time.Time) *data.Measurement {
	measurement := data.NewMeasurement(1, "BTC/USD", "detector", 1, 1)
	measurement.ID = 1
	measurement.At = timestamp
	measurement.From = timestamp.Add(-5 * time.Second)
	measurement.SetMeta("type", class)
	measurement.SetMeta("start_tick", "1")
	measurement.SetMeta("b_tick", "2")
	measurement.SetMeta("c_tick", "3")
	measurement.SetMeta("b_price", "100.00")
	measurement.SetMeta("c_price", "102.00")
	return measurement
}

func TestBalanceDetections(t *testing.T) {
	Convey("Given detected excursions across all classes", t, func() {
		baseTime := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)

		Convey("When up count is positive, flat, chop, and up_friction are capped to up count", func() {
			var detected []*data.Measurement

			for index := 0; index < 10; index++ {
				detected = append(detected, makeTestDetection("up", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := 0; index < 8; index++ {
				detected = append(detected, makeTestDetection("down", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := 0; index < 25; index++ {
				detected = append(detected, makeTestDetection("flat", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := 0; index < 30; index++ {
				detected = append(detected, makeTestDetection("chop", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := 0; index < 15; index++ {
				detected = append(detected, makeTestDetection("up_friction", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			balanced, capped := balanceDetections(detected)

			counts := make(map[string]int)

			for _, measurement := range balanced {
				counts[measurement.Meta("type")]++
			}

			So(counts["up"], ShouldEqual, 10)
			So(counts["down"], ShouldEqual, 8)
			So(counts["flat"], ShouldEqual, 10)
			So(counts["chop"], ShouldEqual, 10)
			So(counts["up_friction"], ShouldEqual, 10)
			So(len(balanced), ShouldEqual, 48)

			So(capped["flat"], ShouldEqual, 15)
			So(capped["chop"], ShouldEqual, 20)
			So(capped["up_friction"], ShouldEqual, 5)
		})

		Convey("Subsampling selects elements uniformly across the whole tape", func() {
			var detected []*data.Measurement

			for index := 0; index < 5; index++ {
				detected = append(detected, makeTestDetection("up", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := 0; index < 50; index++ {
				measurement := makeTestDetection("flat", baseTime.Add(time.Duration(index)*time.Minute))
				measurement.SetMeta("original_index", fmt.Sprintf("%d", index))
				detected = append(detected, measurement)
			}

			balanced, _ := balanceDetections(detected)

			var selectedIndices []int

			for _, measurement := range balanced {
				if measurement.Meta("type") == "flat" {
					var originalIndex int
					_, err := fmt.Sscanf(measurement.Meta("original_index"), "%d", &originalIndex)
					So(err, ShouldBeNil)
					selectedIndices = append(selectedIndices, originalIndex)
				}
			}

			So(len(selectedIndices), ShouldEqual, 5)
			So(selectedIndices[0], ShouldEqual, 0)
			So(selectedIndices[1], ShouldEqual, 10)
			So(selectedIndices[2], ShouldEqual, 20)
			So(selectedIndices[3], ShouldEqual, 30)
			So(selectedIndices[4], ShouldEqual, 40)
		})

		Convey("When counts are already below or equal to up count, all elements are retained", func() {
			var detected []*data.Measurement

			for index := 0; index < 10; index++ {
				detected = append(detected, makeTestDetection("up", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := 0; index < 5; index++ {
				detected = append(detected, makeTestDetection("flat", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := 0; index < 10; index++ {
				detected = append(detected, makeTestDetection("chop", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := 0; index < 2; index++ {
				detected = append(detected, makeTestDetection("up_friction", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			balanced, capped := balanceDetections(detected)

			counts := make(map[string]int)

			for _, measurement := range balanced {
				counts[measurement.Meta("type")]++
			}

			So(counts["up"], ShouldEqual, 10)
			So(counts["flat"], ShouldEqual, 5)
			So(counts["chop"], ShouldEqual, 10)
			So(counts["up_friction"], ShouldEqual, 2)
			So(len(capped), ShouldEqual, 0)
		})

		Convey("When up count is 0, flat, chop, and up_friction are removed", func() {
			var detected []*data.Measurement

			for index := 0; index < 5; index++ {
				detected = append(detected, makeTestDetection("down", baseTime.Add(time.Duration(index)*time.Minute)))
				detected = append(detected, makeTestDetection("flat", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			balanced, capped := balanceDetections(detected)

			So(len(balanced), ShouldEqual, 5)
			So(balanced[0].Meta("type"), ShouldEqual, "down")
			So(capped["flat"], ShouldEqual, 5)
		})
	})
}
