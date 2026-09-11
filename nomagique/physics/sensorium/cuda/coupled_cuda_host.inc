#include "coupled_kernels.cuh"
#include "../shared/coupled_core.h"
#include "../shared/conservative_remap.h"
#include "../shared/pilot_checked.h"
#include "../shared/pilot_temporal.h"
#include "../shared/contact_hash.h"
#include "../shared/gravity_compatible.h"
#include "../shared/reciprocal_coherence.h"
#define MC_UINT_ADD(p,v) atomicAdd(p,v)
static bool mc_launch_mc_pic_deposit(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* vel, ManifoldBuffer* mass, ManifoldBuffer* heat, ManifoldBuffer* out, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p) {
    kernels::mc_pic_deposit<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const float>(pos), ptr<const float>(vel), ptr<const float>(mass), ptr<const float>(heat), ptr<float>(out), ptr<unsigned>(status), particles, p);
    ctx->launch_status(cudaGetLastError(),"mc_pic_deposit");return !ctx->error[0];
}
static bool mc_launch_mc_pic_gather(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* mass, ManifoldBuffer* pos_out, ManifoldBuffer* vel_out, ManifoldBuffer* heat_out, ManifoldBuffer* state, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p) {
    kernels::mc_pic_gather<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const float>(pos), ptr<const float>(mass), ptr<float>(pos_out), ptr<float>(vel_out), ptr<float>(heat_out), ptr<const MFHydroStateV2>(state), ptr<unsigned>(status), particles, p);
    ctx->launch_status(cudaGetLastError(),"mc_pic_gather");return !ctx->error[0];
}
static bool mc_launch_mc_hydro_export(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* rho, ManifoldBuffer* mom, ManifoldBuffer* thermal, ManifoldBuffer* status, MFHydroParamsV2 p) {
    kernels::mc_hydro_export<<<(p.n+255)/256,256,0,ctx->stream>>>(ptr<const MFHydroStateV2>(state), ptr<float>(rho), ptr<float>(mom), ptr<float>(thermal), ptr<unsigned>(status), p);
    ctx->launch_status(cudaGetLastError(),"mc_hydro_export");return !ctx->error[0];
}
static bool mc_launch_mc_hydro_budget(ManifoldContext* ctx, ManifoldBuffer* initial, ManifoldBuffer* stage, ManifoldBuffer* ledger, ManifoldBuffer* status, MFHydroParamsV2 p) {
    kernels::mc_hydro_budget<<<(p.n+255)/256,256,0,ctx->stream>>>(ptr<const MFHydroStateV2>(initial), ptr<const MFHydroStateV2>(stage), ptr<float>(ledger), ptr<unsigned>(status), p);
    ctx->launch_status(cudaGetLastError(),"mc_hydro_budget");return !ctx->error[0];
}
static bool mc_launch_mc_poisson_init(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* field, ManifoldBuffer* status, MFHydroParamsV2 p, float G) {
    kernels::mc_poisson_init<<<(p.n+255)/256,256,0,ctx->stream>>>(ptr<const MFHydroStateV2>(state), ptr<MRComplex>(field), ptr<unsigned>(status), p, G);
    ctx->launch_status(cudaGetLastError(),"mc_poisson_init");return !ctx->error[0];
}
static bool mc_launch_mc_poisson_spectral(ManifoldContext* ctx, ManifoldBuffer* field, ManifoldBuffer* status, MFHydroParamsV2 p) {
    kernels::mc_poisson_spectral<<<(p.n+255)/256,256,0,ctx->stream>>>(ptr<MRComplex>(field), ptr<unsigned>(status), p);
    ctx->launch_status(cudaGetLastError(),"mc_poisson_spectral");return !ctx->error[0];
}
static bool mc_launch_mc_poisson_export(ManifoldContext* ctx, ManifoldBuffer* field, ManifoldBuffer* potential, ManifoldBuffer* acceleration, ManifoldBuffer* status, MFHydroParamsV2 p) {
    kernels::mc_poisson_export<<<(p.n+255)/256,256,0,ctx->stream>>>(ptr<const MRComplex>(field), ptr<float>(potential), ptr<float>(acceleration), ptr<unsigned>(status), p);
    ctx->launch_status(cudaGetLastError(),"mc_poisson_export");return !ctx->error[0];
}
static bool mc_launch_mc_gravity_kick(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* acceleration, ManifoldBuffer* status, MFHydroParamsV2 p) {
    kernels::mc_gravity_kick<<<(p.n+255)/256,256,0,ctx->stream>>>(ptr<MFHydroStateV2>(state), ptr<const float>(acceleration), ptr<unsigned>(status), p);
    ctx->launch_status(cudaGetLastError(),"mc_gravity_kick");return !ctx->error[0];
}
static bool mc_launch_mc_poisson_axis(ManifoldContext* ctx, ManifoldBuffer* field, ManifoldBuffer* work, MFHydroParamsV2 p, unsigned axis, int sign, unsigned workspace) {
    kernels::mc_poisson_axis<<<p.n/mc_line_length(axis,p),256,0,ctx->stream>>>(ptr<MRComplex>(field), ptr<MRComplex>(work), p, axis, sign, workspace);
    ctx->launch_status(cudaGetLastError(),"mc_poisson_axis");return !ctx->error[0];
}
static bool mc_launch_mc_pic_deposit_material(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* vel, ManifoldBuffer* mass, ManifoldBuffer* heat, ManifoldBuffer* total, ManifoldBuffer* out, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p) {
    kernels::mc_pic_deposit_material<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const float>(pos), ptr<const float>(vel), ptr<const float>(mass), ptr<const float>(heat), ptr<const float>(total), ptr<float>(out), ptr<unsigned>(status), particles, p);
    ctx->launch_status(cudaGetLastError(),"mc_pic_deposit_material");return !ctx->error[0];
}
static bool mc_launch_mc_remap_rows(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* pos, ManifoldBuffer* columns, ManifoldBuffer* rows, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float width, float mass_scale) {
    kernels::mc_remap_rows<<<(p.n+255)/256,256,0,ctx->stream>>>(ptr<const MFHydroStateV2>(state), ptr<const float>(pos), ptr<const float>(columns), ptr<float>(rows), ptr<unsigned>(status), particles, p, width, mass_scale);
    ctx->launch_status(cudaGetLastError(),"mc_remap_rows");return !ctx->error[0];
}
static bool mc_launch_mc_remap_columns(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* pos, ManifoldBuffer* mass, ManifoldBuffer* rows, ManifoldBuffer* columns, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float width) {
    kernels::mc_remap_columns<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const MFHydroStateV2>(state), ptr<const float>(pos), ptr<const float>(mass), ptr<const float>(rows), ptr<float>(columns), ptr<unsigned>(status), particles, p, width);
    ctx->launch_status(cudaGetLastError(),"mc_remap_columns");return !ctx->error[0];
}
static bool mc_launch_mc_remap_residual(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* pos, ManifoldBuffer* rows, ManifoldBuffer* columns, ManifoldBuffer* residual, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float width, float mass_scale) {
    kernels::mc_remap_residual<<<(p.n+255)/256,256,0,ctx->stream>>>(ptr<const MFHydroStateV2>(state), ptr<const float>(pos), ptr<const float>(rows), ptr<const float>(columns), ptr<float>(residual), ptr<unsigned>(status), particles, p, width, mass_scale);
    ctx->launch_status(cudaGetLastError(),"mc_remap_residual");return !ctx->error[0];
}
static bool mc_launch_mc_remap_gather(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* pos, ManifoldBuffer* mass, ManifoldBuffer* rows, ManifoldBuffer* velocity, ManifoldBuffer* heat, ManifoldBuffer* total, ManifoldBuffer* mixing, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float width, float mass_scale) {
    kernels::mc_remap_gather<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const MFHydroStateV2>(state), ptr<const float>(pos), ptr<const float>(mass), ptr<const float>(rows), ptr<float>(velocity), ptr<float>(heat), ptr<float>(total), ptr<float>(mixing), ptr<unsigned>(status), particles, p, width, mass_scale);
    ctx->launch_status(cudaGetLastError(),"mc_remap_gather");return !ctx->error[0];
}
static bool mc_launch_mc_pilot_checked_step(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* mass, ManifoldBuffer* prior, ManifoldBuffer* re, ManifoldBuffer* im, ManifoldBuffer* position, ManifoldBuffer* guide, ManifoldBuffer* report, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float hbar, float tolerance, float maxcells) {
    kernels::mc_pilot_checked_step<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const float>(pos), ptr<const float>(mass), ptr<const float>(prior), ptr<const float>(re), ptr<const float>(im), ptr<float>(position), ptr<float>(guide), ptr<float>(report), ptr<unsigned>(status), particles, p, hbar, tolerance, maxcells);
    ctx->launch_status(cudaGetLastError(),"mc_pilot_checked_step");return !ctx->error[0];
}
static bool mc_launch_mc_contact_pack(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* vel, ManifoldBuffer* mass, ManifoldBuffer* heat, ManifoldBuffer* state, ManifoldBuffer* cell, ManifoldBuffer* count, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p) {
    kernels::mc_contact_pack<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const float>(pos), ptr<const float>(vel), ptr<const float>(mass), ptr<const float>(heat), ptr<MFParticleStateV2>(state), ptr<unsigned>(cell), ptr<unsigned>(count), ptr<unsigned>(status), particles, p);
    ctx->launch_status(cudaGetLastError(),"mc_contact_pack");return !ctx->error[0];
}
static bool mc_launch_mc_contact_sort(ManifoldContext* ctx, ManifoldBuffer* cell, ManifoldBuffer* starts, ManifoldBuffer* offset, ManifoldBuffer* sorted, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p) {
    kernels::mc_contact_sort<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const unsigned>(cell), ptr<const unsigned>(starts), ptr<unsigned>(offset), ptr<unsigned>(sorted), ptr<unsigned>(status), particles, p);
    ctx->launch_status(cudaGetLastError(),"mc_contact_sort");return !ctx->error[0];
}
static bool mc_launch_mc_contact_hash_kick(ManifoldContext* ctx, ManifoldBuffer* initial, ManifoldBuffer* stage, ManifoldBuffer* starts, ManifoldBuffer* sorted, ManifoldBuffer* out, ManifoldBuffer* diagnostic, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, MFContactParamsV2 contact, unsigned second, float max_speed) {
    kernels::mc_contact_hash_kick<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const MFParticleStateV2>(initial), ptr<const MFParticleStateV2>(stage), ptr<const unsigned>(starts), ptr<const unsigned>(sorted), ptr<MFParticleStateV2>(out), ptr<MFContactResultV2>(diagnostic), ptr<unsigned>(status), particles, p, contact, second, max_speed);
    ctx->launch_status(cudaGetLastError(),"mc_contact_hash_kick");return !ctx->error[0];
}
static bool mc_launch_mc_contact_unpack(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* vel, ManifoldBuffer* heat, unsigned particles, MFHydroParamsV2 p) {
    kernels::mc_contact_unpack<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const MFParticleStateV2>(state), ptr<float>(vel), ptr<float>(heat), particles, p);
    ctx->launch_status(cudaGetLastError(),"mc_contact_unpack");return !ctx->error[0];
}
static bool mc_launch_mc_gravity_compatible(ManifoldContext* ctx, ManifoldBuffer* flux_initial, ManifoldBuffer* flux_stage, ManifoldBuffer* state, ManifoldBuffer* phi0, ManifoldBuffer* phi1, ManifoldBuffer* kick_work, ManifoldBuffer* out, ManifoldBuffer* status, MFHydroParamsV2 p) {
    kernels::mc_gravity_compatible<<<(p.n+255)/256,256,0,ctx->stream>>>(ptr<const MFHydroStateV2>(flux_initial), ptr<const MFHydroStateV2>(flux_stage), ptr<const MFHydroStateV2>(state), ptr<const float>(phi0), ptr<const float>(phi1), ptr<const float>(kick_work), ptr<MFHydroStateV2>(out), ptr<unsigned>(status), p);
    ctx->launch_status(cudaGetLastError(),"mc_gravity_compatible");return !ctx->error[0];
}
static bool mc_launch_mc_gravity_remap(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* pos, ManifoldBuffer* mass, ManifoldBuffer* rows, ManifoldBuffer* phi0, ManifoldBuffer* phi1, ManifoldBuffer* energy, ManifoldBuffer* work, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float width) {
    kernels::mc_gravity_remap<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const MFHydroStateV2>(state), ptr<const float>(pos), ptr<const float>(mass), ptr<const float>(rows), ptr<const float>(phi0), ptr<const float>(phi1), ptr<float>(energy), ptr<float>(work), ptr<unsigned>(status), particles, p, width);
    ctx->launch_status(cudaGetLastError(),"mc_gravity_remap");return !ctx->error[0];
}
static bool mc_launch_mc_reciprocal_potential(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* amplitude, ManifoldBuffer* omega, ManifoldBuffer* mode_omega, ManifoldBuffer* linewidth, ManifoldBuffer* indices, ManifoldBuffer* weights, ManifoldBuffer* potential, ManifoldBuffer* status, unsigned particles, unsigned modes, unsigned anchors, float sigma, MFHydroParamsV2 p) {
    kernels::mc_reciprocal_potential<<<(modes+255)/256,256,0,ctx->stream>>>(ptr<const float>(pos), ptr<const float>(amplitude), ptr<const float>(omega), ptr<const float>(mode_omega), ptr<const float>(linewidth), ptr<const unsigned>(indices), ptr<const float>(weights), ptr<float>(potential), ptr<unsigned>(status), particles, modes, anchors, sigma, p);
    ctx->launch_status(cudaGetLastError(),"mc_reciprocal_potential");return !ctx->error[0];
}
static bool mc_launch_mc_reciprocal_force(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* amplitude, ManifoldBuffer* omega, ManifoldBuffer* mode_omega, ManifoldBuffer* linewidth, ManifoldBuffer* indices, ManifoldBuffer* weights, ManifoldBuffer* occupation, ManifoldBuffer* force, ManifoldBuffer* status, unsigned particles, unsigned modes, unsigned anchors, float sigma, MFHydroParamsV2 p) {
    kernels::mc_reciprocal_force<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const float>(pos), ptr<const float>(amplitude), ptr<const float>(omega), ptr<const float>(mode_omega), ptr<const float>(linewidth), ptr<const unsigned>(indices), ptr<const float>(weights), ptr<const float>(occupation), ptr<float>(force), ptr<unsigned>(status), particles, modes, anchors, sigma, p);
    ctx->launch_status(cudaGetLastError(),"mc_reciprocal_force");return !ctx->error[0];
}
static bool mc_launch_mc_reciprocal_occupation(ManifoldContext* ctx, ManifoldBuffer* old_re, ManifoldBuffer* old_im, ManifoldBuffer* ledger, ManifoldBuffer* occupation, ManifoldBuffer* status, unsigned modes, float domega, MFHydroParamsV2 p) {
    kernels::mc_reciprocal_occupation<<<(modes+255)/256,256,0,ctx->stream>>>(ptr<const float>(old_re), ptr<const float>(old_im), ptr<const float>(ledger), ptr<float>(occupation), ptr<unsigned>(status), modes, domega, p);
    ctx->launch_status(cudaGetLastError(),"mc_reciprocal_occupation");return !ctx->error[0];
}
static bool mc_launch_mc_pilot_checked_time_step(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* mass, ManifoldBuffer* prior, ManifoldBuffer* re0, ManifoldBuffer* im0, ManifoldBuffer* re1, ManifoldBuffer* im1, ManifoldBuffer* position, ManifoldBuffer* guide, ManifoldBuffer* report, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float hbar, float tolerance, float maxcells) {
    kernels::mc_pilot_checked_time_step<<<(particles+255)/256,256,0,ctx->stream>>>(ptr<const float>(pos), ptr<const float>(mass), ptr<const float>(prior), ptr<const float>(re0), ptr<const float>(im0), ptr<const float>(re1), ptr<const float>(im1), ptr<float>(position), ptr<float>(guide), ptr<float>(report), ptr<unsigned>(status), particles, p, hbar, tolerance, maxcells);
    ctx->launch_status(cudaGetLastError(),"mc_pilot_checked_time_step");return !ctx->error[0];
}
#include "../shared/coupled_host.inc"
