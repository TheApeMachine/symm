package hawkes

import (
	"io"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
)

func excitationBurstSamples(base time.Time, count int) []float64 {
	buyTimes := make([]float64, 0, count/2)
	sellTimes := make([]float64, 0, count/2)

	for index := range count {
		wall := base.Add(time.Duration(index) * 100 * time.Millisecond)
		seconds := float64(wall.UnixNano()) / float64(time.Second)

		if index%2 == 0 {
			sellTimes = append(sellTimes, seconds)
			continue
		}

		buyTimes = append(buyTimes, seconds)
	}

	horizon := float64(base.Add(time.Duration(count)*100*time.Millisecond).UnixNano()) / float64(time.Second)
	span := base.Add(time.Duration(count) * 100 * time.Millisecond).Sub(base)
	cooldown := algorithm.DeriveFitCooldown(span).Seconds()

	samples := []float64{
		horizon,
		cooldown,
		float64(len(buyTimes)),
		float64(len(sellTimes)),
		0,
	}
	samples = append(samples, buyTimes...)
	samples = append(samples, sellTimes...)

	return samples
}

func readExcitationOutbound(stage io.Reader) (*datura.Artifact, error) {
	chunk := make([]byte, 262144)
	frame := make([]byte, 0, len(chunk))

	for {
		readCount, err := stage.Read(chunk)

		if readCount > 0 {
			frame = append(frame, chunk[:readCount]...)
		}

		if err == io.EOF {
			break
		}

		if err != nil && err != io.ErrShortBuffer {
			return nil, errnie.Error(errnie.Err(errnie.IO, "hawkes test: stage read failed", err))
		}

		if readCount == 0 {
			break
		}
	}

	if len(frame) == 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "hawkes test: stage produced no output", nil))
	}

	outbound := datura.Acquire("hawkes-out", datura.APPJSON)
	_, err := outbound.Write(frame)

	if err != nil {
		outbound.Release()

		return nil, errnie.Error(errnie.Err(errnie.IO, "hawkes test: outbound write failed", err))
	}

	if !outbound.HasEncryptedPayload() {
		outbound.Release()

		return nil, errnie.Error(errnie.Err(errnie.Validation, "hawkes test: stage produced no output", nil))
	}

	return outbound, nil
}

func flopExcitationArtifact(inbound *datura.Artifact, stage io.ReadWriter) error {
	if _, err := stage.Write(inbound.Pack()); err != nil {
		return err
	}

	outbound, err := readExcitationOutbound(stage)

	if err != nil {
		return err
	}

	defer outbound.Release()

	_, err = inbound.Write(outbound.Pack())

	return err
}

func warmExcitationScope(
	excitation *algorithm.Excitation,
	scope string,
	rows ...[]float64,
) float64 {
	strength := 0.0

	for _, row := range rows {
		processed := datura.Acquire("hawkes", datura.APPJSON)
		processed.WithScope(scope)
		processed.WithPayload(equation.MarshalFeatureSchema(algorithm.ExcitationSampleInputKeys, row))

		if flopExcitationArtifact(processed, excitation) == nil {
			strength = datura.Peek[float64](processed, "output", "strength")
		}

		processed.Release()
	}

	return strength
}

func frenzyExcitationPayload() []float64 {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	return excitationBurstSamples(base, 8)
}

func organicExcitationPayload() []float64 {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	return excitationBurstSamples(base, 128)
}

func TestExcitationPayloadWarmScope(testingTB *testing.T) {
	Convey("Given warmed excitation payloads", testingTB, func() {
		excitation := algorithm.NewExcitation(
			datura.Acquire("excitation-config", datura.APPJSON),
		)
		strength := warmExcitationScope(
			excitation,
			"BTC/USD",
			frenzyExcitationPayload(),
			organicExcitationPayload(),
		)

		Convey("It should publish thermal strength", func() {
			So(strength, ShouldBeGreaterThan, 0)
		})
	})
}
