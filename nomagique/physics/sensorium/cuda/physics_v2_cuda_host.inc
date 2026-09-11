// Included after the existing CUDA ops definitions.
#include "physics_v2_kernels.cuh"
#include "../shared/physics_v2_core.h"
static bool mf_v2_ready(ManifoldContext* ctx,std::initializer_list<ManifoldBuffer*> bs) {
    return ready(ctx,bs);
}
static bool mf_v2_sync(ManifoldContext* ctx) { return ctx && ctx->synchronize(); }
static bool mf_v2_status_ok(ManifoldContext* ctx,ManifoldBuffer* s,unsigned n) {
    if(!mf_v2_sync(ctx)) return false;
    const auto* words=static_cast<const unsigned*>(s->data);
    for(unsigned i=0;i<n;++i) if(words[i]) return false;
    return true;
}

static bool mf_launch_mf_hydro_pack_v2(ManifoldContext* ctx, ManifoldBuffer* b_rho, ManifoldBuffer* b_mom, ManifoldBuffer* b_thermal, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFHydroParamsV2 p) {
    kernels::mf_hydro_pack_v2<<<blocks(p.n),kBlockSize,0,ctx->stream>>>(
        ptr<const float>(b_rho),
        ptr<const float>(b_mom),
        ptr<const float>(b_thermal),
        ptr<MFHydroStateV2>(b_out),
        ptr<unsigned>(b_status),
        p);
    ctx->launch_status(cudaGetLastError(),"mf_hydro_pack_v2");
    return !ctx->error[0];
}

static bool mf_launch_mf_hydro_unpack_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_rho, ManifoldBuffer* b_mom, ManifoldBuffer* b_thermal, ManifoldBuffer* b_status, MFHydroParamsV2 p) {
    kernels::mf_hydro_unpack_v2<<<blocks(p.n),kBlockSize,0,ctx->stream>>>(
        ptr<const MFHydroStateV2>(b_in),
        ptr<float>(b_rho),
        ptr<float>(b_mom),
        ptr<float>(b_thermal),
        ptr<unsigned>(b_status),
        p);
    ctx->launch_status(cudaGetLastError(),"mf_hydro_unpack_v2");
    return !ctx->error[0];
}

static bool mf_launch_mf_hydro_stage_v2(ManifoldContext* ctx, ManifoldBuffer* b_initial, ManifoldBuffer* b_stage, ManifoldBuffer* b_acceleration, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFHydroParamsV2 p, unsigned second) {
    kernels::mf_hydro_stage_v2<<<blocks(p.n),kBlockSize,0,ctx->stream>>>(
        ptr<const MFHydroStateV2>(b_initial),
        ptr<const MFHydroStateV2>(b_stage),
        ptr<const float>(b_acceleration),
        ptr<MFHydroStateV2>(b_out),
        ptr<unsigned>(b_status),
        p,
        second);
    ctx->launch_status(cudaGetLastError(),"mf_hydro_stage_v2");
    return !ctx->error[0];
}

static bool mf_launch_mf_hydro_diagnostics_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_acceleration, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFHydroParamsV2 p) {
    kernels::mf_hydro_diagnostics_v2<<<blocks(p.n),kBlockSize,0,ctx->stream>>>(
        ptr<const MFHydroStateV2>(b_in),
        ptr<const float>(b_acceleration),
        ptr<MFHydroDiagnosticV2>(b_out),
        ptr<unsigned>(b_status),
        p);
    ctx->launch_status(cudaGetLastError(),"mf_hydro_diagnostics_v2");
    return !ctx->error[0];
}

static bool mf_launch_mf_wave_local_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_out, ManifoldBuffer* b_potential, ManifoldBuffer* b_status, MFWaveParamsV2 p, float dt, unsigned reset) {
    kernels::mf_wave_local_v2<<<blocks(p.n),kBlockSize,0,ctx->stream>>>(
        ptr<const MFWaveValueV2>(b_in),
        ptr<MFWaveValueV2>(b_out),
        ptr<const float>(b_potential),
        ptr<unsigned>(b_status),
        p,
        dt,
        reset);
    ctx->launch_status(cudaGetLastError(),"mf_wave_local_v2");
    return !ctx->error[0];
}

static bool mf_launch_mf_wave_bond_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFWaveParamsV2 p, unsigned axis, unsigned color, float dt) {
    kernels::mf_wave_bond_v2<<<blocks(p.n),kBlockSize,0,ctx->stream>>>(
        ptr<const MFWaveValueV2>(b_in),
        ptr<MFWaveValueV2>(b_out),
        ptr<unsigned>(b_status),
        p,
        axis,
        color,
        dt);
    ctx->launch_status(cudaGetLastError(),"mf_wave_bond_v2");
    return !ctx->error[0];
}

static bool mf_launch_mf_wave_diagnostics_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_potential, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFWaveParamsV2 p) {
    kernels::mf_wave_diagnostics_v2<<<blocks(p.n),kBlockSize,0,ctx->stream>>>(
        ptr<const MFWaveValueV2>(b_in),
        ptr<const float>(b_potential),
        ptr<MFWaveDiagnosticV2>(b_out),
        ptr<unsigned>(b_status),
        p);
    ctx->launch_status(cudaGetLastError(),"mf_wave_diagnostics_v2");
    return !ctx->error[0];
}

static bool mf_launch_mf_contact_stage_v2(ManifoldContext* ctx, ManifoldBuffer* b_initial, ManifoldBuffer* b_stage, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFContactParamsV2 p, unsigned second) {
    kernels::mf_contact_stage_v2<<<blocks(p.n),kBlockSize,0,ctx->stream>>>(
        ptr<const MFParticleStateV2>(b_initial),
        ptr<const MFParticleStateV2>(b_stage),
        ptr<MFParticleStateV2>(b_out),
        ptr<unsigned>(b_status),
        p,
        second);
    ctx->launch_status(cudaGetLastError(),"mf_contact_stage_v2");
    return !ctx->error[0];
}

static bool mf_launch_mf_contact_diagnostics_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_out, ManifoldBuffer* b_status, MFContactParamsV2 p) {
    kernels::mf_contact_diagnostics_v2<<<blocks(p.n),kBlockSize,0,ctx->stream>>>(
        ptr<const MFParticleStateV2>(b_in),
        ptr<MFContactResultV2>(b_out),
        ptr<unsigned>(b_status),
        p);
    ctx->launch_status(cudaGetLastError(),"mf_contact_diagnostics_v2");
    return !ctx->error[0];
}

static bool mf_launch_mf_hydro_copy_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_out, unsigned n) {
    kernels::mf_hydro_copy_v2<<<blocks(n),kBlockSize,0,ctx->stream>>>(
        ptr<const MFHydroStateV2>(b_in),
        ptr<MFHydroStateV2>(b_out),
        n);
    ctx->launch_status(cudaGetLastError(),"mf_hydro_copy_v2");
    return !ctx->error[0];
}

static bool mf_launch_mf_wave_copy_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_out, unsigned n) {
    kernels::mf_wave_copy_v2<<<blocks(n),kBlockSize,0,ctx->stream>>>(
        ptr<const MFWaveValueV2>(b_in),
        ptr<MFWaveValueV2>(b_out),
        n);
    ctx->launch_status(cudaGetLastError(),"mf_wave_copy_v2");
    return !ctx->error[0];
}

static bool mf_launch_mf_contact_copy_v2(ManifoldContext* ctx, ManifoldBuffer* b_in, ManifoldBuffer* b_out, unsigned n) {
    kernels::mf_contact_copy_v2<<<blocks(n),kBlockSize,0,ctx->stream>>>(
        ptr<const MFParticleStateV2>(b_in),
        ptr<MFParticleStateV2>(b_out),
        n);
    ctx->launch_status(cudaGetLastError(),"mf_contact_copy_v2");
    return !ctx->error[0];
}

#include "../shared/physics_v2_host_common.inc"
