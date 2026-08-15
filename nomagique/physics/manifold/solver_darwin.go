//go:build darwin && cgo

//go:generate go run ./metallibgen

package manifold

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc -I${SRCDIR}
#cgo darwin LDFLAGS: -framework Metal -framework Foundation -framework Accelerate
#include "bridge.h"
#include <stdlib.h>
#include <dispatch/dispatch.h>
*/
import "C"
import (
	_ "embed"
	"fmt"
	"runtime"
	"unsafe"
)

//go:embed kernels.metallib
var manifoldMetallib []byte

/*
Solver runs the 3D PIC + GPE manifold on Metal.
*/
type Solver struct {
	handle unsafe.Pointer
	config Config
	errBuf [512]byte
}

/*
Oscillator couples one market actor into the coherence layer at a torus coordinate.
*/
type Oscillator struct {
	Phase     float64
	Omega     float64
	Amplitude float64
	PosX      float64
	PosY      float64
	PosZ      float64
	Heat      float64
	VelX      float64
	VelY      float64
	VelZ      float64
}

func (config Config) toC() C.ManifoldConfig {
	return C.ManifoldConfig{
		grid_x:               C.uint32_t(config.GridX),
		grid_y:               C.uint32_t(config.GridY),
		grid_z:               C.uint32_t(config.GridZ),
		domain_x:             C.float(config.DomainX),
		domain_y:             C.float(config.DomainY),
		domain_z:             C.float(config.DomainZ),
		dt:                   C.float(config.DeltaT),
		gamma:                C.float(config.Gamma),
		c_v:                  C.float(config.CV),
		rho_min:              C.float(config.RhoMin),
		p_min:                C.float(config.PMin),
		gas_envelope_rho_min: C.float(config.GasEnvelopeRhoMin),
		gas_p_min:            C.float(config.GasPMin),
		k_thermal:            C.float(config.KThermal),
		max_carriers:         C.uint32_t(config.MaxModes),
		hbar_eff:             C.float(config.HbarEffective()),
		g_interaction:        C.float(config.GInteraction()),
		energy_decay:         C.float(config.EnergyDecay()),
		metabolic_rate:       C.float(config.MetabolicRate()),
		coupling_scale:       C.float(config.CouplingScale()),
		gate_width_min:       C.float(config.GateWidthMin()),
		gate_width_max:       C.float(config.GateWidthMax()),
		boundary_x_low:       C.uint32_t(config.BoundaryXLow),
		boundary_x_high:      C.uint32_t(config.BoundaryXHigh),
		boundary_y_low:       C.uint32_t(config.BoundaryYLow),
		boundary_y_high:      C.uint32_t(config.BoundaryYHigh),
		boundary_z_low:       C.uint32_t(config.BoundaryZLow),
		boundary_z_high:      C.uint32_t(config.BoundaryZHigh),
	}
}

func NewSolver(config Config) (*Solver, error) {
	cConfig := config.toC()
	errBuf := make([]byte, 512)

	handle := C.manifold_solver_create(
		&cConfig,
		unsafe.Pointer(&manifoldMetallib[0]),
		C.size_t(len(manifoldMetallib)),
		(*C.char)(unsafe.Pointer(&errBuf[0])),
		C.int(len(errBuf)),
	)

	if handle == nil {
		return nil, fmt.Errorf("physics: failed to create solver")
	}

	return &Solver{
		handle: handle,
		config: config,
	}, nil
}

func (solver *Solver) Close() {
	if solver == nil || solver.handle == nil {
		return
	}

	C.manifold_solver_destroy(solver.handle)
	solver.handle = nil
}

func (solver *Solver) SetControls(controls RuntimeControls) error {
	if solver == nil || solver.handle == nil {
		return fmt.Errorf("physics: solver is not initialized")
	}

	if err := controls.Validate(); err != nil {
		return err
	}

	cControls := C.ManifoldControls{
		dt:                   C.float(controls.DeltaT),
		metabolic_rate:       C.float(controls.MetabolicRate),
		topdown_phase_scale:  C.float(controls.TopdownPhaseScale),
		topdown_energy_scale: C.float(controls.TopdownEnergyScale),
		g_interaction:        C.float(controls.GInteraction),
		energy_decay:         C.float(controls.EnergyDecay),
	}

	err := solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_set_controls(
			solver.handle,
			&cControls,
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
	runtime.KeepAlive(&cControls)

	return err
}

func (solver *Solver) ResetDeposits() error {
	return solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_reset_deposits(
			solver.handle,
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
}

func (solver *Solver) ResetSources() error {
	return solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_reset_sources(
			solver.handle,
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
}

func (solver *Solver) SourceCell(
	cellX, cellY, cellZ uint32,
	deltaMomX, deltaMomY, deltaMomZ, deltaRho, deltaE float64,
) error {
	return solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_source_cell(
			solver.handle,
			C.uint32_t(cellX),
			C.uint32_t(cellY),
			C.uint32_t(cellZ),
			C.float(deltaMomX),
			C.float(deltaMomY),
			C.float(deltaMomZ),
			C.float(deltaRho),
			C.float(deltaE),
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
}

func (solver *Solver) ApplySources() error {
	return solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_apply_sources(
			solver.handle,
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
}

func (solver *Solver) ReadCell(cellX, cellY, cellZ uint32) (rho, momX, momY, momZ, eInt float64, err error) {
	if solver == nil || solver.handle == nil {
		return 0, 0, 0, 0, 0, fmt.Errorf("physics: solver is not initialized")
	}

	var (
		cRho  C.float
		cMomX C.float
		cMomY C.float
		cMomZ C.float
		cEInt C.float
	)

	callErr := solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_read_cell(
			solver.handle,
			C.uint32_t(cellX),
			C.uint32_t(cellY),
			C.uint32_t(cellZ),
			&cRho,
			&cMomX,
			&cMomY,
			&cMomZ,
			&cEInt,
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
	runtime.KeepAlive(&cRho)
	runtime.KeepAlive(&cMomX)
	runtime.KeepAlive(&cMomY)
	runtime.KeepAlive(&cMomZ)
	runtime.KeepAlive(&cEInt)

	if callErr != nil {
		return 0, 0, 0, 0, 0, callErr
	}

	return float64(cRho), float64(cMomX), float64(cMomY), float64(cMomZ), float64(cEInt), nil
}

func (solver *Solver) DepositCell(
	cellX, cellY, cellZ uint32,
	rho, momX, momY, momZ, eInt float64,
) error {
	return solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_deposit_cell(
			solver.handle,
			C.uint32_t(cellX),
			C.uint32_t(cellY),
			C.uint32_t(cellZ),
			C.float(rho),
			C.float(momX),
			C.float(momY),
			C.float(momZ),
			C.float(eInt),
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
}

func (solver *Solver) SetOscillators(oscillators []Oscillator) error {
	if len(oscillators) == 0 {
		return fmt.Errorf("physics: at least one oscillator is required")
	}

	if uint32(len(oscillators)) > solver.config.MaxModes {
		return fmt.Errorf("physics: oscillator count %d exceeds max_modes %d", len(oscillators), solver.config.MaxModes)
	}

	cOscillators := make([]C.ManifoldOscillator, len(oscillators))

	for index, oscillator := range oscillators {
		cOscillators[index] = C.ManifoldOscillator{
			phase:     C.float(oscillator.Phase),
			omega:     C.float(oscillator.Omega),
			amplitude: C.float(oscillator.Amplitude),
			pos_x:     C.float(oscillator.PosX),
			pos_y:     C.float(oscillator.PosY),
			pos_z:     C.float(oscillator.PosZ),
			heat:      C.float(oscillator.Heat),
			vel_x:     C.float(oscillator.VelX),
			vel_y:     C.float(oscillator.VelY),
			vel_z:     C.float(oscillator.VelZ),
		}
	}

	err := solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_set_oscillators(
			solver.handle,
			(*C.ManifoldOscillator)(unsafe.Pointer(&cOscillators[0])),
			C.uint32_t(len(cOscillators)),
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
	runtime.KeepAlive(cOscillators)

	return err
}

func (solver *Solver) RunGasTransport() error {
	return solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_run_gas_transport(
			solver.handle,
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
}

func (solver *Solver) Step() (Reading, error) {
	var cReading C.ManifoldReading

	err := solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_step(
			solver.handle,
			&cReading,
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
	runtime.KeepAlive(&cReading)

	if err != nil {
		return Reading{}, err
	}

	return Reading{
		PressureGradX:    float64(cReading.pressure_grad_x),
		PressureGradY:    float64(cReading.pressure_grad_y),
		PressureGradZ:    float64(cReading.pressure_grad_z),
		PressureGradNorm: float64(cReading.pressure_grad_norm),
		Divergence:       float64(cReading.divergence),
		CoherenceMag2:    float64(cReading.coherence_mag2),
		GuidanceSpeed:    float64(cReading.guidance_speed),
		ViscosityProxy:   float64(cReading.viscosity_proxy),
	}, nil
}

func (solver *Solver) ReadRhoProjection() ([][]float64, error) {
	if solver == nil || solver.handle == nil {
		return nil, fmt.Errorf("physics: solver is not initialized")
	}

	gridX := solver.config.GridX
	gridZ := solver.config.GridZ
	length := int(gridX * gridZ)
	buffer := make([]float32, length)

	var (
		cGridX C.uint32_t
		cGridZ C.uint32_t
	)

	err := solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_read_rho_projection(
			solver.handle,
			(*C.float)(unsafe.Pointer(&buffer[0])),
			C.uint32_t(length),
			&cGridX,
			&cGridZ,
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
	runtime.KeepAlive(buffer)
	runtime.KeepAlive(&cGridX)
	runtime.KeepAlive(&cGridZ)

	if err != nil {
		return nil, err
	}

	return rowsFromBuffer(buffer, cGridX, cGridZ), nil
}

/*
ReadPilotWaveProjection returns the max-|psi|^2 slice over Y and the guidance
velocity field derived from the Bohm current at each projected column.
*/
func (solver *Solver) ReadPilotWaveProjection() (PilotWaveProjection, error) {
	if solver == nil || solver.handle == nil {
		return PilotWaveProjection{}, fmt.Errorf("physics: solver is not initialized")
	}

	gridX := solver.config.GridX
	gridZ := solver.config.GridZ
	length := int(gridX * gridZ)
	mag2 := make([]float32, length)
	velX := make([]float32, length)
	velZ := make([]float32, length)

	var (
		cGridX C.uint32_t
		cGridZ C.uint32_t
	)

	err := solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_read_pilot_wave_projection(
			solver.handle,
			(*C.float)(unsafe.Pointer(&mag2[0])),
			(*C.float)(unsafe.Pointer(&velX[0])),
			(*C.float)(unsafe.Pointer(&velZ[0])),
			C.uint32_t(length),
			&cGridX,
			&cGridZ,
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
	runtime.KeepAlive(mag2)
	runtime.KeepAlive(velX)
	runtime.KeepAlive(velZ)
	runtime.KeepAlive(&cGridX)
	runtime.KeepAlive(&cGridZ)

	if err != nil {
		return PilotWaveProjection{}, err
	}

	return PilotWaveProjection{
		Mag2: rowsFromBuffer(mag2, cGridX, cGridZ),
		VelX: rowsFromBuffer(velX, cGridX, cGridZ),
		VelZ: rowsFromBuffer(velZ, cGridX, cGridZ),
	}, nil
}

func rowsFromBuffer(buffer []float32, gridX, gridZ C.uint32_t) [][]float64 {
	rows := make([][]float64, gridZ)

	for zIndex := range gridZ {
		row := make([]float64, gridX)

		for xIndex := range gridX {
			row[xIndex] = float64(buffer[xIndex+zIndex*gridX])
		}

		rows[zIndex] = row
	}

	return rows
}

func (solver *Solver) ReadProjectionReading() (Reading, error) {
	if solver == nil || solver.handle == nil {
		return Reading{}, fmt.Errorf("physics: solver is not initialized")
	}

	var cReading C.ManifoldReading

	err := solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_read_projection_reading(
			solver.handle,
			&cReading,
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
	runtime.KeepAlive(&cReading)

	if err != nil {
		return Reading{}, err
	}

	return Reading{
		PressureGradNorm: float64(cReading.pressure_grad_norm),
		Divergence:       float64(cReading.divergence),
		ViscosityProxy:   float64(cReading.viscosity_proxy),
	}, nil
}

func (solver *Solver) ReadOscillators(count int) ([]Oscillator, error) {
	if solver == nil || solver.handle == nil {
		return nil, fmt.Errorf("physics: solver is not initialized")
	}

	if count <= 0 {
		return nil, fmt.Errorf("physics: oscillator count must be positive")
	}

	cOscillators := make([]C.ManifoldOscillator, count)

	err := solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_read_oscillators(
			solver.handle,
			(*C.ManifoldOscillator)(unsafe.Pointer(&cOscillators[0])),
			C.uint32_t(count),
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
	runtime.KeepAlive(cOscillators)

	if err != nil {
		return nil, err
	}

	oscillators := make([]Oscillator, count)

	for index := range cOscillators {
		oscillators[index] = Oscillator{
			Phase:     float64(cOscillators[index].phase),
			Omega:     float64(cOscillators[index].omega),
			Amplitude: float64(cOscillators[index].amplitude),
			PosX:      float64(cOscillators[index].pos_x),
			PosY:      float64(cOscillators[index].pos_y),
			PosZ:      float64(cOscillators[index].pos_z),
			Heat:      float64(cOscillators[index].heat),
			VelX:      float64(cOscillators[index].vel_x),
			VelY:      float64(cOscillators[index].vel_y),
			VelZ:      float64(cOscillators[index].vel_z),
		}
	}

	return oscillators, nil
}

func (solver *Solver) call(run func(errBuf []byte) C.int) error {
	if solver == nil || solver.handle == nil {
		return fmt.Errorf("physics: solver is not initialized")
	}

	errBuf := solver.errBuf[:]

	for index := range errBuf {
		errBuf[index] = 0
	}

	if run(errBuf) != 0 {
		return fmt.Errorf("physics: %s", cString(errBuf))
	}

	return nil
}

func cString(buffer []byte) string {
	for index, value := range buffer {
		if value == 0 {
			return string(buffer[:index])
		}
	}

	return string(buffer)
}
