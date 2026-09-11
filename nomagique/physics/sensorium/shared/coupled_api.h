#ifndef MF_COUPLED_API_H
#define MF_COUPLED_API_H
#include "physics_v2_types.h"
#ifdef __cplusplus
extern "C" {
#endif
bool manifold_pic_deposit_dual(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,unsigned,const MFHydroParamsV2*);
bool manifold_pic_gather_dual(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,unsigned,const MFHydroParamsV2*);
bool manifold_hydro_export_dual(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,const MFHydroParamsV2*);
bool manifold_hydro_budget(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,const MFHydroParamsV2*);
bool manifold_periodic_poisson(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,const MFHydroParamsV2*,float);
bool manifold_gravity_kick(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,const MFHydroParamsV2*);
bool manifold_phase_copy_ledger(ManifoldContext*,ManifoldBuffer*,unsigned);
bool manifold_coherence_copy_ledger(ManifoldContext*,ManifoldBuffer*,unsigned);
bool manifold_pic_deposit_material(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,unsigned,const MFHydroParamsV2*);
bool manifold_pic_remap_conservative(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,unsigned,const MFHydroParamsV2*,float,float,unsigned,float);
bool manifold_pilot_checked(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,unsigned,const MFHydroParamsV2*,float,float,float);
bool manifold_contact_hash_kick(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,const MFHydroParamsV2*,const MFContactParamsV2*,unsigned);
bool manifold_gravity_compatible(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,const MFHydroParamsV2*);
bool manifold_coherence_reciprocal(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,unsigned,unsigned,unsigned,float,float,const MFHydroParamsV2*);
bool manifold_pilot_checked_time(ManifoldContext*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,ManifoldBuffer*,unsigned,const MFHydroParamsV2*,float,float,float);
#ifdef __cplusplus
}
#endif
#endif
