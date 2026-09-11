//go:build darwin && cgo

// CGO compiles .cxx/.cpp, not .mm. This translation unit is the Metal host.
#define MANIFOLD_HOST_CXX 1
#include "ops.mm"

#include "physics_v2_metal_host.inc"

#include "coupled_metal_host.inc"
