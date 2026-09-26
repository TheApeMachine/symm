package store

import (
	"context"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"math"
	"strconv"
	"testing"
)

func TestResonanceSnapshot(t *testing.T) {
	Convey("Checkpoint restores matrices, measured rank and outstanding causal rows exactly", t, func() {
		ctx := context.Background()
		original := Resonance_ServerToClient(NewResonance())
		defer original.Release()
		restored := Resonance_ServerToClient(NewResonance())
		defer restored.Release()
		for sequence := int64(1); sequence <= 80; sequence++ {
			So(writeResonanceFixture(ctx, original, sequence), ShouldBeNil)
		}
		future, release := original.Snapshot(ctx, nil)
		result, err := future.Struct()
		So(err, ShouldBeNil)
		encoded, err := result.Data()
		So(err, ShouldBeNil)
		state := append([]byte(nil), encoded...)
		release()
		restore, releaseRestore := restored.Restore(ctx, func(args runtime.Snapshot_restore_Params) error { return args.SetData(state) })
		_, err = restore.Struct()
		So(err, ShouldBeNil)
		releaseRestore()
		Convey("A new source epoch preserves learned evidence and expires unresolved outcomes", func() {
			restartedServer := NewResonance()
			restarted := Resonance_ServerToClient(restartedServer)
			defer restarted.Release()
			restore, releaseRestore := restarted.Restore(ctx, func(args runtime.Snapshot_restore_Params) error { return args.SetData(state) })
			_, err := restore.Struct()
			So(err, ShouldBeNil)
			releaseRestore()
			model := restartedServer.manifold
			coefficients := append([]float64(nil), model.taskLearners[0].beta...)
			temporal := append([]float64(nil), model.temporalOperators[0].RawMatrix().Data...)
			resolved, support := restartedServer.resolved, model.taskLearners[0].observations
			noiseCount, noiseMean, noiseM2 := restartedServer.returnCount, restartedServer.returnMean, restartedServer.returnM2
			memoryCount, memoryCovariance := restartedServer.memory.count, restartedServer.memory.covariance
			So(len(restartedServer.pending), ShouldBeGreaterThan, 0)
			So(restarted.Write(ctx, func(args Resonance_write_Params) error {
				if err := writeFloats([]float64{0.3, 0.7, -0.2}, args.NewFeatures); err != nil {
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
				args.SetReference(500) // Restart price gap must not train a fictitious return.
				args.SetEpoch(2)
				args.SetSequence(0)
				return nil
			}), ShouldBeNil)
			So(restarted.WaitStreaming(), ShouldBeNil)
			future, release := restarted.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			reading, err := result.Reading()
			So(err, ShouldBeNil)
			So(reading.HasTemporalError(), ShouldBeFalse)
			So(result.HasLastResolution(), ShouldBeFalse)
			release()
			So(restartedServer.manifold, ShouldEqual, model)
			So(restartedServer.observations, ShouldEqual, 81)
			So(restartedServer.resolved, ShouldEqual, resolved)
			So(model.taskLearners[0].observations, ShouldEqual, support)
			So(model.taskLearners[0].beta, ShouldResemble, coefficients)
			So(model.temporalOperators[0].RawMatrix().Data, ShouldResemble, temporal)
			So(restartedServer.returnCount, ShouldEqual, noiseCount)
			So(restartedServer.returnMean, ShouldEqual, noiseMean)
			So(restartedServer.returnM2, ShouldEqual, noiseM2)
			So(restartedServer.memory.count, ShouldEqual, memoryCount)
			So(restartedServer.memory.covariance, ShouldEqual, memoryCovariance)
			So(len(restartedServer.pending), ShouldEqual, 1)
			So(restartedServer.pending[0].issued, ShouldEqual, 81)
		})
		for sequence := int64(81); sequence <= 160; sequence++ {
			So(writeResonanceFixture(ctx, original, sequence), ShouldBeNil)
			So(writeResonanceFixture(ctx, restored, sequence), ShouldBeNil)
		}
		left, releaseLeft := original.Snapshot(ctx, nil)
		defer releaseLeft()
		leftResult, err := left.Struct()
		So(err, ShouldBeNil)
		leftData, err := leftResult.Data()
		So(err, ShouldBeNil)
		right, releaseRight := restored.Snapshot(ctx, nil)
		defer releaseRight()
		rightResult, err := right.Struct()
		So(err, ShouldBeNil)
		rightData, err := rightResult.Data()
		So(err, ShouldBeNil)
		So(rightData, ShouldResemble, leftData)
	})
}

func TestResonanceRestore(t *testing.T) {
	Convey("Malformed checkpoint cannot replace an existing retained model", t, func() {
		ctx := context.Background()
		server := NewResonance()
		client := Resonance_ServerToClient(server)
		defer client.Release()
		So(writeResonanceFixture(ctx, client, 1), ShouldBeNil)
		restore, release := client.Restore(ctx, func(args runtime.Snapshot_restore_Params) error { return args.SetData([]byte("invalid")) })
		defer release()
		_, err := restore.Struct()
		So(err, ShouldNotBeNil)
		So(server.observations, ShouldEqual, 1)
	})
}

func writeResonanceFixture(ctx context.Context, client Resonance, sequence int64) error {
	if err := client.Write(ctx, func(args Resonance_write_Params) error {
		features := []float64{math.Sin(float64(sequence) / 3), math.Cos(float64(sequence) / 5), math.Sin(float64(sequence) / 7)}
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
		return nil
	}); err != nil {
		return err
	}
	if err := client.WaitStreaming(); err != nil {
		return err
	}
	future, release := client.Done(ctx, nil)
	defer release()
	_, err := future.Struct()
	return err
}
