package store

import (
	"context"
	. "github.com/smartystreets/goconvey/convey"
	"math"
	"strconv"
	"testing"
)

func TestResonanceWrite(t *testing.T) {
	Convey("The native retained owner produces complete layers and only causal resolutions", t, func() {
		ctx := context.Background()
		server := NewResonance()
		client := Resonance_ServerToClient(server)
		defer client.Release()
		for sequence := int64(1); sequence <= 128; sequence++ {
			features := []float64{math.Sin(float64(sequence) / 3), math.Cos(float64(sequence) / 5), math.Sin(float64(sequence) / 7)}
			So(client.Write(ctx, func(args Resonance_write_Params) error {
				if err := writeFloats(features, args.NewFeatures); err != nil {
					return err
				}
				labels, err := args.NewFeatureIdentities(3)
				if err != nil {
					return err
				}
				for index := range 3 {
					if err := labels.Set(index, "fixture:"+strconv.Itoa(index)); err != nil {
						return err
					}
				}
				args.SetReference(100 + math.Sin(float64(sequence)/3))
				args.SetEpoch(1)
				args.SetSequence(sequence)
				args.SetTimestamp(float64(sequence))
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			reading, err := result.Reading()
			So(err, ShouldBeNil)
			layers, err := reading.Layers()
			So(err, ShouldBeNil)
			So(layers.Len(), ShouldEqual, 3)
			So(reading.Energy(), ShouldBeGreaterThanOrEqualTo, 0)
			So(reading.ReconstructionError(), ShouldBeGreaterThanOrEqualTo, 0)
			sensory, err := layers.At(0).State()
			So(err, ShouldBeNil)
			for index, value := range features {
				So(sensory.At(index), ShouldEqual, value)
			}
			So(result.ResolvedSteps(), ShouldBeLessThanOrEqualTo, sequence*int64(server.manifold.taskRows))
			if sequence == 1 {
				So(result.Calibrated(), ShouldBeFalse)
				So(server.manifold.taskLearners[0].observations, ShouldEqual, 0)
			}
			if result.HasLastResolution() {
				resolution, err := result.LastResolution()
				So(err, ShouldBeNil)
				So(resolution.Step(), ShouldEqual, sequence)
			}
			release()
		}
		So(server.manifold.taskLearners[0].observations, ShouldBeGreaterThan, 0)
		So(server.resolved, ShouldBeGreaterThan, 0)
		So(server.manifold.taskRows, ShouldBeGreaterThan, 1)
		Convey("Regressing the causal stamp is rejected", func() {
			So(client.Write(ctx, func(args Resonance_write_Params) error {
				if err := writeFloats([]float64{1, 2, 3}, args.NewFeatures); err != nil {
					return err
				}
				labels, err := args.NewFeatureIdentities(3)
				if err != nil {
					return err
				}
				for index := range 3 {
					if err := labels.Set(index, "fixture:"+strconv.Itoa(index)); err != nil {
						return err
					}
				}
				args.SetReference(100)
				args.SetEpoch(1)
				args.SetSequence(1)
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}

func BenchmarkResonanceWrite(b *testing.B) {
	ctx := context.Background()
	client := Resonance_ServerToClient(NewResonance())
	defer client.Release()
	b.ReportAllocs()
	b.ResetTimer()
	for observation := range b.N {
		err := client.Write(ctx, func(args Resonance_write_Params) error {
			features, err := args.NewFeatures(55)
			if err != nil {
				return err
			}
			for coordinate := range 55 {
				features.Set(coordinate, math.Sin(float64(observation+coordinate)/float64(coordinate+1)))
			}
			labels, err := args.NewFeatureIdentities(55)
			if err != nil {
				return err
			}
			for index := range 55 {
				if err := labels.Set(index, "fixture:"+strconv.Itoa(index)); err != nil {
					return err
				}
			}
			args.SetReference(100 + math.Sin(float64(observation)/3))
			args.SetEpoch(1)
			args.SetSequence(int64(observation))
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		_, err = future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}
