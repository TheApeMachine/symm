package types

import (
	flatbuffers "github.com/google/flatbuffers/go"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func EncodeManifold(manifold *ManifoldState) ([]byte, error) {
	if manifold == nil || manifold.State == nil {
		return nil, nil
	}

	builder := measurementsBuilderPool.Get().(*flatbuffers.Builder)
	builder.Reset()
	defer measurementsBuilderPool.Put(builder)

	state := manifold.State

	// Create vectors
	var bytesOffset, seqsOffset, tokenIdsOffset, contentIdsOffset flatbuffers.UOffsetT
	var phaseOffset, omegaOffset, energyOffset, massOffset, heatOffset, ampOffset flatbuffers.UOffsetT
	var posOffset, velOffset flatbuffers.UOffsetT
	var clampedOffset, darkOffset flatbuffers.UOffsetT

	// Float32 vectors
	if len(state.Phase) > 0 {
		wire.ManifoldFrameStartPhaseVector(builder, len(state.Phase))
		for i := len(state.Phase) - 1; i >= 0; i-- {
			builder.PrependFloat32(state.Phase[i])
		}
		phaseOffset = builder.EndVector(len(state.Phase))
	}

	if len(state.Omega) > 0 {
		wire.ManifoldFrameStartOmegaVector(builder, len(state.Omega))
		for i := len(state.Omega) - 1; i >= 0; i-- {
			builder.PrependFloat32(state.Omega[i])
		}
		omegaOffset = builder.EndVector(len(state.Omega))
	}

	if len(state.Energy) > 0 {
		wire.ManifoldFrameStartEnergyVector(builder, len(state.Energy))
		for i := len(state.Energy) - 1; i >= 0; i-- {
			builder.PrependFloat32(state.Energy[i])
		}
		energyOffset = builder.EndVector(len(state.Energy))
	}

	if len(state.Mass) > 0 {
		wire.ManifoldFrameStartMassVector(builder, len(state.Mass))
		for i := len(state.Mass) - 1; i >= 0; i-- {
			builder.PrependFloat32(state.Mass[i])
		}
		massOffset = builder.EndVector(len(state.Mass))
	}

	if len(state.Heat) > 0 {
		wire.ManifoldFrameStartHeatVector(builder, len(state.Heat))
		for i := len(state.Heat) - 1; i >= 0; i-- {
			builder.PrependFloat32(state.Heat[i])
		}
		heatOffset = builder.EndVector(len(state.Heat))
	}

	if len(state.Amp) > 0 {
		wire.ManifoldFrameStartAmpVector(builder, len(state.Amp))
		for i := len(state.Amp) - 1; i >= 0; i-- {
			builder.PrependFloat32(state.Amp[i])
		}
		ampOffset = builder.EndVector(len(state.Amp))
	}

	if len(state.Pos) > 0 {
		wire.ManifoldFrameStartPosVector(builder, len(state.Pos))
		for i := len(state.Pos) - 1; i >= 0; i-- {
			builder.PrependFloat32(state.Pos[i])
		}
		posOffset = builder.EndVector(len(state.Pos))
	}

	if len(state.Vel) > 0 {
		wire.ManifoldFrameStartVelVector(builder, len(state.Vel))
		for i := len(state.Vel) - 1; i >= 0; i-- {
			builder.PrependFloat32(state.Vel[i])
		}
		velOffset = builder.EndVector(len(state.Vel))
	}

	// Int64 vectors
	if len(state.Bytes) > 0 {
		wire.ManifoldFrameStartBytesVector(builder, len(state.Bytes))
		for i := len(state.Bytes) - 1; i >= 0; i-- {
			builder.PrependInt64(state.Bytes[i])
		}
		bytesOffset = builder.EndVector(len(state.Bytes))
	}

	if len(state.Seqs) > 0 {
		wire.ManifoldFrameStartSeqsVector(builder, len(state.Seqs))
		for i := len(state.Seqs) - 1; i >= 0; i-- {
			builder.PrependInt64(state.Seqs[i])
		}
		seqsOffset = builder.EndVector(len(state.Seqs))
	}

	if len(state.TokenIDs) > 0 {
		wire.ManifoldFrameStartTokenIdsVector(builder, len(state.TokenIDs))
		for i := len(state.TokenIDs) - 1; i >= 0; i-- {
			builder.PrependInt64(state.TokenIDs[i])
		}
		tokenIdsOffset = builder.EndVector(len(state.TokenIDs))
	}

	if len(state.ContentIDs) > 0 {
		wire.ManifoldFrameStartContentIdsVector(builder, len(state.ContentIDs))
		for i := len(state.ContentIDs) - 1; i >= 0; i-- {
			builder.PrependInt64(state.ContentIDs[i])
		}
		contentIdsOffset = builder.EndVector(len(state.ContentIDs))
	}

	// Bool vectors
	if len(state.Clamped) > 0 {
		wire.ManifoldFrameStartClampedVector(builder, len(state.Clamped))
		for i := len(state.Clamped) - 1; i >= 0; i-- {
			builder.PrependBool(state.Clamped[i])
		}
		clampedOffset = builder.EndVector(len(state.Clamped))
	}

	if len(state.Dark) > 0 {
		wire.ManifoldFrameStartDarkVector(builder, len(state.Dark))
		for i := len(state.Dark) - 1; i >= 0; i-- {
			builder.PrependBool(state.Dark[i])
		}
		darkOffset = builder.EndVector(len(state.Dark))
	}
	
	// Create ManifoldReading
	wire.ManifoldReadingStart(builder)
	wire.ManifoldReadingAddDivergence(builder, manifold.Reading.Divergence)
	wire.ManifoldReadingAddGuidanceSpeed(builder, manifold.Reading.GuidanceSpeed)
	wire.ManifoldReadingAddCoherenceMag2(builder, manifold.Reading.CoherenceMag2)
	wire.ManifoldReadingAddPressureGradNorm(builder, manifold.Reading.PressureGradNorm)
	wire.ManifoldReadingAddViscosityProxy(builder, manifold.Reading.ViscosityProxy)
	wire.ManifoldReadingAddKuramotoR(builder, manifold.Reading.KuramotoR)
	wire.ManifoldReadingAddKuramotoPsi(builder, manifold.Reading.KuramotoPsi)
	readingOffset := wire.ManifoldReadingEnd(builder)

	// Fields
	var momRhoOffset, fieldEnergyOffset, waveRealOffset, waveImagOffset flatbuffers.UOffsetT

	if len(manifold.MomRho) > 0 {
		wire.ManifoldFrameStartMomRhoVector(builder, len(manifold.MomRho))
		for i := len(manifold.MomRho) - 1; i >= 0; i-- {
			builder.PrependFloat32(manifold.MomRho[i])
		}
		momRhoOffset = builder.EndVector(len(manifold.MomRho))
	}

	if len(manifold.FieldEnergy) > 0 {
		wire.ManifoldFrameStartFieldEnergyVector(builder, len(manifold.FieldEnergy))
		for i := len(manifold.FieldEnergy) - 1; i >= 0; i-- {
			builder.PrependFloat32(manifold.FieldEnergy[i])
		}
		fieldEnergyOffset = builder.EndVector(len(manifold.FieldEnergy))
	}

	if len(manifold.WaveReal) > 0 {
		wire.ManifoldFrameStartWaveRealVector(builder, len(manifold.WaveReal))
		for i := len(manifold.WaveReal) - 1; i >= 0; i-- {
			builder.PrependFloat32(manifold.WaveReal[i])
		}
		waveRealOffset = builder.EndVector(len(manifold.WaveReal))
	}

	if len(manifold.WaveImag) > 0 {
		wire.ManifoldFrameStartWaveImagVector(builder, len(manifold.WaveImag))
		for i := len(manifold.WaveImag) - 1; i >= 0; i-- {
			builder.PrependFloat32(manifold.WaveImag[i])
		}
		waveImagOffset = builder.EndVector(len(manifold.WaveImag))
	}
	
	wire.ManifoldFrameStart(builder)
	wire.ManifoldFrameAddAt(builder, manifold.At.UnixNano())
	wire.ManifoldFrameAddVersion(builder, manifold.Version)
	wire.ManifoldFrameAddN(builder, int64(state.N))
	
	wire.ManifoldFrameAddBytes(builder, bytesOffset)
	wire.ManifoldFrameAddSeqs(builder, seqsOffset)
	wire.ManifoldFrameAddTokenIds(builder, tokenIdsOffset)
	wire.ManifoldFrameAddContentIds(builder, contentIdsOffset)
	wire.ManifoldFrameAddPhase(builder, phaseOffset)
	wire.ManifoldFrameAddOmega(builder, omegaOffset)
	wire.ManifoldFrameAddEnergy(builder, energyOffset)
	wire.ManifoldFrameAddMass(builder, massOffset)
	wire.ManifoldFrameAddHeat(builder, heatOffset)
	wire.ManifoldFrameAddAmp(builder, ampOffset)
	wire.ManifoldFrameAddPos(builder, posOffset)
	wire.ManifoldFrameAddVel(builder, velOffset)
	wire.ManifoldFrameAddClamped(builder, clampedOffset)
	wire.ManifoldFrameAddDark(builder, darkOffset)
	wire.ManifoldFrameAddReading(builder, readingOffset)
	
	wire.ManifoldFrameAddGridX(builder, int32(manifold.GridX))
	wire.ManifoldFrameAddGridY(builder, int32(manifold.GridY))
	wire.ManifoldFrameAddGridZ(builder, int32(manifold.GridZ))
	wire.ManifoldFrameAddGridSpacing(builder, manifold.GridSpacing)
	
	wire.ManifoldFrameAddMomRho(builder, momRhoOffset)
	wire.ManifoldFrameAddFieldEnergy(builder, fieldEnergyOffset)
	wire.ManifoldFrameAddWaveReal(builder, waveRealOffset)
	wire.ManifoldFrameAddWaveImag(builder, waveImagOffset)
	
	wire.ManifoldFrameAddDensityScale(builder, manifold.DensityScale)
	wire.ManifoldFrameAddMomentumScale(builder, manifold.MomentumScale)
	wire.ManifoldFrameAddEnergyScale(builder, manifold.EnergyScale)
	wire.ManifoldFrameAddWaveScale(builder, manifold.WaveScale)

	manifoldOffset := wire.ManifoldFrameEnd(builder)
	
	wire.MessageStart(builder)
	wire.MessageAddFrameType(builder, wire.FrameManifoldFrame)
	wire.MessageAddFrame(builder, manifoldOffset)
	msgOffset := wire.MessageEnd(builder)
	
	builder.Finish(msgOffset)

	return append([]byte{}, builder.FinishedBytes()...), nil
}
