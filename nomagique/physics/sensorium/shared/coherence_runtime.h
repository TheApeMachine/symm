#ifndef MANIFOLD_COHERENCE_RUNTIME_H
#define MANIFOLD_COHERENCE_RUNTIME_H
#include <stdint.h>
#include <stddef.h>
#include <string.h>
#define MANIFOLD_COHERENCE_ACCUM_ABI 2
#define MANIFOLD_LOCAL_CARRIER_CAPACITY 256u
/* Quiescent host/readback representation. GPU code uses typed atomic fields.
   Zero the WHOLE record before an accumulation epoch, after previous readers.
   The final eight bytes are one key, NOT two independently meaningful u32s. */
typedef struct ManifoldCarrierAccumulator {
    float force_r, force_i, w_sum, w_omega_sum, w_omega2_sum, w_amp_sum;
    uint64_t packed_offender;
} ManifoldCarrierAccumulator;
typedef struct ManifoldLocalCarrierAccumulator {
    float force_r, force_i, w_sum, w_omega_sum, w_omega2_sum, w_amp_sum;
} ManifoldLocalCarrierAccumulator;
static inline uint32_t manifold_offender_index(uint64_t key) {
    return key ? ~((uint32_t)key) : UINT32_MAX;
}
static inline float manifold_offender_weight(uint64_t key) {
    uint32_t bits=key ? (uint32_t)(key>>32)^0x80000000u : 0u;
    float out; memcpy(&out,&bits,sizeof(out)); return out;
}
#ifdef __cplusplus
static_assert(sizeof(ManifoldCarrierAccumulator)==32, "carrier record size");
static_assert(alignof(ManifoldCarrierAccumulator)==8, "carrier atomic alignment");
static_assert(offsetof(ManifoldCarrierAccumulator,packed_offender)==24, "key offset");
static_assert(sizeof(ManifoldLocalCarrierAccumulator)==24, "local record size");
#endif
#endif
