//go:build darwin && cgo

// CGO compiles .cxx/.cpp, not .mm. This translation unit is the Metal host.
#define MANIFOLD_HOST_CXX 1
#include "ops.mm"

#include "physics_v2_metal_host.inc"

// Conservative remap kernels retain density separately from log row scaling; safeguarded secants accelerate the shared fixed point through mesh-resolution continuation.
#include "coupled_metal_host.inc"
