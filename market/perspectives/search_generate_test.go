package perspectives

import (
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGenerateDocument(t *testing.T) {
	Convey("Given a replay-derived primitive profile", t, func() {
		random := rand.New(rand.NewSource(12))
		var document Document
		foundFreeEntryRoot := false

		for range 32 {
			document = GenerateDocument(testSearchProfile(), random)
			_, err := BuildStrategies(document)
			So(err, ShouldBeNil)

			if documentHasFreeEntryRoot(document) {
				foundFreeEntryRoot = true
			}
		}

		Convey("It should produce a valid free-form YAML candidate", func() {
			So(document.Playbooks, ShouldNotBeEmpty)
			So(len(document.Playbooks), ShouldBeLessThanOrEqualTo, maxGeneratedPlaybooks)
			So(foundFreeEntryRoot, ShouldBeTrue)
		})
	})
}

func testSearchProfile() SearchProfile {
	return SearchProfile{Categories: []CategoryStat{
		{
			Name:    CategoryAggressiveDrive.String(),
			Source:  SourceCVD.String(),
			Count:   10,
			MeanSNR: 1.4,
			MaxSNR:  2.0,
			P50SNR:  1.2,
			P75SNR:  1.5,
			P90SNR:  1.8,
		},
		{
			Name:    CategoryVerticalIgnition.String(),
			Source:  SourcePumpDump.String(),
			Count:   8,
			MeanSNR: 1.6,
			MaxSNR:  2.2,
			P50SNR:  1.1,
			P75SNR:  1.6,
			P90SNR:  2.0,
		},
		{
			Name:    CategoryActiveReversal.String(),
			Source:  SourceExhaustion.String(),
			Count:   6,
			MeanSNR: 1.3,
			MaxSNR:  1.9,
			P50SNR:  1.0,
			P75SNR:  1.4,
			P90SNR:  1.8,
		},
	}}
}
