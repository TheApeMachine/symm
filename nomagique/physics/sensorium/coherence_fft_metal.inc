// Shared radix-2/Bluestein implementation: one owner block per periodic lattice.
#include "shared/radix2_math.h"
#include "shared/spectral_geometry.h"

#define MR_IMPL inline
#define MR_MEM threadgroup
#define MR_BARRIER() threadgroup_barrier(mem_flags::mem_threadgroup)
#define MR_RADIX mr_radix_shared
#define MR_TRANSFORM mr_transform_shared
#include "shared/radix2_parallel.inc"
#undef MR_IMPL
#undef MR_MEM
#undef MR_BARRIER
#undef MR_RADIX
#undef MR_TRANSFORM

kernel void coherence_gpe_fft_fused(
    device float* mode_real [[buffer(0)]],
    device float* mode_imag [[buffer(1)]],
    device CarrierAccumulators* accums [[buffer(2)]],
    device const uint* num_modes_in [[buffer(3)]],
    constant CoherenceModeParams& p [[buffer(4)]],
    constant GPEParams& gp [[buffer(5)]],
    device float* kinetic_real [[buffer(6)]],
    device float* kinetic_imag [[buffer(7)]],
    constant uint& scratch_complex [[buffer(8)]],
    threadgroup MRComplex* scratch [[threadgroup(0)]],
    device float* ledger [[buffer(10)]],
    device const float* extra_potential [[buffer(11)]],
    device const float* metric_volume [[buffer(12)]],
    constant uint& geometry_flags [[buffer(13)]],
    uint lane [[thread_index_in_threadgroup]], uint lanes [[threads_per_threadgroup]]) {
#define MR_GEOMETRY 1
#define MR_EMIT_LEDGER(k,c,d,f) do { ledger[6*(k)]=(c).r; ledger[6*(k)+1]=(c).i; ledger[6*(k)+2]=(d).r; ledger[6*(k)+3]=(d).i; ledger[6*(k)+4]=(f).r; ledger[6*(k)+5]=(f).i; } while(0)
#define MR_ACC_READ(x) atomic_load_explicit(&(x),memory_order_relaxed)
#define MR_FFT_BARRIER() threadgroup_barrier(mem_flags::mem_threadgroup)
#define MR_FFT_TRANSFORM mr_transform_shared
#include "shared/coherence_fft_body.inc"
#undef MR_GEOMETRY
#undef MR_EMIT_LEDGER
#undef MR_ACC_READ
#undef MR_FFT_BARRIER
#undef MR_FFT_TRANSFORM
}

#define MR_IMPL inline
#define MR_MEM device
#define MR_BARRIER() threadgroup_barrier(mem_flags::mem_device)
#define MR_RADIX mr_radix_global
#define MR_TRANSFORM mr_transform_global
#include "shared/radix2_parallel.inc"
#undef MR_IMPL
#undef MR_MEM
#undef MR_BARRIER
#undef MR_RADIX
#undef MR_TRANSFORM

kernel void coherence_gpe_fft_fused_global(
    device float* mode_real [[buffer(0)]],
    device float* mode_imag [[buffer(1)]],
    device CarrierAccumulators* accums [[buffer(2)]],
    device const uint* num_modes_in [[buffer(3)]],
    constant CoherenceModeParams& p [[buffer(4)]],
    constant GPEParams& gp [[buffer(5)]],
    device float* kinetic_real [[buffer(6)]],
    device float* kinetic_imag [[buffer(7)]],
    constant uint& scratch_complex [[buffer(8)]],
    device MRComplex* scratch [[buffer(9)]],
    device float* ledger [[buffer(10)]],
    device const float* extra_potential [[buffer(11)]],
    device const float* metric_volume [[buffer(12)]],
    constant uint& geometry_flags [[buffer(13)]],
    uint lane [[thread_index_in_threadgroup]], uint lanes [[threads_per_threadgroup]]) {
#define MR_GEOMETRY 1
#define MR_EMIT_LEDGER(k,c,d,f) do { ledger[6*(k)]=(c).r; ledger[6*(k)+1]=(c).i; ledger[6*(k)+2]=(d).r; ledger[6*(k)+3]=(d).i; ledger[6*(k)+4]=(f).r; ledger[6*(k)+5]=(f).i; } while(0)
#define MR_ACC_READ(x) atomic_load_explicit(&(x),memory_order_relaxed)
#define MR_FFT_BARRIER() threadgroup_barrier(mem_flags::mem_device)
#define MR_FFT_TRANSFORM mr_transform_global
#include "shared/coherence_fft_body.inc"
#undef MR_GEOMETRY
#undef MR_EMIT_LEDGER
#undef MR_ACC_READ
#undef MR_FFT_BARRIER
#undef MR_FFT_TRANSFORM
}
