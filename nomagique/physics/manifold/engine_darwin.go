//go:build darwin && cgo

package manifold

/*
#include "bridge.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

/*
Engine owns the process-wide shared Metal device and library. Fields allocate
their own pipelines and resident buffers on that toolchain.
*/
type Engine struct {
	handle     unsafe.Pointer
	config     Config
	fieldBytes uint64
	errBuf     [512]byte
}

/*
NewEngine ensures the shared Metal host exists and returns a token used to
create Fields without repeatedly loading the Metal library.
*/
func NewEngine(config Config) (*Engine, error) {
	cConfig := config.toC()
	errBuf := make([]byte, 512)

	handle := C.manifold_engine_create(
		&cConfig,
		unsafe.Pointer(&manifoldMetallib[0]),
		C.size_t(len(manifoldMetallib)),
		(*C.char)(unsafe.Pointer(&errBuf[0])),
		C.int(len(errBuf)),
	)

	if handle == nil {
		return nil, fmt.Errorf("physics: failed to create manifold engine")
	}

	return &Engine{
		handle: handle,
		config: config,
	}, nil
}

/*
Close releases the engine token. Shared Metal host stays alive for the process
so remaining Fields keep their pipelines.
*/
func (engine *Engine) Close() {
	if engine == nil || engine.handle == nil {
		return
	}

	C.manifold_engine_destroy(engine.handle)
	engine.handle = nil
}

/*
NewField allocates one symbol's resident GPU field buffers on the shared engine.
*/
func (engine *Engine) NewField() (*Solver, error) {
	if engine == nil || engine.handle == nil {
		return nil, fmt.Errorf("physics: engine is not initialized")
	}

	cConfig := engine.config.toC()
	errBuf := make([]byte, 512)

	handle := C.manifold_field_create(
		engine.handle,
		&cConfig,
		(*C.char)(unsafe.Pointer(&errBuf[0])),
		C.int(len(errBuf)),
	)

	if handle == nil {
		return nil, fmt.Errorf("physics: failed to create manifold field")
	}

	return &Solver{
		handle: handle,
		config: engine.config,
	}, nil
}

/*
FieldBytes reports the measured resident bytes of one newly allocated Field
probe, used to derive MaxFields from the available device working set.
*/
func (engine *Engine) FieldBytes() (uint64, error) {
	if engine == nil || engine.handle == nil {
		return 0, fmt.Errorf("physics: engine is not initialized")
	}

	if engine.fieldBytes > 0 {
		return engine.fieldBytes, nil
	}

	probe, err := engine.NewField()

	if err != nil {
		return 0, fmt.Errorf("physics: field memory probe failed: %w", err)
	}

	engine.fieldBytes = uint64(C.manifold_solver_resident_bytes(probe.handle))
	probe.Close()

	if engine.fieldBytes == 0 {
		return 0, fmt.Errorf("physics: field memory probe returned zero bytes")
	}

	return engine.fieldBytes, nil
}

/*
MaxFields returns the number of measured Fields that fit in the device's
currently available recommended working set.
*/
func (engine *Engine) MaxFields() (int, error) {
	if engine == nil || engine.handle == nil {
		return 0, fmt.Errorf("physics: engine is not initialized")
	}

	budget := uint64(C.manifold_device_working_set_budget())
	allocated := uint64(C.manifold_device_allocated_bytes())
	perField, err := engine.FieldBytes()

	if err != nil {
		return 0, err
	}

	if budget == 0 {
		return 0, fmt.Errorf("physics: device working-set budget is unavailable")
	}

	if allocated >= budget {
		return 0, fmt.Errorf(
			"physics: device working set exhausted: %d allocated of %d bytes",
			allocated,
			budget,
		)
	}

	maxFields := int((budget - allocated) / perField)

	if maxFields < 1 {
		return 0, fmt.Errorf(
			"physics: available device working set cannot fit one field",
		)
	}

	return maxFields, nil
}

/*
AllocatedBytes reports the Metal device's current resource allocation. It makes
field lifetime observable without relying on process-wide resident memory that
also includes unrelated Go and graphics clients.
*/
func (engine *Engine) AllocatedBytes() uint64 {
	if engine == nil || engine.handle == nil {
		return 0
	}

	return uint64(C.manifold_device_allocated_bytes())
}

/*
ResidentBytes returns the field buffer footprint for one Solver/Field handle.
*/
func (solver *Solver) ResidentBytes() uint64 {
	if solver == nil || solver.handle == nil {
		return 0
	}

	return uint64(C.manifold_solver_resident_bytes(solver.handle))
}
