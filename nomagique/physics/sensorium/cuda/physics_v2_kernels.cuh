#ifndef MANIFOLD_PHYSICS_V2_KERNELS_CUH
#define MANIFOLD_PHYSICS_V2_KERNELS_CUH
#include <cuda_runtime.h>
#include "../shared/physics_v2_types.h"
namespace sensorium::kernels {

__global__ void mf_hydro_pack_v2(
    const float* rho,
    const float* mom,
    const float* thermal,
    MFHydroStateV2* out,
    unsigned* status,
    MFHydroParamsV2 p
);

__global__ void mf_hydro_unpack_v2(
    const MFHydroStateV2* in,
    float* rho,
    float* mom,
    float* thermal,
    unsigned* status,
    MFHydroParamsV2 p
);

__global__ void mf_hydro_stage_v2(
    const MFHydroStateV2* initial,
    const MFHydroStateV2* stage,
    const float* acceleration,
    MFHydroStateV2* out,
    unsigned* status,
    MFHydroParamsV2 p,
    unsigned second
);

__global__ void mf_hydro_diagnostics_v2(
    const MFHydroStateV2* in,
    const float* acceleration,
    MFHydroDiagnosticV2* out,
    unsigned* status,
    MFHydroParamsV2 p
);

__global__ void mf_wave_local_v2(
    const MFWaveValueV2* in,
    MFWaveValueV2* out,
    const float* potential,
    unsigned* status,
    MFWaveParamsV2 p,
    float dt,
    unsigned reset
);

__global__ void mf_wave_bond_v2(
    const MFWaveValueV2* in,
    MFWaveValueV2* out,
    unsigned* status,
    MFWaveParamsV2 p,
    unsigned axis,
    unsigned color,
    float dt
);

__global__ void mf_wave_diagnostics_v2(
    const MFWaveValueV2* in,
    const float* potential,
    MFWaveDiagnosticV2* out,
    unsigned* status,
    MFWaveParamsV2 p
);

__global__ void mf_contact_stage_v2(
    const MFParticleStateV2* initial,
    const MFParticleStateV2* stage,
    MFParticleStateV2* out,
    unsigned* status,
    MFContactParamsV2 p,
    unsigned second
);

__global__ void mf_contact_diagnostics_v2(
    const MFParticleStateV2* in,
    MFContactResultV2* out,
    unsigned* status,
    MFContactParamsV2 p
);

__global__ void mf_hydro_copy_v2(
    const MFHydroStateV2* in,
    MFHydroStateV2* out,
    unsigned n
);

__global__ void mf_wave_copy_v2(
    const MFWaveValueV2* in,
    MFWaveValueV2* out,
    unsigned n
);

__global__ void mf_contact_copy_v2(
    const MFParticleStateV2* in,
    MFParticleStateV2* out,
    unsigned n
);

}
#endif
