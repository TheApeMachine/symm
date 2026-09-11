#include "shared/coupled_core.h"
#include "shared/conservative_remap.h"
#include "shared/pilot_checked.h"
#include "shared/pilot_temporal.h"
#include "shared/contact_hash.h"
#include "shared/gravity_compatible.h"
#include "shared/reciprocal_coherence.h"
#define MC_UINT_ADD(p,v) atomic_fetch_add_explicit(p,v,memory_order_relaxed)
static bool mc_launch_mc_pic_deposit(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* vel, ManifoldBuffer* mass, ManifoldBuffer* heat, ManifoldBuffer* out, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_pic_deposit");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(pos,0);
    k.set_buffer(vel,1);
    k.set_buffer(mass,2);
    k.set_buffer(heat,3);
    k.set_buffer(out,4);
    k.set_buffer(status,5);
    k.set_bytes(particles,6);
    k.set_bytes(p,7);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_pic_gather(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* mass, ManifoldBuffer* pos_out, ManifoldBuffer* vel_out, ManifoldBuffer* heat_out, ManifoldBuffer* state, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_pic_gather");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(pos,0);
    k.set_buffer(mass,1);
    k.set_buffer(pos_out,2);
    k.set_buffer(vel_out,3);
    k.set_buffer(heat_out,4);
    k.set_buffer(state,5);
    k.set_buffer(status,6);
    k.set_bytes(particles,7);
    k.set_bytes(p,8);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_hydro_export(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* rho, ManifoldBuffer* mom, ManifoldBuffer* thermal, ManifoldBuffer* status, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_hydro_export");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(state,0);
    k.set_buffer(rho,1);
    k.set_buffer(mom,2);
    k.set_buffer(thermal,3);
    k.set_buffer(status,4);
    k.set_bytes(p,5);
    k.dispatch_1d(p.n);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_hydro_budget(ManifoldContext* ctx, ManifoldBuffer* initial, ManifoldBuffer* stage, ManifoldBuffer* ledger, ManifoldBuffer* status, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_hydro_budget");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(initial,0);
    k.set_buffer(stage,1);
    k.set_buffer(ledger,2);
    k.set_buffer(status,3);
    k.set_bytes(p,4);
    k.dispatch_1d(p.n);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_poisson_init(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* field, ManifoldBuffer* status, MFHydroParamsV2 p, float G) {
    KernelDispatch k(ctx,"mc_poisson_init");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(state,0);
    k.set_buffer(field,1);
    k.set_buffer(status,2);
    k.set_bytes(p,3);
    k.set_bytes(G,4);
    k.dispatch_1d(p.n);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_poisson_spectral(ManifoldContext* ctx, ManifoldBuffer* field, ManifoldBuffer* status, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_poisson_spectral");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(field,0);
    k.set_buffer(status,1);
    k.set_bytes(p,2);
    k.dispatch_1d(p.n);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_poisson_export(ManifoldContext* ctx, ManifoldBuffer* field, ManifoldBuffer* potential, ManifoldBuffer* acceleration, ManifoldBuffer* status, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_poisson_export");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(field,0);
    k.set_buffer(potential,1);
    k.set_buffer(acceleration,2);
    k.set_buffer(status,3);
    k.set_bytes(p,4);
    k.dispatch_1d(p.n);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_gravity_kick(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* acceleration, ManifoldBuffer* status, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_gravity_kick");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(state,0);
    k.set_buffer(acceleration,1);
    k.set_buffer(status,2);
    k.set_bytes(p,3);
    k.dispatch_1d(p.n);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_poisson_axis(ManifoldContext* ctx, ManifoldBuffer* field, ManifoldBuffer* work, MFHydroParamsV2 p, unsigned axis, int sign, unsigned workspace) {
    KernelDispatch k(ctx,"mc_poisson_axis");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(field,0);
    k.set_buffer(work,1);
    k.set_bytes(p,2);
    k.set_bytes(axis,3);
    k.set_bytes(sign,4);
    k.set_bytes(workspace,5);
    k.dispatch_groups(p.n/mc_line_length(axis,p),std::min<NSUInteger>(256,k.pso.maxTotalThreadsPerThreadgroup));
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_pic_deposit_material(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* vel, ManifoldBuffer* mass, ManifoldBuffer* heat, ManifoldBuffer* total, ManifoldBuffer* out, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_pic_deposit_material");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(pos,0);
    k.set_buffer(vel,1);
    k.set_buffer(mass,2);
    k.set_buffer(heat,3);
    k.set_buffer(total,4);
    k.set_buffer(out,5);
    k.set_buffer(status,6);
    k.set_bytes(particles,7);
    k.set_bytes(p,8);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_remap_rows(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* pos, ManifoldBuffer* columns, ManifoldBuffer* rows, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float width, float mass_scale) {
    KernelDispatch k(ctx,"mc_remap_rows");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(state,0);
    k.set_buffer(pos,1);
    k.set_buffer(columns,2);
    k.set_buffer(rows,3);
    k.set_buffer(status,4);
    k.set_bytes(particles,5);
    k.set_bytes(p,6);
    k.set_bytes(width,7);
    k.set_bytes(mass_scale,8);
    k.dispatch_1d(p.n);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_remap_columns(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* pos, ManifoldBuffer* mass, ManifoldBuffer* rows, ManifoldBuffer* columns, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float width) {
    KernelDispatch k(ctx,"mc_remap_columns");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(state,0);
    k.set_buffer(pos,1);
    k.set_buffer(mass,2);
    k.set_buffer(rows,3);
    k.set_buffer(columns,4);
    k.set_buffer(status,5);
    k.set_bytes(particles,6);
    k.set_bytes(p,7);
    k.set_bytes(width,8);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_remap_residual(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* pos, ManifoldBuffer* rows, ManifoldBuffer* columns, ManifoldBuffer* residual, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float width, float mass_scale) {
    KernelDispatch k(ctx,"mc_remap_residual");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(state,0);
    k.set_buffer(pos,1);
    k.set_buffer(rows,2);
    k.set_buffer(columns,3);
    k.set_buffer(residual,4);
    k.set_buffer(status,5);
    k.set_bytes(particles,6);
    k.set_bytes(p,7);
    k.set_bytes(width,8);
    k.set_bytes(mass_scale,9);
    k.dispatch_1d(p.n);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_remap_gather(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* pos, ManifoldBuffer* mass, ManifoldBuffer* rows, ManifoldBuffer* velocity, ManifoldBuffer* heat, ManifoldBuffer* total, ManifoldBuffer* mixing, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float width, float mass_scale) {
    KernelDispatch k(ctx,"mc_remap_gather");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(state,0);
    k.set_buffer(pos,1);
    k.set_buffer(mass,2);
    k.set_buffer(rows,3);
    k.set_buffer(velocity,4);
    k.set_buffer(heat,5);
    k.set_buffer(total,6);
    k.set_buffer(mixing,7);
    k.set_buffer(status,8);
    k.set_bytes(particles,9);
    k.set_bytes(p,10);
    k.set_bytes(width,11);
    k.set_bytes(mass_scale,12);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_pilot_checked_step(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* mass, ManifoldBuffer* prior, ManifoldBuffer* re, ManifoldBuffer* im, ManifoldBuffer* position, ManifoldBuffer* guide, ManifoldBuffer* report, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float hbar, float tolerance, float maxcells) {
    KernelDispatch k(ctx,"mc_pilot_checked_step");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(pos,0);
    k.set_buffer(mass,1);
    k.set_buffer(prior,2);
    k.set_buffer(re,3);
    k.set_buffer(im,4);
    k.set_buffer(position,5);
    k.set_buffer(guide,6);
    k.set_buffer(report,7);
    k.set_buffer(status,8);
    k.set_bytes(particles,9);
    k.set_bytes(p,10);
    k.set_bytes(hbar,11);
    k.set_bytes(tolerance,12);
    k.set_bytes(maxcells,13);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_contact_pack(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* vel, ManifoldBuffer* mass, ManifoldBuffer* heat, ManifoldBuffer* state, ManifoldBuffer* cell, ManifoldBuffer* count, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_contact_pack");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(pos,0);
    k.set_buffer(vel,1);
    k.set_buffer(mass,2);
    k.set_buffer(heat,3);
    k.set_buffer(state,4);
    k.set_buffer(cell,5);
    k.set_buffer(count,6);
    k.set_buffer(status,7);
    k.set_bytes(particles,8);
    k.set_bytes(p,9);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_contact_sort(ManifoldContext* ctx, ManifoldBuffer* cell, ManifoldBuffer* starts, ManifoldBuffer* offset, ManifoldBuffer* sorted, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_contact_sort");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(cell,0);
    k.set_buffer(starts,1);
    k.set_buffer(offset,2);
    k.set_buffer(sorted,3);
    k.set_buffer(status,4);
    k.set_bytes(particles,5);
    k.set_bytes(p,6);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_contact_hash_kick(ManifoldContext* ctx, ManifoldBuffer* initial, ManifoldBuffer* stage, ManifoldBuffer* starts, ManifoldBuffer* sorted, ManifoldBuffer* out, ManifoldBuffer* diagnostic, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, MFContactParamsV2 contact, unsigned second, float max_speed) {
    KernelDispatch k(ctx,"mc_contact_hash_kick");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(initial,0);
    k.set_buffer(stage,1);
    k.set_buffer(starts,2);
    k.set_buffer(sorted,3);
    k.set_buffer(out,4);
    k.set_buffer(diagnostic,5);
    k.set_buffer(status,6);
    k.set_bytes(particles,7);
    k.set_bytes(p,8);
    k.set_bytes(contact,9);
    k.set_bytes(second,10);
    k.set_bytes(max_speed,11);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_contact_unpack(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* vel, ManifoldBuffer* heat, unsigned particles, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_contact_unpack");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(state,0);
    k.set_buffer(vel,1);
    k.set_buffer(heat,2);
    k.set_bytes(particles,3);
    k.set_bytes(p,4);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_gravity_compatible(ManifoldContext* ctx, ManifoldBuffer* flux_initial, ManifoldBuffer* flux_stage, ManifoldBuffer* state, ManifoldBuffer* phi0, ManifoldBuffer* phi1, ManifoldBuffer* kick_work, ManifoldBuffer* out, ManifoldBuffer* status, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_gravity_compatible");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(flux_initial,0);
    k.set_buffer(flux_stage,1);
    k.set_buffer(state,2);
    k.set_buffer(phi0,3);
    k.set_buffer(phi1,4);
    k.set_buffer(kick_work,5);
    k.set_buffer(out,6);
    k.set_buffer(status,7);
    k.set_bytes(p,8);
    k.dispatch_1d(p.n);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_gravity_remap(ManifoldContext* ctx, ManifoldBuffer* state, ManifoldBuffer* pos, ManifoldBuffer* mass, ManifoldBuffer* rows, ManifoldBuffer* phi0, ManifoldBuffer* phi1, ManifoldBuffer* energy, ManifoldBuffer* work, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float width) {
    KernelDispatch k(ctx,"mc_gravity_remap");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(state,0);
    k.set_buffer(pos,1);
    k.set_buffer(mass,2);
    k.set_buffer(rows,3);
    k.set_buffer(phi0,4);
    k.set_buffer(phi1,5);
    k.set_buffer(energy,6);
    k.set_buffer(work,7);
    k.set_buffer(status,8);
    k.set_bytes(particles,9);
    k.set_bytes(p,10);
    k.set_bytes(width,11);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_reciprocal_potential(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* amplitude, ManifoldBuffer* omega, ManifoldBuffer* mode_omega, ManifoldBuffer* linewidth, ManifoldBuffer* indices, ManifoldBuffer* weights, ManifoldBuffer* potential, ManifoldBuffer* status, unsigned particles, unsigned modes, unsigned anchors, float sigma, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_reciprocal_potential");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(pos,0);
    k.set_buffer(amplitude,1);
    k.set_buffer(omega,2);
    k.set_buffer(mode_omega,3);
    k.set_buffer(linewidth,4);
    k.set_buffer(indices,5);
    k.set_buffer(weights,6);
    k.set_buffer(potential,7);
    k.set_buffer(status,8);
    k.set_bytes(particles,9);
    k.set_bytes(modes,10);
    k.set_bytes(anchors,11);
    k.set_bytes(sigma,12);
    k.set_bytes(p,13);
    k.dispatch_1d(modes);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_reciprocal_force(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* amplitude, ManifoldBuffer* omega, ManifoldBuffer* mode_omega, ManifoldBuffer* linewidth, ManifoldBuffer* indices, ManifoldBuffer* weights, ManifoldBuffer* occupation, ManifoldBuffer* force, ManifoldBuffer* status, unsigned particles, unsigned modes, unsigned anchors, float sigma, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_reciprocal_force");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(pos,0);
    k.set_buffer(amplitude,1);
    k.set_buffer(omega,2);
    k.set_buffer(mode_omega,3);
    k.set_buffer(linewidth,4);
    k.set_buffer(indices,5);
    k.set_buffer(weights,6);
    k.set_buffer(occupation,7);
    k.set_buffer(force,8);
    k.set_buffer(status,9);
    k.set_bytes(particles,10);
    k.set_bytes(modes,11);
    k.set_bytes(anchors,12);
    k.set_bytes(sigma,13);
    k.set_bytes(p,14);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_reciprocal_occupation(ManifoldContext* ctx, ManifoldBuffer* old_re, ManifoldBuffer* old_im, ManifoldBuffer* ledger, ManifoldBuffer* occupation, ManifoldBuffer* status, unsigned modes, float domega, MFHydroParamsV2 p) {
    KernelDispatch k(ctx,"mc_reciprocal_occupation");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(old_re,0);
    k.set_buffer(old_im,1);
    k.set_buffer(ledger,2);
    k.set_buffer(occupation,3);
    k.set_buffer(status,4);
    k.set_bytes(modes,5);
    k.set_bytes(domega,6);
    k.set_bytes(p,7);
    k.dispatch_1d(modes);
    k.buffer_barrier();return true;
}
static bool mc_launch_mc_pilot_checked_time_step(ManifoldContext* ctx, ManifoldBuffer* pos, ManifoldBuffer* mass, ManifoldBuffer* prior, ManifoldBuffer* re0, ManifoldBuffer* im0, ManifoldBuffer* re1, ManifoldBuffer* im1, ManifoldBuffer* position, ManifoldBuffer* guide, ManifoldBuffer* report, ManifoldBuffer* status, unsigned particles, MFHydroParamsV2 p, float hbar, float tolerance, float maxcells) {
    KernelDispatch k(ctx,"mc_pilot_checked_time_step");if(!k.pso)return false;k.buffer_barrier();
    k.set_buffer(pos,0);
    k.set_buffer(mass,1);
    k.set_buffer(prior,2);
    k.set_buffer(re0,3);
    k.set_buffer(im0,4);
    k.set_buffer(re1,5);
    k.set_buffer(im1,6);
    k.set_buffer(position,7);
    k.set_buffer(guide,8);
    k.set_buffer(report,9);
    k.set_buffer(status,10);
    k.set_bytes(particles,11);
    k.set_bytes(p,12);
    k.set_bytes(hbar,13);
    k.set_bytes(tolerance,14);
    k.set_bytes(maxcells,15);
    k.dispatch_1d(particles);
    k.buffer_barrier();return true;
}
#include "shared/coupled_host.inc"
