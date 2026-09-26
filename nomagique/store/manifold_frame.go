package store

import (
	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

/*
	advance reconciles only the named market, leaving all other resident orders

in the same coupled domain. The source book owns departures and queue rank.
*/
func (server *ManifoldServer) advance(input manifoldInput) error {
	market := server.resident[input.symbol]

	if market == nil {
		market = &manifoldMarket{identities: map[string]int64{}}
		server.resident[input.symbol] = market
	}
	batch, departed := market.project(input, server.grid, &server.nextID)
	if len(departed) > 0 {
		if _, err := server.physics.Remove(departed); err != nil {
			return errnie.Error(err)
		}
	}
	if batch.N == 0 && server.physics.State().N == 0 {
		frame, err := server.snapshot(input, server.physics.State(), sensorium.Reading{})

		if err != nil {
			return errnie.Error(err)
		}
		server.publish(frame)
		return nil
	}
	state, err := server.physics.Step(batch)

	if err != nil {
		return errnie.Error(err)
	}
	frame, err := server.snapshot(input, state, server.physics.Reading())

	if err != nil {
		return errnie.Error(err)
	}
	server.publish(frame)
	return nil
}

/*
	snapshot publishes the actual particle arrays, field textures and measured

dynamics from one completed GPU step in an immutable Cap'n Proto message.
*/
func (server *ManifoldServer) snapshot(input manifoldInput, state *sensorium.State, reading sensorium.Reading) (ManifoldFrame, error) {
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return ManifoldFrame{}, errnie.Error(err)
	}
	frame, err := NewRootManifoldFrame(segment)

	if err != nil {
		message.Release()
		return ManifoldFrame{}, errnie.Error(err)
	}
	frame.SetEpoch(input.epoch)
	frame.SetSequence(input.sequence)
	frame.SetGridX(server.grid[0])
	frame.SetGridY(server.grid[1])
	frame.SetGridZ(server.grid[2])
	_, _, _, spacing := server.physics.Grid()
	frame.SetSpacing(spacing)
	frame.SetPopulation(uint32(state.N))
	frame.SetDivergence(reading.Divergence)
	frame.SetGuidanceSpeed(reading.GuidanceSpeed)
	frame.SetCoherence(reading.CoherenceMag2)
	frame.SetPressureGradient(reading.PressureGradNorm)
	frame.SetViscosity(reading.ViscosityProxy)
	frame.SetSynchronization(reading.KuramotoR)
	frame.SetPhysicalTime(reading.Health.Integrator.Time)
	frame.SetAcceptedStep(reading.Health.Integrator.AcceptedDT)
	frame.SetSubsteps(uint32(reading.Health.Integrator.Substeps))
	for _, field := range []struct {
		values   []float32
		allocate func(int32) (capnp.Float32List, error)
	}{
		{state.Pos, frame.NewPositions}, {state.Vel, frame.NewVelocities}, {state.Mass, frame.NewMasses}, {state.Energy, frame.NewEnergies},
		{state.Phase, frame.NewPhases}, {state.Omega, frame.NewFrequencies}, {state.Amp, frame.NewAmplitudes}, {state.Heat, frame.NewHeat},
	} {
		list, err := field.allocate(int32(len(field.values)))

		if err != nil {
			message.Release()
			return ManifoldFrame{}, errnie.Error(err)
		}
		for index, value := range field.values {
			list.Set(index, value)
		}
	}
	identities, err := frame.NewContentIds(int32(state.N))

	if err != nil {
		message.Release()
		return ManifoldFrame{}, errnie.Error(err)
	}
	for index, value := range state.ContentIDs {
		identities.Set(index, value)
	}
	if err := server.fields(frame); err != nil {
		message.Release()
		return ManifoldFrame{}, errnie.Error(err)
	}
	return frame, nil
}

func (server *ManifoldServer) fields(frame ManifoldFrame) error {
	cells := int(server.grid[0]) * int(server.grid[1]) * int(server.grid[2])
	momentum, energy, real, imaginary := make([]float32, 4*cells), make([]float32, cells), make([]float32, cells), make([]float32, cells)
	densityScale, momentumScale, energyScale, waveScale := server.physics.PackFields(momentum, energy, real, imaginary)
	frame.SetDensityScale(densityScale)
	frame.SetMomentumScale(momentumScale)
	frame.SetEnergyScale(energyScale)
	frame.SetWaveScale(waveScale)
	for _, field := range []struct {
		values   []float32
		allocate func(int32) (capnp.Float32List, error)
	}{
		{momentum, frame.NewDensityMomentum}, {energy, frame.NewFieldEnergy}, {real, frame.NewWaveReal}, {imaginary, frame.NewWaveImaginary},
	} {
		list, err := field.allocate(int32(len(field.values)))

		if err != nil {
			return errnie.Error(err)
		}
		for index, value := range field.values {
			list.Set(index, value)
		}
	}
	return nil
}
