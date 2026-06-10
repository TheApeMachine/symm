//go:build darwin && cgo

//go:generate go run ./metallibgen

package physics

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

func NewSolver(config Config) (*Solver, error) {
	cConfig := C.ManifoldConfig{
		grid_x:         C.uint32_t(config.GridX),
		grid_y:         C.uint32_t(config.GridY),
		grid_z:         C.uint32_t(config.GridZ),
		domain_x:       C.float(config.DomainX),
		domain_y:       C.float(config.DomainY),
		domain_z:       C.float(config.DomainZ),
		dt:             C.float(config.DeltaT),
		gamma:          C.float(config.Gamma),
		c_v:            C.float(config.CV),
		rho_min:        C.float(config.RhoMin),
		p_min:          C.float(config.PMin),
		k_thermal:      C.float(config.KThermal),
		max_carriers:   C.uint32_t(config.MaxModes),
		hbar_eff:       C.float(config.HbarEffective()),
		g_interaction:  C.float(config.GInteraction()),
		energy_decay:   C.float(config.EnergyDecay()),
		metabolic_rate: C.float(config.MetabolicRate()),
		coupling_scale: C.float(config.CouplingScale()),
		gate_width_min: C.float(config.GateWidthMin()),
		gate_width_max: C.float(config.GateWidthMax()),
	}

	errBuf := make([]byte, 512)

	handle := C.manifold_solver_create(
		&cConfig,
		unsafe.Pointer(&manifoldMetallib[0]),
		C.size_t(len(manifoldMetallib)),
		(*C.char)(unsafe.Pointer(&errBuf[0])),
		C.int(len(errBuf)),
	)

	if handle == nil {
		return nil, fmt.Errorf("physics: %s", cString(errBuf))
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

func (solver *Solver) ResetDeposits() error {
	return solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_reset_deposits(
			solver.handle,
			(*C.char)(unsafe.Pointer(&errBuf[0])),
			C.int(len(errBuf)),
		)
	})
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

	return solver.call(func(errBuf []byte) C.int {
		return C.manifold_solver_set_oscillators(
			solver.handle,
			(*C.ManifoldOscillator)(unsafe.Pointer(&cOscillators[0])),
			C.uint32_t(len(cOscillators)),
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

	if err != nil {
		return nil, err
	}

	rows := make([][]float64, cGridZ)

	for zIndex := range cGridZ {
		row := make([]float64, cGridX)

		for xIndex := range cGridX {
			row[xIndex] = float64(buffer[xIndex+zIndex*cGridX])
		}

		rows[zIndex] = row
	}

	return rows, nil
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
	errBuf := make([]byte, 512)

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
