// Included by host.cxx AFTER ops.mm, reusing its real context/dispatch types.
#include <initializer_list>
#include "shared/physics_v2_core.h"
static bool mf_v2_ready(ManifoldContext* ctx,std::initializer_list<ManifoldBuffer*> bs) {
    if(!ctx || !ctx->error.empty() || !ctx->device || !ctx->command_queue || !ctx->library) return false;
    for(auto b:bs) if(!b || !b->mtl_buffer || [b->mtl_buffer device]!=ctx->device ||
                    ![b->mtl_buffer contents] || b->size_bytes/4>UINT32_MAX) return false;
    return true;
}
static bool mf_v2_sync(ManifoldContext* ctx) {
    if(!ctx || !ctx->error.empty()) return false;
    id<MTLCommandBuffer> command=ctx->current_command_buffer;
    ctx->commit_and_wait();
    if(command && [command status]==MTLCommandBufferStatusError) {
        NSLog(@"Manifold physics v2 GPU failure: %@",[command error]); return false;
    }
    return ctx->error.empty();
}
static bool mf_v2_status_ok(ManifoldContext* ctx,ManifoldBuffer* s,unsigned n) {
    if(!mf_v2_sync(ctx)) return false;
    const auto* words=static_cast<const unsigned*>([s->mtl_buffer contents]);
    for(unsigned i=0;i<n;++i) if(words[i]) return false;
    return true;
}

static bool mf_launch_mf_hydro_pack_v2(ManifoldContext* ctx, ManifoldBuffer* b_rho, ManifoldBuffer* b_mom, ManifoldBuffer* b_thermal, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFHydroParamsV2 p) {
    if(!ctx->get_pipeline("mf_hydro_pack_v2")) return false;
    KernelDispatch k(ctx,"mf_hydro_pack_v2");

    k.set_buffer(b_rho,0);

    k.set_buffer(b_mom,1);

    k.set_buffer(b_thermal,2);

    k.set_buffer(b_out,3);

    k.set_buffer(b_status,4);

    k.set_bytes(p,5);

    k.dispatch_1d(p.n);
    k.buffer_barrier();
    return true;
}

static bool mf_launch_mf_hydro_unpack_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_rho, ManifoldBuffer* b_mom, ManifoldBuffer* b_thermal, ManifoldBuffer* b_status, MFHydroParamsV2 p) {
    if(!ctx->get_pipeline("mf_hydro_unpack_v2")) return false;
    KernelDispatch k(ctx,"mf_hydro_unpack_v2");

    k.set_buffer(b_in,0);

    k.set_buffer(b_rho,1);

    k.set_buffer(b_mom,2);

    k.set_buffer(b_thermal,3);

    k.set_buffer(b_status,4);

    k.set_bytes(p,5);

    k.dispatch_1d(p.n);
    k.buffer_barrier();
    return true;
}

static bool mf_launch_mf_hydro_stage_v2(ManifoldContext* ctx, ManifoldBuffer* b_initial, ManifoldBuffer* b_stage, ManifoldBuffer* b_acceleration, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFHydroParamsV2 p, unsigned second) {
    if(!ctx->get_pipeline("mf_hydro_stage_v2")) return false;
    KernelDispatch k(ctx,"mf_hydro_stage_v2");

    k.set_buffer(b_initial,0);

    k.set_buffer(b_stage,1);

    k.set_buffer(b_acceleration,2);

    k.set_buffer(b_out,3);

    k.set_buffer(b_status,4);

    k.set_bytes(p,5);

    k.set_bytes(second,6);

    k.dispatch_1d(p.n);
    k.buffer_barrier();
    return true;
}

static bool mf_launch_mf_hydro_diagnostics_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_acceleration, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFHydroParamsV2 p) {
    if(!ctx->get_pipeline("mf_hydro_diagnostics_v2")) return false;
    KernelDispatch k(ctx,"mf_hydro_diagnostics_v2");

    k.set_buffer(b_in,0);

    k.set_buffer(b_acceleration,1);

    k.set_buffer(b_out,2);

    k.set_buffer(b_status,3);

    k.set_bytes(p,4);

    k.dispatch_1d(p.n);
    k.buffer_barrier();
    return true;
}

static bool mf_launch_mf_wave_local_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_out, ManifoldBuffer* b_potential, ManifoldBuffer* b_status, MFWaveParamsV2 p, float dt, unsigned reset) {
    if(!ctx->get_pipeline("mf_wave_local_v2")) return false;
    KernelDispatch k(ctx,"mf_wave_local_v2");

    k.set_buffer(b_in,0);

    k.set_buffer(b_out,1);

    k.set_buffer(b_potential,2);

    k.set_buffer(b_status,3);

    k.set_bytes(p,4);

    k.set_bytes(dt,5);

    k.set_bytes(reset,6);

    k.dispatch_1d(p.n);
    k.buffer_barrier();
    return true;
}

static bool mf_launch_mf_wave_bond_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFWaveParamsV2 p, unsigned axis, unsigned color, float dt) {
    if(!ctx->get_pipeline("mf_wave_bond_v2")) return false;
    KernelDispatch k(ctx,"mf_wave_bond_v2");

    k.set_buffer(b_in,0);

    k.set_buffer(b_out,1);

    k.set_buffer(b_status,2);

    k.set_bytes(p,3);

    k.set_bytes(axis,4);

    k.set_bytes(color,5);

    k.set_bytes(dt,6);

    k.dispatch_1d(p.n);
    k.buffer_barrier();
    return true;
}

static bool mf_launch_mf_wave_diagnostics_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_potential, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFWaveParamsV2 p) {
    if(!ctx->get_pipeline("mf_wave_diagnostics_v2")) return false;
    KernelDispatch k(ctx,"mf_wave_diagnostics_v2");

    k.set_buffer(b_in,0);

    k.set_buffer(b_potential,1);

    k.set_buffer(b_out,2);

    k.set_buffer(b_status,3);

    k.set_bytes(p,4);

    k.dispatch_1d(p.n);
    k.buffer_barrier();
    return true;
}

static bool mf_launch_mf_contact_stage_v2(ManifoldContext* ctx, ManifoldBuffer* b_initial, ManifoldBuffer* b_stage, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFContactParamsV2 p, unsigned second) {
    if(!ctx->get_pipeline("mf_contact_stage_v2")) return false;
    KernelDispatch k(ctx,"mf_contact_stage_v2");

    k.set_buffer(b_initial,0);

    k.set_buffer(b_stage,1);

    k.set_buffer(b_out,2);

    k.set_buffer(b_status,3);

    k.set_bytes(p,4);

    k.set_bytes(second,5);

    k.dispatch_1d(p.n);
    k.buffer_barrier();
    return true;
}

static bool mf_launch_mf_contact_diagnostics_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFContactParamsV2 p) {
    if(!ctx->get_pipeline("mf_contact_diagnostics_v2")) return false;
    KernelDispatch k(ctx,"mf_contact_diagnostics_v2");

    k.set_buffer(b_in,0);

    k.set_buffer(b_out,1);

    k.set_buffer(b_status,2);

    k.set_bytes(p,3);

    k.dispatch_1d(p.n);
    k.buffer_barrier();
    return true;
}

static bool mf_launch_mf_hydro_copy_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_out, unsigned n) {
    if(!ctx->get_pipeline("mf_hydro_copy_v2")) return false;
    KernelDispatch k(ctx,"mf_hydro_copy_v2");

    k.set_buffer(b_in,0);

    k.set_buffer(b_out,1);

    k.set_bytes(n,2);

    k.dispatch_1d(n);
    k.buffer_barrier();
    return true;
}

static bool mf_launch_mf_wave_copy_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_out, unsigned n) {
    if(!ctx->get_pipeline("mf_wave_copy_v2")) return false;
    KernelDispatch k(ctx,"mf_wave_copy_v2");

    k.set_buffer(b_in,0);

    k.set_buffer(b_out,1);

    k.set_bytes(n,2);

    k.dispatch_1d(n);
    k.buffer_barrier();
    return true;
}

static bool mf_launch_mf_contact_copy_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_out, unsigned n) {
    if(!ctx->get_pipeline("mf_contact_copy_v2")) return false;
    KernelDispatch k(ctx,"mf_contact_copy_v2");

    k.set_buffer(b_in,0);

    k.set_buffer(b_out,1);

    k.set_bytes(n,2);

    k.dispatch_1d(n);
    k.buffer_barrier();
    return true;
}

#include "shared/physics_v2_host_common.inc"
