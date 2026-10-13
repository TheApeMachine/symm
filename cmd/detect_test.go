package cmd

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func makeTestDetection(class string, timestamp time.Time, extra ...*data.StringEntry) *data.Measurement {
	meta := []*data.StringEntry{
		{Key: "type", Value: class},
		{Key: "start_tick", Value: "1"},
		{Key: "b_tick", Value: "2"},
		{Key: "c_tick", Value: "3"},
		{Key: "b_price", Value: "100.00"},
		{Key: "c_price", Value: "102.00"},
	}
	meta = append(meta, extra...)
	measurement := data.NewMeasurement(1, "BTC/USD", "detector", timestamp, timestamp.Add(-5*time.Second), meta...)
	measurement.ID = 1
	return measurement
}

func TestBalanceDetections(t *testing.T) {
	Convey("Given detected excursions across all classes", t, func() {
		baseTime := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)

		Convey("When up count is positive, flat, chop, and up_friction are capped to up count", func() {
			var detected []*data.Measurement

			for index := range 10 {
				detected = append(detected, makeTestDetection("up", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := range 8 {
				detected = append(detected, makeTestDetection("down", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := range 25 {
				detected = append(detected, makeTestDetection("flat", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := range 30 {
				detected = append(detected, makeTestDetection("chop", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := range 15 {
				detected = append(detected, makeTestDetection("up_friction", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			balanced, capped := balanceDetections(detected)

			counts := make(map[string]int)

			for _, measurement := range balanced {
				counts[detectionType(measurement)]++
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

			for index := range 5 {
				detected = append(detected, makeTestDetection("up", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := range 50 {
				measurement := makeTestDetection("flat", baseTime.Add(time.Duration(index)*time.Minute), &data.StringEntry{
					Key:   "original_index",
					Value: fmt.Sprintf("%d", index),
				})
				detected = append(detected, measurement)
			}

			balanced, _ := balanceDetections(detected)

			var selectedIndices []int

			for _, measurement := range balanced {
				if detectionType(measurement) == "flat" {
					var originalIndex int
					if origEntry := data.Pull(measurement.Meta("original_index")); origEntry != nil {
						_, err := fmt.Sscanf(origEntry.Value, "%d", &originalIndex)
						So(err, ShouldBeNil)
					}
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

			for index := range 10 {
				detected = append(detected, makeTestDetection("up", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := range 5 {
				detected = append(detected, makeTestDetection("flat", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := range 10 {
				detected = append(detected, makeTestDetection("chop", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			for index := range 2 {
				detected = append(detected, makeTestDetection("up_friction", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			balanced, capped := balanceDetections(detected)

			counts := make(map[string]int)

			for _, measurement := range balanced {
				counts[detectionType(measurement)]++
			}

			So(counts["up"], ShouldEqual, 10)
			So(counts["flat"], ShouldEqual, 5)
			So(counts["chop"], ShouldEqual, 10)
			So(counts["up_friction"], ShouldEqual, 2)
			So(len(capped), ShouldEqual, 0)
		})

		Convey("When up count is 0, flat, chop, and up_friction are removed", func() {
			var detected []*data.Measurement

			for index := range 5 {
				detected = append(detected, makeTestDetection("down", baseTime.Add(time.Duration(index)*time.Minute)))
				detected = append(detected, makeTestDetection("flat", baseTime.Add(time.Duration(index)*time.Minute)))
			}

			balanced, capped := balanceDetections(detected)

			So(len(balanced), ShouldEqual, 5)
			So(detectionType(balanced[0]), ShouldEqual, "down")
			So(capped["flat"], ShouldEqual, 5)
		})
	})
}
