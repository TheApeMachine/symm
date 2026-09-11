// Shared radix-2/Bluestein implementation: one owner block per periodic lattice.
#include "../shared/radix2_math.h"
#include "../shared/spectral_geometry.h"

#define MR_IMPL __device__ inline
#define MR_MEM
#define MR_BARRIER() __syncthreads()
#define MR_RADIX mr_radix_shared
#define MR_TRANSFORM mr_transform_shared
#include "../shared/radix2_parallel.inc"
#undef MR_IMPL
#undef MR_MEM
#undef MR_BARRIER
#undef MR_RADIX
#undef MR_TRANSFORM

__global__ void coherence_gpe_fft_fused(
    float* mode_real, float* mode_imag, CarrierAccumulators* accums,
    const uint* num_modes_in, CoherenceModeParams p, GPEParams gp,
    float* kinetic_real, float* kinetic_imag, uint scratch_complex, float* ledger, const float* extra_potential, const float* metric_volume, uint geometry_flags) {
    const uint lane=threadIdx.x, lanes=blockDim.x;
    extern __shared__ MRComplex scratch[];
#define MR_GEOMETRY 1
#define MR_EMIT_LEDGER(k,c,d,f) do { ledger[6*(k)]=(c).r; ledger[6*(k)+1]=(c).i; ledger[6*(k)+2]=(d).r; ledger[6*(k)+3]=(d).i; ledger[6*(k)+4]=(f).r; ledger[6*(k)+5]=(f).i; } while(0)
#define MR_ACC_READ(x) atomic_read(&(x))
#define MR_FFT_BARRIER() __syncthreads()
#define MR_FFT_TRANSFORM mr_transform_shared
#include "../shared/coherence_fft_body.inc"
#undef MR_GEOMETRY
#undef MR_EMIT_LEDGER
#undef MR_ACC_READ
#undef MR_FFT_BARRIER
#undef MR_FFT_TRANSFORM
}

#define MR_IMPL __device__ inline
#define MR_MEM
#define MR_BARRIER() __syncthreads()
#define MR_RADIX mr_radix_global
#define MR_TRANSFORM mr_transform_global
#include "../shared/radix2_parallel.inc"
#undef MR_IMPL
#undef MR_MEM
#undef MR_BARRIER
#undef MR_RADIX
#undef MR_TRANSFORM

__global__ void coherence_gpe_fft_fused_global(
    float* mode_real, float* mode_imag, CarrierAccumulators* accums,
    const uint* num_modes_in, CoherenceModeParams p, GPEParams gp,
    float* kinetic_real, float* kinetic_imag, uint scratch_complex, void* global_scratch, float* ledger, const float* extra_potential, const float* metric_volume, uint geometry_flags) {
    const uint lane=threadIdx.x, lanes=blockDim.x;
    MRComplex* scratch=static_cast<MRComplex*>(global_scratch);
#define MR_GEOMETRY 1
#define MR_EMIT_LEDGER(k,c,d,f) do { ledger[6*(k)]=(c).r; ledger[6*(k)+1]=(c).i; ledger[6*(k)+2]=(d).r; ledger[6*(k)+3]=(d).i; ledger[6*(k)+4]=(f).r; ledger[6*(k)+5]=(f).i; } while(0)
#define MR_ACC_READ(x) atomic_read(&(x))
#define MR_FFT_BARRIER() __syncthreads()
#define MR_FFT_TRANSFORM mr_transform_global
#include "../shared/coherence_fft_body.inc"
#undef MR_GEOMETRY
#undef MR_EMIT_LEDGER
#undef MR_ACC_READ
#undef MR_FFT_BARRIER
#undef MR_FFT_TRANSFORM
}
