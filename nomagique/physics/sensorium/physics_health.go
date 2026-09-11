package sensorium

import "reflect"

// Health values are retained in Reading; presentation is a viewer concern.
// Different representations of the SAME matter are never added together.
type IntegratorHealth struct {
	ContactDT                                                           float64
	RequestedDT, TargetDT, AcceptedDT, LastDT, MinDT, Time              float64
	HyperbolicDT, ViscousDT, ThermalDT, ParticleDT, PhaseDT, CombinedDT float64
	Substeps, Rejections                                                int
}
type GasHealth struct {
	Mass, Internal, Kinetic, Total                                       float64
	Momentum                                                             [3]float64
	MinDensity, MinPressure, MinTemperature, MaxSpeed, MaxSound, MaxMach float64
	VorticityRMS, VorticityMax, StrainRMS, StrainMax, ViscousPower       float64
	DisagreementMean, DisagreementMax, AuxiliaryFraction                 float64
	DisagreementCount, ColdMovingCells                                   int
}
type WaveHealth struct {
	Norm, Kinetic, Potential, Nonlinear, Chemical float64
	ProjectedNorm, PhasePotential                 float64
}
type PilotHealth struct {
	DensityP01, DensityP10, DensityMedian, IntegrationErrorMax       float64
	SpeedRMS, SpeedMax, DisplacementRMS, DisplacementMax, MinDensity float64
	DensityRegularizerFraction, MassRegularizerFraction              float64
}

// Work is SIGNED. In particular damping can increase a negative-potential
// Hamiltonian while still decreasing norm. That is not a positive heat source.
type SourceLedger struct {
	CoherenceMechanicalWork, CoherenceParameterWork, CoherenceMotionPotentialChange float64

	GravityRemapWork, GravityRemapResidual                                               float64
	ContactMaterialWork                                                                  float64
	ExogenousParticleEnergy, PlanckToOscillator, PlanckRoundoff                          float64
	CouplingHeatExport, CoherenceDriveWork, CoherenceDampingWork, CoherencePotentialWork float64
	CoherenceDriveNorm, CoherenceDampingNorm, ConservativeWaveError                      float64
	PICDepositEnergyResidual, ParticleBalanceResidual                                    float64
	PhasePotentialWork, PhaseDriveWork, PhaseDissipation                                 float64
	PilotWork, PICRemapEnergy, PICRemapMass, GasEnergyResidual                           float64
	PICRemapMomentum                                                                     [3]float64
	GravityWork, GravityFieldEnergy, GravityBalanceResidual, GravityRebuildChange        float64
	ContactToHeat, ThermalConductionNet, ViscousToHeat                                   float64
	MaterialRoundoff, RemapMixingToAuxiliary                                             float64
}
type RemapHealth struct {
	MaxMarginalResidual, MassRoundoffScale, MixingToAuxiliary, EnergyResidual, WidthCells float64
	MomentumResidual                                                                      [3]float64
	Iterations                                                                            int
}
type ContactHealth struct {
	Enabled                  bool
	ElasticEnergy, LimitedDT float64
	Kicks                    int
}
type PhysicsImplementation struct {
	ConservativeRemap, ReciprocalSpectralForce, NodeCheckedSpaceTimeGuidance bool
	ProjectedSpatialWave, ExternalFieldDrive                                 bool
	NativeParityValidated, MarketCalibrationValidated                        bool
}
type PhysicsHealth struct {
	Implementation                                       PhysicsImplementation
	Contact                                              ContactHealth
	Remap                                                RemapHealth
	ParticleMaterialTotal, ParticleEnergyDisagreement    float64
	Integrator                                           IntegratorHealth
	Gas                                                  GasHealth
	Wave                                                 WaveHealth
	Pilot                                                PilotHealth
	Sources                                              SourceLedger
	ParticleThermal, ParticleOscillator, ParticleKinetic float64
	SpatialSigmaRaw, SpatialSigmaUsed                    float64
	SigmaUnderResolved, SigmaUniformLimit                bool
	UnresolvedCoupling                                   bool // projected-wave density equivariance is not implied by conservative remap
}

// Diagnostics are small fixed POD-like structs. Reflective traversal keeps new
// floating fields subject to validation instead of forgetting a hand-written list.
func (health PhysicsHealth) IsFinite() bool { return finiteHealthValue(reflect.ValueOf(health)) }
func finiteHealthValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Float64, reflect.Float32:
		return finite(value.Float())
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if !finiteHealthValue(value.Field(i)) {
				return false
			}
		}
	case reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if !finiteHealthValue(value.Index(i)) {
				return false
			}
		}
	}
	return true
}
