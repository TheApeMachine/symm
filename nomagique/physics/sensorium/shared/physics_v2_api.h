#ifndef MANIFOLD_PHYSICS_V2_API_H
#define MANIFOLD_PHYSICS_V2_API_H
#include "physics_v2_types.h"
/* Include after the existing ManifoldContext/ManifoldBuffer declarations. */
#ifdef __cplusplus
extern "C" {
#endif
unsigned manifold_physics_abi_version(void);
/* New functions are synchronous. False may mean invalid arguments, numerical
   rejection, or a device error. Per-cell status is valid after a validation
   dispatch, not necessarily after an argument/driver failure. Numerical STEP
   rejection before commit leaves input/output unchanged; a device failure during
   commit may invalidate output. Workspaces/status are scratch and cannot alias
   other inputs. Input==output is supported for STEP. Pack/unpack and diagnostics
   are not transactional. Use a serialized owner for a context and its buffers. */
bool manifold_hydro_pack_v2(ManifoldContext*,ManifoldBuffer* rho,ManifoldBuffer* mom,
    ManifoldBuffer* thermal,ManifoldBuffer* state,ManifoldBuffer* status,const MFHydroParamsV2*);
bool manifold_hydro_unpack_v2(ManifoldContext*,ManifoldBuffer* state,ManifoldBuffer* rho,
    ManifoldBuffer* mom,ManifoldBuffer* thermal,ManifoldBuffer* status,const MFHydroParamsV2*);
bool manifold_hydro_step_v2(ManifoldContext*,ManifoldBuffer* initial,ManifoldBuffer* output,
    ManifoldBuffer* work1,ManifoldBuffer* work2,ManifoldBuffer* status,
    ManifoldBuffer* acceleration,const MFHydroParamsV2*);
bool manifold_hydro_diagnostics_v2(ManifoldContext*,ManifoldBuffer* state,
    ManifoldBuffer* acceleration,ManifoldBuffer* diagnostics,ManifoldBuffer* status,const MFHydroParamsV2*);
bool manifold_wave_step_v2(ManifoldContext*,ManifoldBuffer* initial,ManifoldBuffer* output,
    ManifoldBuffer* work1,ManifoldBuffer* work2,ManifoldBuffer* potential,
    ManifoldBuffer* status,const MFWaveParamsV2*);
bool manifold_wave_diagnostics_v2(ManifoldContext*,ManifoldBuffer* state,
    ManifoldBuffer* potential,ManifoldBuffer* diagnostics,ManifoldBuffer* status,const MFWaveParamsV2*);
bool manifold_contact_step_v2(ManifoldContext*,ManifoldBuffer* initial,ManifoldBuffer* output,
    ManifoldBuffer* work1,ManifoldBuffer* work2,ManifoldBuffer* status,const MFContactParamsV2*);
bool manifold_contact_diagnostics_v2(ManifoldContext*,ManifoldBuffer* state,
    ManifoldBuffer* diagnostics,ManifoldBuffer* status,const MFContactParamsV2*);
#ifdef __cplusplus
}
#endif
#endif
