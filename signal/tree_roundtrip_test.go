package signal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

func TestInsertTreeArtifactRoundTrip(testingTB *testing.T) {
	Convey("Given a feature artifact stored in the tree", testingTB, func() {
		tree := dmt.NewTree("")
		payload := []byte(`[1,2,3,4,5,6,7,8,9,10,11,12]`)
		artifact := datura.Acquire("fluid-features", datura.Artifact_Type_json)
		artifact.WithRole("features")
		artifact.WithScope("BTC/EUR")
		artifact.WithPayload(payload)

		InsertTreeArtifact(tree, artifact)
		artifact.Release()

		Convey("It should round-trip encrypted payload through Seek", func() {
			var payloadOK bool

			for inbound := range tree.Seek([]byte("features/BTC/EUR")) {
				encryptedKey, _ := inbound.EncryptedKey()
				So(len(encryptedKey), ShouldBeGreaterThanOrEqualTo, 32)

				roundTrip, ok := inbound.PayloadQuiet()
				payloadOK = ok
				So(roundTrip, ShouldResemble, payload)
				inbound.Release()
			}

			So(payloadOK, ShouldBeTrue)
		})
	})
}
