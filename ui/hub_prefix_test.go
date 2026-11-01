package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/data"
)

func TestChunkThresholds(t *testing.T) {
	Convey("chunkThresholds splits oversized path listings", t, func() {
		all := map[string]float64{
			"model/paths/a/enter.bin": 0.5,
			"model/paths/b/enter.bin": 0.5,
			"model/paths/c/enter.bin": 0.5,
		}

		chunks := chunkThresholds(all, 120)

		So(len(chunks), ShouldBeGreaterThan, 1)

		seen := 0
		for _, chunk := range chunks {
			seen += len(chunk)
		}
		So(seen, ShouldEqual, 3)
	})
}

func TestExploreBookThreshold(t *testing.T) {
	Convey("exploreBookThreshold seeds 0.5 and sums resistance", t, func() {
		So(exploreBookThreshold(nil), ShouldEqual, 0.5)

		value := data.NewValue("model", "enter", "paths").
			Append(0.05, "resistance").
			Append(-0.02, "resistance")

		So(exploreBookThreshold(value), ShouldAlmostEqual, 0.53, 1e-9)
	})
}

func TestPolicyPathKey(t *testing.T) {
	Convey("policyPathKey keeps .bin keys and appends action scope", t, func() {
		So(
			policyPathKey("model/paths/R01/enter.bin", "enter"),
			ShouldEqual,
			"model/paths/R01/enter.bin",
		)
		So(
			policyPathKey("model/paths/R01/R02", "exit"),
			ShouldEqual,
			"model/paths/R01/R02/exit.bin",
		)
	})
}
