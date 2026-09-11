#pragma once
#include "../shared/physics_v2_types.h"
#include "../shared/radix2_math.h"
namespace sensorium::kernels {
__global__ void mc_pic_deposit(const float* pos, const float* vel, const float* mass, const float* heat, float* out, unsigned* status, unsigned particles, MFHydroParamsV2 p);
__global__ void mc_pic_gather(const float* pos, const float* mass, float* pos_out, float* vel_out, float* heat_out, const MFHydroStateV2* state, unsigned* status, unsigned particles, MFHydroParamsV2 p);
__global__ void mc_hydro_export(const MFHydroStateV2* state, float* rho, float* mom, float* thermal, unsigned* status, MFHydroParamsV2 p);
__global__ void mc_hydro_budget(const MFHydroStateV2* initial, const MFHydroStateV2* stage, float* ledger, unsigned* status, MFHydroParamsV2 p);
__global__ void mc_poisson_init(const MFHydroStateV2* state, MRComplex* field, unsigned* status, MFHydroParamsV2 p, float G);
__global__ void mc_poisson_spectral(MRComplex* field, unsigned* status, MFHydroParamsV2 p);
__global__ void mc_poisson_export(const MRComplex* field, float* potential, float* acceleration, unsigned* status, MFHydroParamsV2 p);
__global__ void mc_gravity_kick(MFHydroStateV2* state, const float* acceleration, unsigned* status, MFHydroParamsV2 p);
__global__ void mc_poisson_axis(MRComplex* field, MRComplex* work, MFHydroParamsV2 p, unsigned axis, int sign, unsigned workspace);
__global__ void mc_pic_deposit_material(const float* pos, const float* vel, const float* mass, const float* heat, const float* total, float* out, unsigned* status, unsigned particles, MFHydroParamsV2 p);
__global__ void mc_remap_rows(const MFHydroStateV2* state, const float* pos, const float* columns, float* rows, unsigned* status, unsigned particles, MFHydroParamsV2 p, float width, float mass_scale);
__global__ void mc_remap_columns(const MFHydroStateV2* state, const float* pos, const float* mass, const float* rows, float* columns, unsigned* status, unsigned particles, MFHydroParamsV2 p, float width);
__global__ void mc_remap_residual(const MFHydroStateV2* state, const float* pos, const float* rows, const float* columns, float* residual, unsigned* status, unsigned particles, MFHydroParamsV2 p, float width, float mass_scale);
__global__ void mc_remap_gather(const MFHydroStateV2* state, const float* pos, const float* mass, const float* rows, float* velocity, float* heat, float* total, float* mixing, unsigned* status, unsigned particles, MFHydroParamsV2 p, float width, float mass_scale);
__global__ void mc_pilot_checked_step(const float* pos, const float* mass, const float* prior, const float* re, const float* im, float* position, float* guide, float* report, unsigned* status, unsigned particles, MFHydroParamsV2 p, float hbar, float tolerance, float maxcells);
__global__ void mc_contact_pack(const float* pos, const float* vel, const float* mass, const float* heat, MFParticleStateV2* state, unsigned* cell, unsigned* count, unsigned* status, unsigned particles, MFHydroParamsV2 p);
__global__ void mc_contact_sort(const unsigned* cell, const unsigned* starts, unsigned* offset, unsigned* sorted, unsigned* status, unsigned particles, MFHydroParamsV2 p);
__global__ void mc_contact_hash_kick(const MFParticleStateV2* initial, const MFParticleStateV2* stage, const unsigned* starts, const unsigned* sorted, MFParticleStateV2* out, MFContactResultV2* diagnostic, unsigned* status, unsigned particles, MFHydroParamsV2 p, MFContactParamsV2 contact, unsigned second, float max_speed);
__global__ void mc_contact_unpack(const MFParticleStateV2* state, float* vel, float* heat, unsigned particles, MFHydroParamsV2 p);
__global__ void mc_gravity_compatible(const MFHydroStateV2* flux_initial, const MFHydroStateV2* flux_stage, const MFHydroStateV2* state, const float* phi0, const float* phi1, const float* kick_work, MFHydroStateV2* out, unsigned* status, MFHydroParamsV2 p);
__global__ void mc_gravity_remap(const MFHydroStateV2* state, const float* pos, const float* mass, const float* rows, const float* phi0, const float* phi1, float* energy, float* work, unsigned* status, unsigned particles, MFHydroParamsV2 p, float width);
__global__ void mc_reciprocal_potential(const float* pos, const float* amplitude, const float* omega, const float* mode_omega, const float* linewidth, const unsigned* indices, const float* weights, float* potential, unsigned* status, unsigned particles, unsigned modes, unsigned anchors, float sigma, MFHydroParamsV2 p);
__global__ void mc_reciprocal_force(const float* pos, const float* amplitude, const float* omega, const float* mode_omega, const float* linewidth, const unsigned* indices, const float* weights, const float* occupation, float* force, unsigned* status, unsigned particles, unsigned modes, unsigned anchors, float sigma, MFHydroParamsV2 p);
__global__ void mc_reciprocal_occupation(const float* old_re, const float* old_im, const float* ledger, float* occupation, unsigned* status, unsigned modes, float domega, MFHydroParamsV2 p);
__global__ void mc_pilot_checked_time_step(const float* pos, const float* mass, const float* prior, const float* re0, const float* im0, const float* re1, const float* im1, float* position, float* guide, float* report, unsigned* status, unsigned particles, MFHydroParamsV2 p, float hbar, float tolerance, float maxcells);
}