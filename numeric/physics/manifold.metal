#include <metal_stdlib>
using namespace metal;
 
constant uint kGridX [[function_constant(0)]];
constant uint kGridY [[function_constant(1)]];
constant uint kGridZ [[function_constant(2)]];

// =============================================================================
// Manifold Physics Kernels
// =============================================================================
// Implements the core physics simulation for the thermo-manifold:
// - Particle-to-field scatter (gravity, heat)
// - Field-to-particle gather + integrated state update
// - Carrier-oscillator coupling
//
// Design principles:
// - Fused operations to minimize memory bandwidth
// - Hardware-accelerated trilinear interpolation via texture3d
// - All physics in one gather-update pass
// =============================================================================

// -----------------------------------------------------------------------------
// Utility: Quiet NaN (fail-loudly sentinel)
// -----------------------------------------------------------------------------
// Metal does not guarantee `nanf()` is available; use a quiet-NaN bit pattern.
inline float qnan_f() {
    return as_type<float>(0x7FC00000u);
}

// -----------------------------------------------------------------------------
// GPU "log book" (debug event buffer)
// -----------------------------------------------------------------------------
// Metal kernels cannot print. Instead, we append compact debug events into a
// device buffer and decode them on the host each step.
//
// Layout (u32 words), per-event:
//   [0]=tag, [1]=gid, [2]=a_bits, [3]=b_bits, [4]=c_bits, [5]=d_bits
//
// IMPORTANT:
// - This is for debugging/instrumentation only; it must not change physics.
// - When dbg_cap==0, logging is a no-op (zero overhead except the branch).
#define DBG_WORDS_PER_EVENT 6u
inline void dbg_log(
    device atomic_uint* dbg_head,
    device uint* dbg_words,
    uint dbg_cap,
    uint tag,
    uint gid,
    float a,
    float b,
    float c,
    float d
) {
    if (dbg_cap == 0u) return;
    uint idx = atomic_fetch_add_explicit(dbg_head, 1u, memory_order_relaxed);
    if (idx >= dbg_cap) return;
    uint base = idx * DBG_WORDS_PER_EVENT;
    dbg_words[base + 0u] = tag;
    dbg_words[base + 1u] = gid;
    dbg_words[base + 2u] = as_type<uint>(a);
    dbg_words[base + 3u] = as_type<uint>(b);
    dbg_words[base + 4u] = as_type<uint>(c);
    dbg_words[base + 5u] = as_type<uint>(d);
}

// -----------------------------------------------------------------------------
// Parameter structs
// -----------------------------------------------------------------------------
// All active parameter structs live near the kernels that use them (e.g.
// `SortScatterParams`, `PicGatherParams`, etc.) to avoid stale bindings.

// =============================================================================
// Adaptive Thermodynamics: Fast reduction for global energy statistics
// =============================================================================
// We use a 2-pass reduction to compute:
//   mean_abs = mean(|x|), mean = mean(x), std = std(x)
// entirely on-GPU, so downstream kernels can do adaptive renormalization without
// CPU sync or "magic number" damping.
//
// Output format (single float4 in `out_stats`):
//   x: mean_abs
//   y: mean
//   z: std
//   w: count (as float)
//
// NOTE: Host must dispatch pass1 with exactly 256 threads/threadgroup and
//       num_threadgroups = ceil(N / 256).
// -----------------------------------------------------------------------------

// NOTE: Program-scope variables must reside in the constant address space in Metal.
constant uint kReduceThreads = 256;

kernel void reduce_float_stats_pass1(
    device const float* x           [[buffer(0)]],  // (N,)
    device float* group_stats       [[buffer(1)]],  // (num_groups * 4,) [sum_abs, sum, sum_sq, count]
    constant uint& N                [[buffer(2)]],
    uint tid                        [[thread_index_in_threadgroup]],
    uint tg_id                      [[threadgroup_position_in_grid]]
) {
    uint idx = tg_id * kReduceThreads + tid;
    float v = (idx < N) ? x[idx] : 0.0f;
    float4 acc = float4(fabs(v), v, v * v, (idx < N) ? 1.0f : 0.0f);

    threadgroup float4 scratch[kReduceThreads];
    scratch[tid] = acc;
    threadgroup_barrier(mem_flags::mem_threadgroup);

    for (uint offset = kReduceThreads / 2; offset > 32; offset >>= 1) {
        if (tid < offset) {
            scratch[tid] += scratch[tid + offset];
        }
        threadgroup_barrier(mem_flags::mem_threadgroup);
    }

    float4 lane_acc = scratch[tid];
    lane_acc += simd_shuffle_down(lane_acc, 16);
    lane_acc += simd_shuffle_down(lane_acc, 8);
    lane_acc += simd_shuffle_down(lane_acc, 4);
    lane_acc += simd_shuffle_down(lane_acc, 2);
    lane_acc += simd_shuffle_down(lane_acc, 1);
    if ((tid & 31u) == 0u) {
        scratch[tid] = lane_acc;
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);

    if (tid < 8u) {
        scratch[tid] += scratch[tid + 32u];
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);

    if (tid == 0) {
        group_stats[tg_id * 4 + 0] = scratch[0].x;
        group_stats[tg_id * 4 + 1] = scratch[0].y;
        group_stats[tg_id * 4 + 2] = scratch[0].z;
        group_stats[tg_id * 4 + 3] = scratch[0].w;
    }
}

kernel void reduce_float_stats_finalize(
    device const float* group_stats [[buffer(0)]],  // (num_groups * 4,)
    device float* out_stats         [[buffer(1)]],  // (4,) [mean_abs, mean, std, count]
    constant uint& num_groups        [[buffer(2)]],
    uint tid                         [[thread_index_in_threadgroup]]
) {
    // One threadgroup (kReduceThreads) reduces all group_stats.
    float4 acc = float4(0.0f);
    for (uint i = tid; i < num_groups; i += kReduceThreads) {
        float sum_abs = group_stats[i * 4 + 0];
        float sum = group_stats[i * 4 + 1];
        float sum_sq = group_stats[i * 4 + 2];
        float count = group_stats[i * 4 + 3];
        acc += float4(sum_abs, sum, sum_sq, count);
    }

    threadgroup float4 scratch[kReduceThreads];
    scratch[tid] = acc;
    threadgroup_barrier(mem_flags::mem_threadgroup);

    for (uint offset = kReduceThreads / 2; offset > 32; offset >>= 1) {
        if (tid < offset) {
            scratch[tid] += scratch[tid + offset];
        }
        threadgroup_barrier(mem_flags::mem_threadgroup);
    }

    float4 lane_acc = scratch[tid];
    lane_acc += simd_shuffle_down(lane_acc, 16);
    lane_acc += simd_shuffle_down(lane_acc, 8);
    lane_acc += simd_shuffle_down(lane_acc, 4);
    lane_acc += simd_shuffle_down(lane_acc, 2);
    lane_acc += simd_shuffle_down(lane_acc, 1);
    if ((tid & 31u) == 0u) {
        scratch[tid] = lane_acc;
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);

    if (tid < 8u) {
        scratch[tid] += scratch[tid + 32u];
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);

    if (tid == 0) {
        float sum_abs = scratch[0].x;
        float sum = scratch[0].y;
        float sum_sq = scratch[0].z;
        float count = scratch[0].w;

        // [CHOICE] reduction empty-case semantics
        // [FORMULA] if count<=0: mean_abs=mean=std=0, count=0
        // [REASON] removes numerical clamp; makes empty reduction explicit
        if (!(count > 0.0f)) {
            out_stats[0] = 0.0f;
            out_stats[1] = 0.0f;
            out_stats[2] = 0.0f;
            out_stats[3] = 0.0f;
            return;
        }

        float mean_abs = sum_abs / count;
        float mean = sum / count;
        // [CHOICE] non-negative variance
        // [FORMULA] var = max(E[x^2] - E[x]^2, 0)
        // [REASON] rounding can produce tiny negative; project back to ℝ_{\ge 0}
        float var = (sum_sq / count) - mean * mean;
        float std = sqrt(var);

        out_stats[0] = mean_abs;
        out_stats[1] = mean;
        out_stats[2] = std;
        out_stats[3] = count;
    }
}

// =============================================================================
// Stochastic helpers (hash + Box-Muller for N(0,1))
// =============================================================================
inline uint hash_u32(uint x) {
    // PCG-inspired mix (fast, decent avalanche)
    x ^= x >> 16;
    x *= 0x7feb352du;
    x ^= x >> 15;
    x *= 0x846ca68bu;
    x ^= x >> 16;
    return x;
}

inline float u01_from_u32(uint x) {
    // [CHOICE] uniform random in (0, 1] without eps-clamps
    // [FORMULA] u = (m + 1) / 2^24, where m ∈ [0, 2^24-1]
    // [REASON] avoids u=0 exactly (Box-Muller needs log(u))
    // [NOTES] allows u=1, which yields r=0 in Box-Muller (benign).
    uint m = (x & 0x00FFFFFFu);
    return (float)(m + 1u) * (1.0f / 16777216.0f);
}

inline float2 box_muller(float u1, float u2) {
    float r = sqrt(-2.0f * log(u1));
    float t = 2.0f * M_PI_F * u2;
    return float2(r * cos(t), r * sin(t));
}

inline float3 randn3(uint seed, uint idx) {
    // Deterministic per-(seed, idx) 3D standard normal.
    // Uses 6 uniforms derived from a hashed stream.
    uint s0 = hash_u32(seed ^ (idx * 0x9e3779b9u));
    uint s1 = hash_u32(s0 + 1u);
    uint s2 = hash_u32(s0 + 2u);
    uint s3 = hash_u32(s0 + 3u);
    float2 z0 = box_muller(u01_from_u32(s0), u01_from_u32(s1));
    float2 z1 = box_muller(u01_from_u32(s2), u01_from_u32(s3));
    // We only need 3 independent N(0,1) samples here.
    return float3(z0.x, z0.y, z1.x);
}

inline float2 randn2(uint seed, uint idx) {
    uint s0 = hash_u32(seed ^ (idx * 0x9e3779b9u));
    uint s1 = hash_u32(s0 + 1u);
    return box_muller(u01_from_u32(s0), u01_from_u32(s1));
}

inline float randn1(uint seed, uint idx) {
    return randn2(seed, idx).x;
}

// =============================================================================
// Spatial Hash Grid Structures (for O(N) collision detection)
// =============================================================================
// The spatial hash divides the simulation domain into cells. Each particle is
// assigned to a cell based on its position. Collision detection only checks
// particles in the same cell and 26 neighboring cells (3x3x3 neighborhood).
//
// Cell size should be >= 2 * particle_radius for correctness.
// For optimal performance, cell_size ≈ 2-4 * particle_radius.

struct SpatialHashParams {
    uint32_t num_particles;
    uint32_t grid_x;         // Number of cells in X
    uint32_t grid_y;         // Number of cells in Y
    uint32_t grid_z;         // Number of cells in Z
    float cell_size;         // Size of each cell
    float inv_cell_size;     // 1.0 / cell_size
    float domain_min_x;      // Domain minimum X
    float domain_min_y;      // Domain minimum Y
    float domain_min_z;      // Domain minimum Z
};

struct SpatialCollisionParams {
    uint32_t num_particles;
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float cell_size;
    float inv_cell_size;
    float domain_min_x;
    float domain_min_y;
    float domain_min_z;
    float dt;
    float particle_radius;
    float young_modulus;
    float thermal_conductivity;
    float specific_heat;
    float restitution;
};

// -----------------------------------------------------------------------------
// Utility: Trilinear Interpolation
// -----------------------------------------------------------------------------

// Compute trilinear weights and grid indices for a position
inline void trilinear_coords(
    float3 pos,
    float inv_spacing,
    uint3 grid_dims,
    thread uint3& base_idx,
    thread float3& frac
) {
    // [CHOICE] periodic grid coordinate mapping
    // [FORMULA] g = (pos / Δx) mod grid_dims
    // [REASON] torus domain: positions and fields are periodic
    // [NOTES] This avoids non-physical boundary clamping artifacts.
    float3 g = pos * inv_spacing;
    float3 gd = float3(grid_dims);
    // Wrap into [0, grid_dims)
    g = g - gd * floor(g / gd);

    base_idx = uint3(floor(g));        // 0..dims-1
    frac = g - float3(base_idx);       // [0,1)
}

// Sample a 3D field with trilinear interpolation
inline float sample_field_trilinear(
    device const float* field,
    uint3 base_idx,
    float3 frac,
    uint3 grid_dims
) {
    // Compute strides
    uint stride_z = 1;
    uint stride_y = grid_dims.z;
    uint stride_x = grid_dims.y * grid_dims.z;

    // [CHOICE] periodic corner sampling
    // [FORMULA] (x1,y1,z1) = (x0+1,y0+1,z0+1) mod dims
    // [REASON] torus domain
    uint x0 = base_idx.x;
    uint y0 = base_idx.y;
    uint z0 = base_idx.z;
    uint x1 = (x0 + 1) % grid_dims.x;
    uint y1 = (y0 + 1) % grid_dims.y;
    uint z1 = (z0 + 1) % grid_dims.z;

    float c000 = field[x0 * stride_x + y0 * stride_y + z0 * stride_z];
    float c001 = field[x0 * stride_x + y0 * stride_y + z1 * stride_z];
    float c010 = field[x0 * stride_x + y1 * stride_y + z0 * stride_z];
    float c011 = field[x0 * stride_x + y1 * stride_y + z1 * stride_z];
    float c100 = field[x1 * stride_x + y0 * stride_y + z0 * stride_z];
    float c101 = field[x1 * stride_x + y0 * stride_y + z1 * stride_z];
    float c110 = field[x1 * stride_x + y1 * stride_y + z0 * stride_z];
    float c111 = field[x1 * stride_x + y1 * stride_y + z1 * stride_z];
    
    // Trilinear interpolation
    float fx = frac.x;
    float fy = frac.y;
    float fz = frac.z;
    
    float c00 = c000 * (1.0f - fz) + c001 * fz;
    float c01 = c010 * (1.0f - fz) + c011 * fz;
    float c10 = c100 * (1.0f - fz) + c101 * fz;
    float c11 = c110 * (1.0f - fz) + c111 * fz;
    
    float c0 = c00 * (1.0f - fy) + c01 * fy;
    float c1 = c10 * (1.0f - fy) + c11 * fy;
    
    return c0 * (1.0f - fx) + c1 * fx;
}

// Compute gradient of a 3D field at a position (central differences)
inline float3 sample_gradient_trilinear(
    device const float* field,
    uint3 base_idx,
    float3 frac,
    uint3 grid_dims,
    float inv_spacing
) {
    uint stride_z = 1;
    uint stride_y = grid_dims.z;
    uint stride_x = grid_dims.y * grid_dims.z;
    
    // Sample at offset positions for gradient
    // We approximate gradient using the interpolated values at slightly offset positions
    // For efficiency, we use the corner values to estimate gradient
    
    // Periodic corner sampling (same as sample_field_trilinear)
    uint x0 = base_idx.x;
    uint y0 = base_idx.y;
    uint z0 = base_idx.z;
    uint x1 = (x0 + 1) % grid_dims.x;
    uint y1 = (y0 + 1) % grid_dims.y;
    uint z1 = (z0 + 1) % grid_dims.z;

    float c000 = field[x0 * stride_x + y0 * stride_y + z0 * stride_z];
    float c001 = field[x0 * stride_x + y0 * stride_y + z1 * stride_z];
    float c010 = field[x0 * stride_x + y1 * stride_y + z0 * stride_z];
    float c011 = field[x0 * stride_x + y1 * stride_y + z1 * stride_z];
    float c100 = field[x1 * stride_x + y0 * stride_y + z0 * stride_z];
    float c101 = field[x1 * stride_x + y0 * stride_y + z1 * stride_z];
    float c110 = field[x1 * stride_x + y1 * stride_y + z0 * stride_z];
    float c111 = field[x1 * stride_x + y1 * stride_y + z1 * stride_z];
    
    // Gradient in each direction (using trilinear interpolation of face values)
    float fy = frac.y;
    float fz = frac.z;
    
    // dF/dx: difference between x=1 and x=0 faces
    float face_x0 = c000 * (1-fy) * (1-fz) + c010 * fy * (1-fz) + c001 * (1-fy) * fz + c011 * fy * fz;
    float face_x1 = c100 * (1-fy) * (1-fz) + c110 * fy * (1-fz) + c101 * (1-fy) * fz + c111 * fy * fz;
    float grad_x = (face_x1 - face_x0) * inv_spacing;
    
    float fx = frac.x;
    // dF/dy
    float face_y0 = c000 * (1-fx) * (1-fz) + c100 * fx * (1-fz) + c001 * (1-fx) * fz + c101 * fx * fz;
    float face_y1 = c010 * (1-fx) * (1-fz) + c110 * fx * (1-fz) + c011 * (1-fx) * fz + c111 * fx * fz;
    float grad_y = (face_y1 - face_y0) * inv_spacing;
    
    // dF/dz
    float face_z0 = c000 * (1-fx) * (1-fy) + c100 * fx * (1-fy) + c010 * (1-fx) * fy + c110 * fx * fy;
    float face_z1 = c001 * (1-fx) * (1-fy) + c101 * fx * (1-fy) + c011 * (1-fx) * fy + c111 * fx * fy;
    float grad_z = (face_z1 - face_z0) * inv_spacing;
    
    return float3(grad_x, grad_y, grad_z);
}

inline float4 sample_value_and_gradient_trilinear(
    device const float* field,
    uint3 base_idx,
    float3 frac,
    uint3 grid_dims,
    float inv_spacing
) {
    uint stride_z = 1;
    uint stride_y = grid_dims.z;
    uint stride_x = grid_dims.y * grid_dims.z;

    uint x0 = base_idx.x;
    uint y0 = base_idx.y;
    uint z0 = base_idx.z;
    uint x1 = (x0 + 1) % grid_dims.x;
    uint y1 = (y0 + 1) % grid_dims.y;
    uint z1 = (z0 + 1) % grid_dims.z;

    float c000 = field[x0 * stride_x + y0 * stride_y + z0 * stride_z];
    float c001 = field[x0 * stride_x + y0 * stride_y + z1 * stride_z];
    float c010 = field[x0 * stride_x + y1 * stride_y + z0 * stride_z];
    float c011 = field[x0 * stride_x + y1 * stride_y + z1 * stride_z];
    float c100 = field[x1 * stride_x + y0 * stride_y + z0 * stride_z];
    float c101 = field[x1 * stride_x + y0 * stride_y + z1 * stride_z];
    float c110 = field[x1 * stride_x + y1 * stride_y + z0 * stride_z];
    float c111 = field[x1 * stride_x + y1 * stride_y + z1 * stride_z];

    float fx = frac.x;
    float fy = frac.y;
    float fz = frac.z;

    float c00 = c000 * (1.0f - fz) + c001 * fz;
    float c01 = c010 * (1.0f - fz) + c011 * fz;
    float c10 = c100 * (1.0f - fz) + c101 * fz;
    float c11 = c110 * (1.0f - fz) + c111 * fz;
    float c0 = c00 * (1.0f - fy) + c01 * fy;
    float c1 = c10 * (1.0f - fy) + c11 * fy;
    float value = c0 * (1.0f - fx) + c1 * fx;

    float face_x0 = c000 * (1.0f - fy) * (1.0f - fz) + c010 * fy * (1.0f - fz) + c001 * (1.0f - fy) * fz + c011 * fy * fz;
    float face_x1 = c100 * (1.0f - fy) * (1.0f - fz) + c110 * fy * (1.0f - fz) + c101 * (1.0f - fy) * fz + c111 * fy * fz;
    float face_y0 = c000 * (1.0f - fx) * (1.0f - fz) + c100 * fx * (1.0f - fz) + c001 * (1.0f - fx) * fz + c101 * fx * fz;
    float face_y1 = c010 * (1.0f - fx) * (1.0f - fz) + c110 * fx * (1.0f - fz) + c011 * (1.0f - fx) * fz + c111 * fx * fz;
    float face_z0 = c000 * (1.0f - fx) * (1.0f - fy) + c100 * fx * (1.0f - fy) + c010 * (1.0f - fx) * fy + c110 * fx * fy;
    float face_z1 = c001 * (1.0f - fx) * (1.0f - fy) + c101 * fx * (1.0f - fy) + c011 * (1.0f - fx) * fy + c111 * fx * fy;

    return float4(
        (face_x1 - face_x0) * inv_spacing,
        (face_y1 - face_y0) * inv_spacing,
        (face_z1 - face_z0) * inv_spacing,
        value
    );
}

// -----------------------------------------------------------------------------
// Helper wrappers for coordinate transformation
// -----------------------------------------------------------------------------

inline int wrap_i32(int v, int dim) {
    int r = v % dim;
    return (r < 0) ? r + dim : r;
}

inline float sample_trilinear(
    device const float* field,
    float3 pos,
    uint gx, uint gy, uint gz,
    float spacing,
    float inv_spacing
) {
    uint3 base_idx; float3 frac;
    trilinear_coords(pos, inv_spacing, uint3(gx, gy, gz), base_idx, frac);
    return sample_field_trilinear(field, base_idx, frac, uint3(gx, gy, gz));
}

inline float3 sample_gradient_trilinear(
    device const float* field,
    float3 pos,
    uint gx, uint gy, uint gz,
    float spacing,
    float inv_spacing
) {
    uint3 base_idx; float3 frac;
    trilinear_coords(pos, inv_spacing, uint3(gx, gy, gz), base_idx, frac);
    return sample_gradient_trilinear(field, base_idx, frac, uint3(gx, gy, gz), inv_spacing);
}

// -----------------------------------------------------------------------------
// Spatial model: compressible ideal gas (Navier–Stokes) + PIC + host-side FFT gravity.
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// Kernel: Clear field (set to zero)
// -----------------------------------------------------------------------------

kernel void clear_field(
    device float* field [[buffer(0)]],
    constant uint& num_elements [[buffer(1)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= num_elements) return;
    field[gid] = 0.0f;
}

kernel void clear_buffer_u32(
    device uint* data                [[buffer(0)]],
    constant uint& num_elements      [[buffer(1)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= num_elements) return;
    data[gid] = 0u;
}

kernel void init_omega_scan_keys(
    device atomic_uint* omega_min_key [[buffer(0)]],
    device atomic_uint* omega_max_key [[buffer(1)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid != 0u) return;
    atomic_store_explicit(omega_min_key, 0xFFFFFFFFu, memory_order_relaxed);
    atomic_store_explicit(omega_max_key, 0u, memory_order_relaxed);
}

kernel void copy_buffer_u32(
    device const uint* src           [[buffer(0)]],
    device uint* dst                 [[buffer(1)]],
    constant uint& num_elements      [[buffer(2)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= num_elements) return;
    dst[gid] = src[gid];
}

kernel void copy_buffer_float(
    device const float* src          [[buffer(0)]],
    device float* dst                [[buffer(1)]],
    constant uint& num_elements      [[buffer(2)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= num_elements) return;
    dst[gid] = src[gid];
}

kernel void scatter_prefix_sum_seed_last(
    device uint* data                [[buffer(0)]],
    constant uint& num_elements      [[buffer(1)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid != 0u || num_elements == 0u) return;
    data[num_elements - 1u] = 0u;
}

kernel void copy_bits_to_float(
    device const uint* src_bits       [[buffer(0)]],
    device float* dst                [[buffer(1)]],
    constant uint& num_elements      [[buffer(2)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= num_elements) return;
    dst[gid] = as_type<float>(src_bits[gid]);
}

// =============================================================================
// Particle-Particle Interaction Kernel (Collision + Excitation Transfer)
// =============================================================================
// This kernel computes short-range forces between particles:
// 1. Soft-sphere repulsion: prevents overlap, stronger for excited particles
// 2. Excitation transfer: when particles "bump", excitation equilibrates
//
// NOTE: This is O(N²) which is fine for N < 1000. For larger systems,
// use spatial hashing or neighbor lists.

struct ParticleInteractionParams {
    uint32_t num_particles;
    float dt;
    float particle_radius;       // r: particle radius for collision detection
    float young_modulus;         // E: Young's modulus for Hertzian contact (spring stiffness)
    float thermal_conductivity;  // k: heat transfer on contact
    float specific_heat;         // c_v: heat capacity per unit mass
    float restitution;           // e: coefficient of restitution (0-1)
};

kernel void particle_interactions(
    device float* particle_pos            [[buffer(0)]],  // N * 3 (read-only for positions)
    device float* particle_vel            [[buffer(1)]],  // N * 3 (read-write for velocity)
    device float* particle_excitation     [[buffer(2)]],  // N (read-write for excitation)
    device const float* particle_mass     [[buffer(3)]],  // N (read-only)
    device float* particle_heat           [[buffer(4)]],  // N (read-write for heat)
    device const float* particle_vel_in   [[buffer(5)]],  // N * 3 (snapshot for consistent reads)
    device const float* particle_heat_in  [[buffer(6)]],  // N (snapshot for consistent reads)
    constant ParticleInteractionParams& p [[buffer(7)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_particles) return;
    
    // Read this particle's state
    float3 pos_i = float3(
        particle_pos[gid * 3 + 0],
        particle_pos[gid * 3 + 1],
        particle_pos[gid * 3 + 2]
    );
    float3 vel_i = float3(
        particle_vel[gid * 3 + 0],
        particle_vel[gid * 3 + 1],
        particle_vel[gid * 3 + 2]
    );
    float mass_i = particle_mass[gid];
    float heat_i = particle_heat[gid];
    // Note: particle_excitation is read-only intrinsic property, not needed for collisions

    // [CHOICE] collision kernel invariants (fail loudly)
    // [FORMULA] require: m_i>0, c_v>0, r>0, dt>0
    // [REASON] silent clamps hide invalid physical states
    // [NOTES] on violation we write NaNs to outputs for this particle.
    if (!(mass_i > 0.0f) || !(p.specific_heat > 0.0f) || !(p.particle_radius > 0.0f) || !(p.dt > 0.0f)) {
        float qn = qnan_f();
        particle_vel[gid * 3 + 0] = qn;
        particle_vel[gid * 3 + 1] = qn;
        particle_vel[gid * 3 + 2] = qn;
        particle_heat[gid] = qn;
        return;
    }
    
    float particle_radius = p.particle_radius;
    float combined_radius = particle_radius + particle_radius;
    float combined_radius_sq = combined_radius * combined_radius;
    float inv_mass_i = 1.0f / mass_i;
    float cv = p.specific_heat;
    float temperature_i = heat_i / (mass_i * cv);
    float restitution = p.restitution;
    float timestep = p.dt;
    float young_modulus = p.young_modulus;
    float thermal_conductivity = p.thermal_conductivity;

    float3 impulse_total = float3(0.0f);
    float heat_delta = 0.0f;

    for (uint neighbor_idx = 0; neighbor_idx < p.num_particles; neighbor_idx++) {
        if (neighbor_idx == gid) continue;

        float3 pos_j = float3(
            particle_pos[neighbor_idx * 3 + 0],
            particle_pos[neighbor_idx * 3 + 1],
            particle_pos[neighbor_idx * 3 + 2]
        );

        float3 delta = pos_i - pos_j;
        float dist_sq = dot(delta, delta);

        if (dist_sq >= combined_radius_sq || dist_sq < 1e-12f) {
            continue;
        }

        float dist = sqrt(dist_sq);
        float3 vel_j = float3(
            particle_vel_in[neighbor_idx * 3 + 0],
            particle_vel_in[neighbor_idx * 3 + 1],
            particle_vel_in[neighbor_idx * 3 + 2]
        );
        float mass_j = particle_mass[neighbor_idx];
        float heat_j = particle_heat_in[neighbor_idx];

        if (!(mass_j > 0.0f)) {
            float qn = qnan_f();
            particle_vel[gid * 3 + 0] = qn;
            particle_vel[gid * 3 + 1] = qn;
            particle_vel[gid * 3 + 2] = qn;
            particle_heat[gid] = qn;
            return;
        }

        {
            // =====================================================
            // COLLISION DETECTED
            // =====================================================
            float3 normal = delta / dist;
            float overlap = combined_radius - dist;
            float3 relative_velocity = vel_i - vel_j;
            float normal_velocity = dot(relative_velocity, normal);

            if (normal_velocity < 0.0f) {
                float reduced_mass = (mass_i * mass_j) / (mass_i + mass_j);
                float impulse_magnitude = (1.0f + restitution) * reduced_mass * (-normal_velocity);
                impulse_total += (normal * impulse_magnitude * inv_mass_i) * 0.5f;
                float kinetic_energy_lost = 0.5f * reduced_mass * normal_velocity * normal_velocity * (1.0f - restitution * restitution);
                heat_delta += kinetic_energy_lost * 0.5f;
            }

            float contact_force = young_modulus * overlap;
            impulse_total += normal * contact_force * timestep * inv_mass_i;

            float temperature_j = heat_j / (mass_j * cv);
            float contact_area = overlap * overlap;
            float heat_conduction = thermal_conductivity * contact_area * (temperature_j - temperature_i) * timestep;
            heat_delta += heat_conduction;
        }
    }
    
    // Apply accumulated changes
    vel_i += impulse_total;
    heat_i += heat_delta;
    
    // [CHOICE] non-negative thermal energy (0 K baseline)
    // [FORMULA] Q >= 0
    // [REASON] internal thermal energy relative to absolute zero cannot be negative
    // NOTE: No clamping. If numerics drive heat < 0, we want it to surface.
    
    // Write back
    particle_vel[gid * 3 + 0] = vel_i.x;
    particle_vel[gid * 3 + 1] = vel_i.y;
    particle_vel[gid * 3 + 2] = vel_i.z;
    // Note: particle_excitation is NOT written - it's an intrinsic property
    particle_heat[gid] = heat_i;
}

// =============================================================================
// SPATIAL HASH GRID ACCELERATION
// =============================================================================
// Three-phase approach for O(N) collision detection:
//   Phase 1: Assign each particle to a cell (compute cell index)
//   Phase 2: Count particles per cell, compute prefix sum → cell start indices
//   Phase 3: Collision detection using cell-based neighbor lookup
//
// This reduces O(N²) to O(N * k) where k = avg particles in 27 neighbor cells.
// For uniform distributions, k ~ 27 * (N / num_cells) which is constant for
// fixed density, giving O(N) total complexity.
// =============================================================================

// -----------------------------------------------------------------------------
// Utility: Compute cell index from position
// -----------------------------------------------------------------------------
inline uint3 position_to_cell(
    float3 pos,
    float inv_cell_size,
    float3 domain_min,
    uint3 grid_dims
) {
    // [CHOICE] periodic spatial hash domain
    // [FORMULA] cell = floor(((pos-domain_min)/h) mod grid_dims)
    // [REASON] collision neighborhood should match torus/periodic simulation domain
    float3 g = (pos - domain_min) * inv_cell_size;
    float3 gd = float3(grid_dims);
    g = g - gd * floor(g / gd); // wrap into [0,grid_dims)
    return uint3(floor(g));
}

inline uint cell_to_linear(uint3 cell, uint3 grid_dims) {
    return cell.x * grid_dims.y * grid_dims.z + cell.y * grid_dims.z + cell.z;
}

inline uint3 linear_to_cell(uint linear_idx, uint3 grid_dims) {
    uint x = linear_idx / (grid_dims.y * grid_dims.z);
    uint rem = linear_idx % (grid_dims.y * grid_dims.z);
    uint y = rem / grid_dims.z;
    uint z = rem % grid_dims.z;
    return uint3(x, y, z);
}

// -----------------------------------------------------------------------------
// Kernel: Assign particles to cells (Phase 1)
// -----------------------------------------------------------------------------
// Each particle computes its cell index and stores it.
// Also atomically increments the cell's particle count.

kernel void spatial_hash_assign(
    device const float* particle_pos       [[buffer(0)]],  // N * 3
    device uint* particle_cell_idx         [[buffer(1)]],  // N (output: linear cell index)
    device atomic_uint* cell_counts        [[buffer(2)]],  // num_cells (output: count per cell)
    constant SpatialHashParams& p          [[buffer(3)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_particles) return;
    
    float3 pos = float3(
        particle_pos[gid * 3 + 0],
        particle_pos[gid * 3 + 1],
        particle_pos[gid * 3 + 2]
    );
    
    float3 domain_min = float3(p.domain_min_x, p.domain_min_y, p.domain_min_z);
    uint3 grid_dims = uint3(p.grid_x, p.grid_y, p.grid_z);
    
    uint3 cell = position_to_cell(pos, p.inv_cell_size, domain_min, grid_dims);
    uint linear_idx = cell_to_linear(cell, grid_dims);
    
    particle_cell_idx[gid] = linear_idx;
    atomic_fetch_add_explicit(&cell_counts[linear_idx], 1, memory_order_relaxed);
}

// -----------------------------------------------------------------------------
// Kernel: Exclusive prefix sum on cell counts (Phase 2a)
// -----------------------------------------------------------------------------
// Computes cell_starts[i] = sum(cell_counts[0..i-1])
// This gives the starting index in the sorted particle array for each cell.
//
// For small grids (< 64³ = 262k cells), single-thread scan is acceptable.
// For larger grids, use parallel Blelloch scan.

kernel void spatial_hash_prefix_sum(
    device const uint* cell_counts         [[buffer(0)]],  // num_cells
    device uint* cell_starts               [[buffer(1)]],  // num_cells + 1
    constant uint& num_cells               [[buffer(2)]],
    uint gid [[thread_position_in_grid]]
) {
    // Single-thread sequential scan (for num_cells up to ~256k)
    if (gid != 0) return;
    
    uint running_sum = 0;
    for (uint i = 0; i < num_cells; i++) {
        cell_starts[i] = running_sum;
        running_sum += cell_counts[i];
    }
    cell_starts[num_cells] = running_sum;  // Total particle count
}

// Parallel Blelloch-style prefix sum for larger grids
// This uses threadgroup-local reductions for better scaling
kernel void spatial_hash_prefix_sum_parallel(
    device uint* cell_counts               [[buffer(0)]],  // num_cells (in/out: becomes cell_starts)
    device uint* block_sums                [[buffer(1)]],  // (num_cells / BLOCK_SIZE) intermediate sums
    constant uint& num_cells               [[buffer(2)]],
    uint gid [[thread_position_in_grid]],
    uint tid [[thread_position_in_threadgroup]],
    uint tg_size [[threads_per_threadgroup]],
    threadgroup uint* shared [[threadgroup(0)]]
) {
    // Load into shared memory
    uint idx = gid;
    shared[tid] = (idx < num_cells) ? cell_counts[idx] : 0;
    threadgroup_barrier(mem_flags::mem_threadgroup);
    
    // Up-sweep (reduce) phase
    for (uint stride = 1; stride < tg_size; stride *= 2) {
        uint ai = (tid + 1) * stride * 2 - 1;
        if (ai < tg_size) {
            shared[ai] += shared[ai - stride];
        }
        threadgroup_barrier(mem_flags::mem_threadgroup);
    }
    
    // Store block sum and clear last element
    if (tid == tg_size - 1) {
        uint block_idx = gid / tg_size;
        block_sums[block_idx] = shared[tid];
        shared[tid] = 0;
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);
    
    // Down-sweep phase
    for (uint stride = tg_size / 2; stride > 0; stride /= 2) {
        uint ai = (tid + 1) * stride * 2 - 1;
        if (ai < tg_size) {
            uint t = shared[ai - stride];
            shared[ai - stride] = shared[ai];
            shared[ai] += t;
        }
        threadgroup_barrier(mem_flags::mem_threadgroup);
    }
    
    // Write back exclusive prefix sum
    if (idx < num_cells) {
        cell_counts[idx] = shared[tid];
    }
}

// -----------------------------------------------------------------------------
// Generic kernel: u32 exclusive scan (parallel, block-hierarchical)
// -----------------------------------------------------------------------------
// [CHOICE] parallel prefix sum (exclusive) for uint32 buffers
// [FORMULA] out[i] = Σ_{j < i} in[j]
// [REASON] required for O(N) spatial hash and spectral frequency binning without CPU sync
// [NOTES] This is implemented as a block scan + hierarchical scan of block sums.
//
// Pass 1: per-block exclusive scan, emitting `block_sums[block]`.
kernel void exclusive_scan_u32_pass1(
    device const uint* in                [[buffer(0)]],  // n
    device uint* out                     [[buffer(1)]],  // n
    device uint* block_sums              [[buffer(2)]],  // num_blocks
    constant uint& n                     [[buffer(3)]],
    uint tid [[thread_index_in_threadgroup]],
    uint tg_id [[threadgroup_position_in_grid]],
    uint tg_size [[threads_per_threadgroup]],
    threadgroup uint* shared             [[threadgroup(0)]]
) {
    uint idx = tg_id * tg_size + tid;
    shared[tid] = (idx < n) ? in[idx] : 0u;
    threadgroup_barrier(mem_flags::mem_threadgroup);

    // Up-sweep
    for (uint stride = 1; stride < tg_size; stride <<= 1) {
        uint ai = ((tid + 1u) * stride * 2u) - 1u;
        if (ai < tg_size) shared[ai] += shared[ai - stride];
        threadgroup_barrier(mem_flags::mem_threadgroup);
    }

    uint total = shared[tg_size - 1u];
    if (tid == tg_size - 1u) {
        block_sums[tg_id] = total;
        shared[tg_size - 1u] = 0u; // exclusive
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);

    // Down-sweep
    for (uint stride = tg_size >> 1; stride > 0; stride >>= 1) {
        uint ai = ((tid + 1u) * stride * 2u) - 1u;
        if (ai < tg_size) {
            uint t = shared[ai - stride];
            shared[ai - stride] = shared[ai];
            shared[ai] += t;
        }
        threadgroup_barrier(mem_flags::mem_threadgroup);
    }

    if (idx < n) out[idx] = shared[tid];
}

// Pass 2/3 helper: add scanned block offsets to per-block scan output.
kernel void exclusive_scan_u32_add_block_offsets(
    device uint* out                      [[buffer(0)]],  // n (in/out)
    device const uint* block_prefix       [[buffer(1)]],  // num_blocks (exclusive scan of block_sums)
    constant uint& n                      [[buffer(2)]],
    uint tid [[thread_index_in_threadgroup]],
    uint tg_id [[threadgroup_position_in_grid]],
    uint tg_size [[threads_per_threadgroup]]
) {
    uint idx = tg_id * tg_size + tid;
    if (idx >= n) return;
    out[idx] += block_prefix[tg_id];
}

// Optional helper: write total sum as out[n] for (n+1)-length start arrays.
kernel void exclusive_scan_u32_finalize_total(
    device const uint* in                 [[buffer(0)]],  // n
    device uint* out                      [[buffer(1)]],  // n+1 (first n already filled with exclusive scan)
    constant uint& n                      [[buffer(2)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid != 0) return;
    if (n == 0) { out[0] = 0u; return; }
    out[n] = out[n - 1u] + in[n - 1u];
}

// -----------------------------------------------------------------------------
// Kernel: Scatter particles to sorted array (Phase 2b)
// -----------------------------------------------------------------------------
// Places particle indices into a sorted array based on their cell.
// Uses atomic counters per cell to handle collisions within cells.

kernel void spatial_hash_scatter(
    device const uint* particle_cell_idx   [[buffer(0)]],  // N (cell index per particle)
    device uint* sorted_particle_idx       [[buffer(1)]],  // N (output: sorted indices)
    device atomic_uint* cell_offsets       [[buffer(2)]],  // num_cells (working offsets)
    constant uint& num_particles           [[buffer(3)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= num_particles) return;
    
    uint cell_idx = particle_cell_idx[gid];
    uint slot = atomic_fetch_add_explicit(&cell_offsets[cell_idx], 1, memory_order_relaxed);
    sorted_particle_idx[slot] = gid;
}

// =============================================================================
// Coherence ω-binning (GPU-only)
// =============================================================================
// Buckets ω-bins by ω_k to enable sparse coupling by scanning only nearby bins.
//
// This is designed to be exact w.r.t. fp32 tuning semantics:
// we will later choose bin width such that bins beyond a fixed neighborhood
// contribute exactly 0 to the Gaussian in fp32 (exp-underflow).

// -----------------------------------------------------------------------------
// Constants / types for spectral binning (must appear before kernels)
// -----------------------------------------------------------------------------
// [CHOICE] fp32 exp underflow boundary for Gaussian tuning
// [FORMULA] Let FLT_TRUE_MIN = 2^-149. exp(-x) underflows to 0 in fp32 for x >= x0,
//           where x0 = -ln(FLT_TRUE_MIN) = -ln(2^-149) = 149 * ln(2).
// [REASON] enables exact sparsity: interactions with (Δω^2/σ^2) >= x0 contribute
//          exactly 0 in fp32, so they are provably irrelevant.
constant float kFp32ExpUnderflowX0 = 103.27893f; // 149*ln(2) (rounded to fp32)

struct SpectralBinParams {
    float omega_min;
    float inv_bin_width;
};

// Type alias for clarity: this module implements a coherence field Ψ(ω).
typedef SpectralBinParams CoherenceBinParams;

// [CHOICE] float→ordered-u32 mapping for atomic min/max
// [FORMULA] key = (sign? ~u : (u ^ 0x80000000)), where u is IEEE-754 bits of float
// [REASON] enables atomic_min/atomic_max on floats using atomic_uint while preserving ordering
inline uint float_to_ordered_u32(float x) {
    uint u = as_type<uint>(x);
    uint sign = u & 0x80000000u;
    return (sign != 0u) ? ~u : (u ^ 0x80000000u);
}

inline float ordered_u32_to_float(uint key) {
    uint sign = key & 0x80000000u;
    uint u = (sign != 0u) ? (key ^ 0x80000000u) : ~key;
    return as_type<float>(u);
}

// Prototypes (definitions appear later in file).
inline void atomic_max_uint_device(device atomic_uint* address, uint val);
inline void atomic_min_uint_device(device atomic_uint* address, uint val);

kernel void coherence_reduce_omega_minmax_keys(
    device const float* carrier_omega       [[buffer(0)]],  // maxM
    device const uint* num_carriers_in      [[buffer(1)]],  // (1,) snapshot
    device atomic_uint* omega_min_key       [[buffer(2)]],  // (1,) init = 0xFFFFFFFF
    device atomic_uint* omega_max_key       [[buffer(3)]],  // (1,) init = 0
    uint gid [[thread_position_in_grid]]
) {
    uint n = (num_carriers_in != nullptr) ? num_carriers_in[0] : 0u;
    if (gid >= n) return;
    float w = carrier_omega[gid];
    if (!isfinite(w)) return;
    uint key = float_to_ordered_u32(w);
    atomic_min_uint_device(&omega_min_key[0], key);
    atomic_max_uint_device(&omega_max_key[0], key);
}

kernel void coherence_compute_bin_params(
    device const atomic_uint* omega_min_key [[buffer(0)]], // (1,)
    device const atomic_uint* omega_max_key [[buffer(1)]], // (1,)
    device const uint* num_carriers_in      [[buffer(2)]], // (1,)
    device CoherenceBinParams* out_params   [[buffer(3)]], // (1,)
    constant float& gate_width_max          [[buffer(4)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid != 0) return;
    uint n = (num_carriers_in != nullptr) ? max(num_carriers_in[0], 1u) : 1u;

    float wmin = ordered_u32_to_float(atomic_load_explicit(&omega_min_key[0], memory_order_relaxed));
    float wmax = ordered_u32_to_float(atomic_load_explicit(&omega_max_key[0], memory_order_relaxed));
    float range = wmax - wmin;

    // [CHOICE] fp32-exact coupling support radius for Gaussian
    // [FORMULA] R_max = sqrt(x0) * σ_max, where x0 = -ln(FLT_TRUE_MIN)
    // [REASON] outside this radius, exp(-(Δω/σ)^2) underflows to 0 in fp32 (exactly)
    float R_max = sqrt(kFp32ExpUnderflowX0) * gate_width_max;

    // [CHOICE] bin width (derived, no knob)
    // [FORMULA] W = max(R_max, range / n)
    // [REASON] ensures finite binning resolution without user-tunable parameters
    float W = max(R_max, (n > 0u) ? (range / (float)n) : R_max);
    if (!(W > 0.0f)) {
        out_params[0].omega_min = qnan_f();
        out_params[0].inv_bin_width = qnan_f();
        return;
    }

    out_params[0].omega_min = wmin;
    out_params[0].inv_bin_width = 1.0f / W;
}

kernel void coherence_bin_count_carriers(
    device const float* carrier_omega       [[buffer(0)]],  // maxM
    device const uint* num_carriers_in      [[buffer(1)]],  // (1,)
    device atomic_uint* bin_counts          [[buffer(2)]],  // num_bins
    device const CoherenceBinParams* bin_p  [[buffer(3)]],  // (1,)
    constant uint& num_bins                 [[buffer(4)]],
    uint gid [[thread_position_in_grid]]
) {
    uint n = (num_carriers_in != nullptr) ? num_carriers_in[0] : 0u;
    if (gid >= n) return;
    float w = carrier_omega[gid];
    float f = (w - bin_p[0].omega_min) * bin_p[0].inv_bin_width;
    int bi = (int)floor(f);
    if (bi < 0 || bi >= (int)num_bins) return;
    atomic_fetch_add_explicit(&bin_counts[(uint)bi], 1u, memory_order_relaxed);
}

kernel void derive_max_carrier_bin(
    device const float* carrier_omega       [[buffer(0)]],
    device const CoherenceBinParams* bin_p  [[buffer(1)]],
    device atomic_uint* max_bin_out         [[buffer(2)]],
    device const uint* num_carriers_in      [[buffer(3)]],
    uint gid [[thread_position_in_grid]]
) {
    uint num_carriers = (num_carriers_in != nullptr) ? num_carriers_in[0] : 0u;
    if (gid >= num_carriers) return;
    float omega = carrier_omega[gid];
    float fbin = (omega - bin_p[0].omega_min) * bin_p[0].inv_bin_width;
    int bin_index = (int)floor(fbin);
    if (bin_index < 0) return;
    atomic_fetch_max_explicit(max_bin_out, (uint)bin_index, memory_order_relaxed);
}

kernel void coherence_bin_scatter_carriers(
    device const float* carrier_omega       [[buffer(0)]],  // maxM
    device const uint* num_carriers_in      [[buffer(1)]],  // (1,)
    device atomic_uint* bin_offsets         [[buffer(2)]],  // num_bins (working copy of starts)
    device const CoherenceBinParams* bin_p  [[buffer(3)]],  // (1,)
    constant uint& num_bins                 [[buffer(4)]],
    device uint* carrier_binned_idx         [[buffer(5)]],  // maxM
    uint gid [[thread_position_in_grid]]
) {
    uint n = (num_carriers_in != nullptr) ? num_carriers_in[0] : 0u;
    if (gid >= n) return;
    float w = carrier_omega[gid];
    float f = (w - bin_p[0].omega_min) * bin_p[0].inv_bin_width;
    int bi = (int)floor(f);
    if (bi < 0 || bi >= (int)num_bins) return;
    uint slot = atomic_fetch_add_explicit(&bin_offsets[(uint)bi], 1u, memory_order_relaxed);
    carrier_binned_idx[slot] = gid;
}

kernel void coherence_fuse_binning(
    device const float* carrier_omega       [[buffer(0)]],  // maxM
    device const uint* num_carriers_in      [[buffer(1)]],  // (1,)
    device uint* bin_counts                 [[buffer(2)]],  // maxM
    device uint* bin_starts                 [[buffer(3)]],  // maxM + 1
    device uint* bin_offsets                [[buffer(4)]],  // maxM
    device uint* carrier_binned_idx         [[buffer(5)]],  // maxM
    device const CoherenceBinParams* bin_p  [[buffer(6)]],  // (1,)
    constant uint& num_bins                 [[buffer(7)]],
    uint tid [[thread_position_in_threadgroup]],
    uint t_dim [[threads_per_threadgroup]]
) {
    threadgroup atomic_uint local_counts[1025];
    threadgroup atomic_uint local_starts[1025];
    threadgroup uint scan_temp_a[1025];
    threadgroup uint scan_temp_b[1025];

    if (num_bins > 1024u) {
        return;
    }

    uint n = (num_carriers_in != nullptr) ? num_carriers_in[0] : 0u;

    for (uint i = tid; i < 1025; i += t_dim) {
        atomic_store_explicit(&local_counts[i], 0u, memory_order_relaxed);
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);

    for (uint i = tid; i < n; i += t_dim) {
        float w = carrier_omega[i];
        float f = (w - bin_p[0].omega_min) * bin_p[0].inv_bin_width;
        int bi = (int)floor(f);
        if (bi >= 0 && bi < (int)num_bins) {
            atomic_fetch_add_explicit(&local_counts[bi], 1u, memory_order_relaxed);
        }
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);

    for (uint i = tid; i < num_bins; i += t_dim) {
        uint val = atomic_load_explicit(&local_counts[i], memory_order_relaxed);
        bin_counts[i] = val;
        atomic_store_explicit(&local_starts[i], val, memory_order_relaxed);
    }
    if (tid == 0) {
        atomic_store_explicit(&local_starts[num_bins], 0u, memory_order_relaxed);
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);

    for (uint i = tid; i <= num_bins; i += t_dim) {
        uint val = (i == 0) ? 0u : atomic_load_explicit(&local_starts[i - 1], memory_order_relaxed);
        scan_temp_a[i] = val;
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);

    threadgroup uint* src = scan_temp_a;
    threadgroup uint* dst = scan_temp_b;

    for (uint offset = 1; offset <= num_bins; offset <<= 1) {
        for (uint i = tid; i <= num_bins; i += t_dim) {
            if (i >= offset) {
                dst[i] = src[i] + src[i - offset];
            } else {
                dst[i] = src[i];
            }
        }
        threadgroup_barrier(mem_flags::mem_threadgroup);
        threadgroup uint* tmp = src;
        src = dst;
        dst = tmp;
    }

    for (uint i = tid; i <= num_bins; i += t_dim) {
        bin_starts[i] = src[i];
        if (i < num_bins) {
            bin_offsets[i] = src[i];
            atomic_store_explicit(&local_starts[i], src[i], memory_order_relaxed);
        }
    }
    threadgroup_barrier(mem_flags::mem_threadgroup | mem_flags::mem_device);

    for (uint i = tid; i < n; i += t_dim) {
        float w = carrier_omega[i];
        float f = (w - bin_p[0].omega_min) * bin_p[0].inv_bin_width;
        int bi = (int)floor(f);
        if (bi >= 0 && bi < (int)num_bins) {
            uint slot = atomic_fetch_add_explicit(&local_starts[bi], 1u, memory_order_relaxed);
            carrier_binned_idx[slot] = i;
        }
    }
}


// -----------------------------------------------------------------------------
// Kernel: Spatial hash collision detection (Phase 3)
// -----------------------------------------------------------------------------
// For each particle, check only particles in the same cell and 26 neighbors.
// This is O(N * k) where k = avg particles per 27-cell neighborhood.

kernel void spatial_hash_collisions(
    // Particle state
    device const float* particle_pos       [[buffer(0)]],  // N * 3
    device float* particle_vel             [[buffer(1)]],  // N * 3
    device float* particle_excitation      [[buffer(2)]],  // N
    device const float* particle_mass      [[buffer(3)]],  // N
    device float* particle_heat            [[buffer(4)]],  // N
    // Spatial hash data
    device const uint* sorted_particle_idx [[buffer(5)]],  // N (sorted by cell)
    device const uint* cell_starts         [[buffer(6)]],  // num_cells + 1
    device const uint* particle_cell_idx   [[buffer(7)]],  // N (cell index per particle)
    // Snapshot inputs for consistent reads (avoid write hazards)
    device const float* particle_vel_in    [[buffer(8)]],  // N * 3
    device const float* particle_heat_in   [[buffer(9)]],  // N
    // Parameters
    constant SpatialCollisionParams& p     [[buffer(10)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_particles) return;
    
    // Read this particle's state
    float3 pos_i = float3(
        particle_pos[gid * 3 + 0],
        particle_pos[gid * 3 + 1],
        particle_pos[gid * 3 + 2]
    );
    float3 vel_i = float3(
        particle_vel[gid * 3 + 0],
        particle_vel[gid * 3 + 1],
        particle_vel[gid * 3 + 2]
    );
    float mass_i = particle_mass[gid];
    float heat_i = particle_heat[gid];
    // Note: particle_excitation is read-only intrinsic property, not needed for collisions

    // Collision kernel invariants (fail loudly). See particle_interactions for rationale.
    if (!(mass_i > 0.0f) || !(p.specific_heat > 0.0f) || !(p.particle_radius > 0.0f) || !(p.dt > 0.0f)) {
        float qn = qnan_f();
        particle_vel[gid * 3 + 0] = qn;
        particle_vel[gid * 3 + 1] = qn;
        particle_vel[gid * 3 + 2] = qn;
        particle_heat[gid] = qn;
        return;
    }
    
    float particle_radius = p.particle_radius;
    float combined_radius = particle_radius + particle_radius;
    float combined_radius_sq = combined_radius * combined_radius;
    float inv_mass_i = 1.0f / mass_i;
    float cv = p.specific_heat;
    float temperature_i = heat_i / (mass_i * cv);
    float restitution = p.restitution;
    float timestep = p.dt;
    float young_modulus = p.young_modulus;
    float thermal_conductivity = p.thermal_conductivity;
    uint3 grid_dims = uint3(p.grid_x, p.grid_y, p.grid_z);

    float3 domain_min = float3(p.domain_min_x, p.domain_min_y, p.domain_min_z);
    uint3 cell_i = position_to_cell(pos_i, p.inv_cell_size, domain_min, grid_dims);

    float3 impulse_total = float3(0.0f);
    float heat_delta = 0.0f;
    
    // Iterate over 3x3x3 neighborhood (27 cells)
    for (int dx = -1; dx <= 1; dx++) {
        for (int dy = -1; dy <= 1; dy++) {
            for (int dz = -1; dz <= 1; dz++) {
                // [CHOICE] periodic neighbor wrap
                // [FORMULA] neighbor = (cell_i + d) mod grid_dims
                // [REASON] consistent with periodic domain; no boundary “dead zones”
                int3 neighbor = int3(cell_i) + int3(dx, dy, dz);
                neighbor.x = (neighbor.x % (int)p.grid_x + (int)p.grid_x) % (int)p.grid_x;
                neighbor.y = (neighbor.y % (int)p.grid_y + (int)p.grid_y) % (int)p.grid_y;
                neighbor.z = (neighbor.z % (int)p.grid_z + (int)p.grid_z) % (int)p.grid_z;

                uint neighbor_linear = cell_to_linear(uint3(neighbor), grid_dims);
                uint start = cell_starts[neighbor_linear];
                uint end = cell_starts[neighbor_linear + 1];
                
                // Iterate over particles in this cell
                for (uint slot = start; slot < end; slot++) {
                    uint j = sorted_particle_idx[slot];
                    if (j == gid) continue;  // Skip self
                    
                    float3 pos_j = float3(
                        particle_pos[j * 3 + 0],
                        particle_pos[j * 3 + 1],
                        particle_pos[j * 3 + 2]
                    );
                    
                    float3 delta = pos_i - pos_j;
                    float dist_sq = dot(delta, delta);

                    if (dist_sq >= combined_radius_sq || dist_sq < 1e-12f) {
                        continue;
                    }

                    float dist = sqrt(dist_sq);
                    float3 vel_j = float3(
                        particle_vel_in[j * 3 + 0],
                        particle_vel_in[j * 3 + 1],
                        particle_vel_in[j * 3 + 2]
                    );
                    float mass_j = particle_mass[j];
                    float heat_j = particle_heat_in[j];

                    if (!(mass_j > 0.0f)) {
                        float qn = qnan_f();
                        particle_vel[gid * 3 + 0] = qn;
                        particle_vel[gid * 3 + 1] = qn;
                        particle_vel[gid * 3 + 2] = qn;
                        particle_heat[gid] = qn;
                        return;
                    }
                    
                    float3 normal = delta / dist;
                    float overlap = combined_radius - dist;
                    float3 relative_velocity = vel_i - vel_j;
                    float normal_velocity = dot(relative_velocity, normal);

                    if (normal_velocity < 0.0f) {
                        float reduced_mass = (mass_i * mass_j) / (mass_i + mass_j);
                        float impulse_magnitude = (1.0f + restitution) * reduced_mass * (-normal_velocity);
                        impulse_total += (normal * impulse_magnitude * inv_mass_i) * 0.5f;
                        float kinetic_energy_lost = 0.5f * reduced_mass * normal_velocity * normal_velocity * (1.0f - restitution * restitution);
                        heat_delta += kinetic_energy_lost * 0.5f;
                    }

                    float contact_force = young_modulus * overlap;
                    impulse_total += normal * contact_force * timestep * inv_mass_i;

                    float temperature_j = heat_j / (mass_j * cv);
                    float contact_area = overlap * overlap;
                    float heat_conduction = thermal_conductivity * contact_area * (temperature_j - temperature_i) * timestep;
                    heat_delta += heat_conduction;
                    
                    // Note: Excitation (oscillator frequency) is INTRINSIC - no equilibration
                }
            }
        }
    }
    
    // Apply accumulated changes
    vel_i += impulse_total;
    heat_i += heat_delta;
    
    // Physical constraints
    // NOTE: No clamping. If numerics drive heat < 0, we want it to surface.
    
    // Write back
    particle_vel[gid * 3 + 0] = vel_i.x;
    particle_vel[gid * 3 + 1] = vel_i.y;
    particle_vel[gid * 3 + 2] = vel_i.z;
    // Note: particle_excitation NOT written - intrinsic property
    particle_heat[gid] = heat_i;
}

inline void atomic_add_float_threadgroup(threadgroup atomic_uint* address, float val) {
    uint old_val = atomic_load_explicit(address, memory_order_relaxed);
    uint new_val;

    while (true) {
        float old_f = as_type<float>(old_val);
        float new_f = old_f + val;
        new_val = as_type<uint>(new_f);

        if (atomic_compare_exchange_weak_explicit(
                address,
                &old_val,
                new_val,
                memory_order_relaxed,
                memory_order_relaxed)) {
            break;
        }
    }
}

inline void atomic_add_float_device(device atomic_uint* address, float val) {
    device atomic_float* floatAddress = reinterpret_cast<device atomic_float*>(address);
    atomic_fetch_add_explicit(floatAddress, val, memory_order_relaxed);
}

// =============================================================================
// SORT-BASED SCATTER (Deterministic, No Hash Collisions)
// =============================================================================
// This implementation pre-sorts particles by their primary grid cell, then
// scatters in sorted order. Benefits over hash-based approach:
// - No warp divergence from hash collision fallback
// - Coalesced memory reads from sorted particle array
// - Performance is CONSTANT regardless of particle density
// - Deterministic floating-point accumulation order
//
// Pipeline:
// 1. scatter_compute_cell_idx: Compute primary cell for each particle
// 2. scatter_count_cells: Count particles per cell (atomic)
// 3. scatter_prefix_sum: Compute cell_starts from cell_counts
// 4. scatter_reorder: Move particles to sorted positions
// 5. scatter_gather_cells: Inverted PIC scatter (one thread per grid cell)

struct SortScatterParams {
    uint32_t num_particles;
    uint32_t num_cells;       // gx * gy * gz
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float grid_spacing;
    float inv_grid_spacing;
};

struct PicGatherParams {
    uint32_t num_particles;
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float grid_spacing;
    float inv_grid_spacing;
    float dt;
    float domain_x;
    float domain_y;
    float domain_z;
    float gamma;
    float R_specific;
    float c_v;
    float rho_min;
    float p_min;
    float gravity_enabled;  // 1.0 if gravity field is valid, 0.0 otherwise
};

// -----------------------------------------------------------------------------
// Quantum Flow / Pilot-Wave coupling
// -----------------------------------------------------------------------------
//
// These parameters and kernels let us build a position-space complex field Ψ(x)
// from ω-modes anchored to particles, then advect particles using the standard
// quantum probability current guidance equation:
//
//   v(x) = (ħ/m) * Im(conj(Ψ) ∇Ψ) / (|Ψ|^2 + ε)
//
// where Im(conj(Ψ) ∇Ψ) = Ψ_re ∇Ψ_im - Ψ_im ∇Ψ_re.
//
// The goal is to let "wave coherence" drive spatial motion directly, without
// inventing additional hand-shaped forces.

struct ModeProjectParams {
    uint32_t num_modes;
    uint32_t num_particles;
    uint32_t anchors_per_mode;
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float grid_spacing;
    float inv_grid_spacing;
};

struct PilotWaveParams {
    uint32_t num_particles;
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float grid_spacing;
    float inv_grid_spacing;
    float dt;
    float domain_x;
    float domain_y;
    float domain_z;
    float hbar_eff;
    float eps_denom;
    float mass_min;
};


// Step 1: Compute primary cell index for each particle
kernel void scatter_compute_cell_idx(
    device const float* particle_pos      [[buffer(0)]],  // N * 3
    device uint* particle_cell_idx        [[buffer(1)]],  // N (output)
    constant SortScatterParams& p         [[buffer(2)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_particles) return;

    float3 pos = float3(
        particle_pos[gid * 3 + 0],
        particle_pos[gid * 3 + 1],
        particle_pos[gid * 3 + 2]
    );

    // Primary cell for sort binning. Positions are host-wrapped into [0,domain);
    // uint(scaled) differs from trilinear_coords floor-mod on negative values.
    uint3 grid_dims = uint3(p.grid_x, p.grid_y, p.grid_z);
    float3 scaled = pos * p.inv_grid_spacing;
    uint3 cell = uint3(
        uint(scaled.x) % grid_dims.x,
        uint(scaled.y) % grid_dims.y,
        uint(scaled.z) % grid_dims.z
    );

    // Linear cell index (x-major order)
    uint stride_z = 1;
    uint stride_y = p.grid_z;
    uint stride_x = p.grid_y * p.grid_z;
    uint cell_idx = cell.x * stride_x + cell.y * stride_y + cell.z * stride_z;

    particle_cell_idx[gid] = cell_idx;
}

// Step 2: Count particles per cell
kernel void scatter_count_cells(
    device const uint* particle_cell_idx  [[buffer(0)]],  // N
    device atomic_uint* cell_counts       [[buffer(1)]],  // num_cells
    constant SortScatterParams& p         [[buffer(2)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_particles) return;
    uint cell = particle_cell_idx[gid];
    atomic_fetch_add_explicit(&cell_counts[cell], 1u, memory_order_relaxed);
}

// Step 3: Prefix sum (Blelloch-style, two-phase)
// Phase A: Up-sweep (reduce)
kernel void scatter_prefix_sum_upsweep(
    device uint* data                     [[buffer(0)]],  // num_cells (in-place)
    constant uint& stride                 [[buffer(1)]],  // Current stride (1, 2, 4, ...)
    constant uint& n                      [[buffer(2)]],  // Array size
    uint gid [[thread_position_in_grid]]
) {
    uint idx = (gid + 1) * stride * 2 - 1;
    if (idx < n) {
        data[idx] += data[idx - stride];
    }
}

// Phase B: Down-sweep
kernel void scatter_prefix_sum_downsweep(
    device uint* data                     [[buffer(0)]],  // num_cells (in-place)
    constant uint& stride                 [[buffer(1)]],  // Current stride (n/2, n/4, ...)
    constant uint& n                      [[buffer(2)]],  // Array size
    uint gid [[thread_position_in_grid]]
) {
    uint idx = (gid + 1) * stride * 2 - 1;
    if (idx + stride < n) {
        data[idx + stride] += data[idx];
    }
}

// Step 4: Reorder particles to sorted positions
kernel void scatter_reorder_particles(
    device const float* particle_pos_in   [[buffer(0)]],  // N * 3
    device const float* particle_vel_in   [[buffer(1)]],  // N * 3
    device const float* particle_mass_in  [[buffer(2)]],  // N
    device const float* particle_heat_in  [[buffer(3)]],  // N
    device const float* particle_energy_in[[buffer(4)]],  // N
    device const uint* particle_cell_idx  [[buffer(5)]],  // N
    device const uint* cell_starts        [[buffer(6)]],  // num_cells (exclusive prefix sum)
    device atomic_uint* cell_offsets      [[buffer(7)]],  // num_cells (working copy, atomically incremented)
    device float* particle_pos_out        [[buffer(8)]],  // N * 3
    device float* particle_vel_out        [[buffer(9)]],  // N * 3
    device float* particle_mass_out       [[buffer(10)]], // N
    device float* particle_heat_out       [[buffer(11)]], // N
    device float* particle_energy_out     [[buffer(12)]], // N
    device uint* sorted_original_idx      [[buffer(13)]], // N
    constant SortScatterParams& p         [[buffer(14)]],
    device float4* particle_cic_a         [[buffer(15)]], // N * 4
    device float4* particle_cic_b         [[buffer(16)]], // N * 4
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_particles) return;

    uint cell = particle_cell_idx[gid];
    uint base = cell_starts[cell];
    uint offset = atomic_fetch_add_explicit(&cell_offsets[cell], 1u, memory_order_relaxed);
    uint dest = base + offset;

    // Copy particle data to sorted position
    float3 pos = float3(
        particle_pos_in[gid * 3 + 0],
        particle_pos_in[gid * 3 + 1],
        particle_pos_in[gid * 3 + 2]
    );
    particle_pos_out[dest * 3 + 0] = pos.x;
    particle_pos_out[dest * 3 + 1] = pos.y;
    particle_pos_out[dest * 3 + 2] = pos.z;
    
    float3 vel = float3(
        particle_vel_in[gid * 3 + 0],
        particle_vel_in[gid * 3 + 1],
        particle_vel_in[gid * 3 + 2]
    );
    particle_vel_out[dest * 3 + 0] = vel.x;
    particle_vel_out[dest * 3 + 1] = vel.y;
    particle_vel_out[dest * 3 + 2] = vel.z;
    
    float mass = particle_mass_in[gid];
    float heat = particle_heat_in[gid];
    
    particle_mass_out[dest] = mass;
    particle_heat_out[dest] = heat;
    particle_energy_out[dest] = particle_energy_in[gid];
    sorted_original_idx[dest] = gid;

    // Precompute CIC fractions/weights at sorted destination index
    uint3 base_idx;
    float3 frac;
    trilinear_coords(pos, p.inv_grid_spacing, uint3(kGridX, kGridY, kGridZ), base_idx, frac);
    float iv = p.inv_grid_spacing * p.inv_grid_spacing * p.inv_grid_spacing;
    particle_cic_a[dest] = float4(frac, mass * iv);
    particle_cic_b[dest] = float4(mass * vel * iv, heat * iv);
}

inline uint scatter_cell_range_end(
    uint base_cell,
    device const uint* cell_starts,
    constant SortScatterParams& scatter_params
) {
    uint next_cell = base_cell + 1u;

    if (next_cell < scatter_params.num_cells) {
        return cell_starts[next_cell];
    }

    return scatter_params.num_particles;
}

// Step 5: Gather particle deposits into each grid cell (deterministic, zero atomics).
kernel void scatter_gather_cells(
    device const float4* particle_cic_a   [[buffer(0)]],
    device const float4* particle_cic_b   [[buffer(1)]],
    device const uint* cell_starts        [[buffer(2)]],
    device float4* mom_rho_field          [[buffer(3)]],
    device float* E_field                 [[buffer(4)]],
    constant SortScatterParams& p         [[buffer(5)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_cells) {
        return;
    }

    uint stride_y = kGridZ;
    uint stride_x = kGridY * kGridZ;
    uint cx = gid / stride_x;
    uint rem = gid % stride_x;
    uint cy = rem / stride_y;
    uint cz = rem - cy * stride_y;

    float4 mr_base = mom_rho_field[gid];
    float e_base = E_field[gid];

    float rho_dep = 0.0f;
    float e_dep = 0.0f;
    float mx_dep = 0.0f;
    float my_dep = 0.0f;
    float mz_dep = 0.0f;

    uint sx_vals[2] = {(cx + kGridX - 1u) % kGridX, cx};
    uint sy_vals[2] = {(cy + kGridY - 1u) % kGridY, cy};
    uint sz_vals[2] = {(cz + kGridZ - 1u) % kGridZ, cz};

    for (uint ix = 0; ix < 2u; ix++) {
        for (uint iy = 0; iy < 2u; iy++) {
            for (uint iz = 0; iz < 2u; iz++) {
                uint sbx = sx_vals[ix];
                uint sby = sy_vals[iy];
                uint sbz = sz_vals[iz];
                uint base_cell = sbx * stride_x + sby * stride_y + sbz;
                uint start = cell_starts[base_cell];
                uint end = scatter_cell_range_end(base_cell, cell_starts, p);

                for (uint particle_idx = start; particle_idx < end; particle_idx++) {
                    float4 a = particle_cic_a[particle_idx];   // frac.xyz, rho_q
                    float4 b = particle_cic_b[particle_idx];   // mom_q.xyz, e_q
                    
                    float wx = (ix == 1u) ? (1.0f - a.x) : a.x;
                    float wy = (iy == 1u) ? (1.0f - a.y) : a.y;
                    float wz = (iz == 1u) ? (1.0f - a.z) : a.z;
                    float w = wx * wy * wz;

                    rho_dep += a.w * w;
                    mx_dep  += b.x * w;
                    my_dep  += b.y * w;
                    mz_dep  += b.z * w;
                    e_dep   += b.w * w;
                }
            }
        }
    }

    mom_rho_field[gid] = float4(mr_base.xyz + float3(mx_dep, my_dep, mz_dep), mr_base.w + rho_dep);
    E_field[gid] = e_base + e_dep;
}

// =============================================================================
// Compressible Ideal-Gas Dynamics (Eulerian grid update)
// =============================================================================
// Port of the correctness-first reference in `sensorium/kernels/gas_dynamics.py`,
// adapted to this project’s **dual-energy** grid semantics:
//
//   - grid carries (rho, mom, e_int) where e_int is INTERNAL energy density
//     (no kinetic energy term stored in the grid scalar channel).
//   - pressure uses ideal-gas closure with constant γ:
//         p = (γ - 1) * e_int
//
// Unlike total-energy formulations, the internal-energy equation contains a
// non-conservative pressure-work term. We therefore evolve:
//
//   ∂t rho  + ∇·(rho u)           = 0
//   ∂t mom  + ∇·(mom ⊗ u + p I)   = 0
//   ∂t e_int + ∇·(e_int u)        = - p (∇·u)  + ∇·(k ∇T)
//
// Numerics:
//   - Inviscid fluxes: Rusanov/LLF at faces (robust, diffusive).
//   - Time stepping: RK2 (Heun): U1 = U0 + dt*k1 ; U2 = U0 + 0.5*dt*(k1+k2)
//   - Spatial derivatives: second-order central differences via periodic indexing.
//
// Notes:
//   - This intentionally matches the periodic torus domain used elsewhere.
//   - Viscosity terms are not included yet (mu parameter reserved).
// =============================================================================

struct GasGridParams {
    uint32_t num_cells;   // gx * gy * gz
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float dx;
    float inv_dx;
    float inv_dx2;
    float dt;
    float gamma;
    float c_v;
    float rho_min;
    float p_min;
    float mu;        // reserved (viscosity) – not used yet
    float k_thermal; // thermal conductivity (constant)
};

struct U5 {
    float rho;
    float3 mom;
    float e_int;
};

struct F5 {
    float frho;
    float3 fmom;
    float fe_int;
};

inline uint idx3_periodic(uint x, uint y, uint z, uint gx, uint gy, uint gz) {
    // x-major, same layout as torch contiguous (gx,gy,gz) and (gx,gy,gz,3).
    return x * (gy * gz) + y * gz + z;
}

inline void ijk_from_linear(uint idx, uint gx, uint gy, uint gz, thread uint& x, thread uint& y, thread uint& z) {
    uint stride_x = gy * gz;
    uint stride_y = gz;
    x = idx / stride_x;
    uint rem = idx - x * stride_x;
    y = rem / stride_y;
    z = rem - y * stride_y;
}

inline uint wrap_minus_one(uint i, uint n) { return (i == 0u) ? (n - 1u) : (i - 1u); }
inline uint wrap_plus_one(uint i, uint n) { return (i + 1u == n) ? 0u : (i + 1u); }

inline U5 load_U5(
    device const float4* mom_rho,
    device const float* e_int,
    uint idx
) {
    U5 U;
    float4 mr = mom_rho[idx];
    U.mom = mr.xyz;
    U.rho = mr.w;
    U.e_int = e_int[idx];
    return U;
}

inline void store_U5(
    device float4* mom_rho,
    device float* e_int,
    uint idx,
    U5 U
) {
    mom_rho[idx] = float4(U.mom, U.rho);
    e_int[idx] = U.e_int;
}

// NOTE: `clamp_pos` was used for silent positivity floors.
// We keep it only for non-physics debug/utility code paths; physics kernels are fail-fast.
inline float clamp_pos(float x, float xmin) { return (x < xmin) ? xmin : x; }

inline void primitives_from_U(
    U5 U,
    float gamma,
    float c_v,
    float rho_min,
    float p_min,
    thread float& rho_safe,
    thread float3& u,
    thread float& p,
    thread float& T,
    thread float& c,
    thread float& speed
) {
    // FAIL-FAST: no clamping / projection.
    // Vacuum is a valid state: (rho=0, mom=0, e_int=0) with u=p=T=c=0.
    // Anything else outside the admissible set returns NaNs to poison the step.
    if (!(gamma > 1.0f) || !isfinite(gamma) || !(c_v > 0.0f) || !isfinite(c_v)) {
        float qn = qnan_f();
        rho_safe = qn;
        u = float3(qn);
        p = qn;
        T = qn;
        c = qn;
        speed = qn;
        return;
    }

    if (!isfinite(U.rho) || !isfinite(U.e_int) || !isfinite(U.mom.x) || !isfinite(U.mom.y) || !isfinite(U.mom.z)) {
        float qn = qnan_f();
        rho_safe = qn;
        u = float3(qn);
        p = qn;
        T = qn;
        c = qn;
        speed = qn;
        return;
    }

    // Numerical low-density envelope:
    // For very-low density cells we regularize primitive recovery to avoid division
    // blow-ups, without projecting or clamping the *conserved* state.
    //
    // Key idea: when |rho| is below a resolution-scale threshold, treat rho as
    // `rho_eps` for primitive *computation* (u, T, c). This keeps velocities and
    // temperatures bounded in cells that are effectively under-resolved.
    float rho_eps = max(rho_min, 0.0f);
    const float f32_eps = 1.1920929e-7f;
    float e_eps = 4.0f * rho_eps * f32_eps;
    // Max internal energy density allowed in the low-density envelope.
    // This prevents T = e_int/(rho c_v) from becoming astronomically large.
    const float e_spec_max = 10.0f; // ~O(1) temperature scale in sim units
    float e_int_max = e_spec_max * rho_eps;
    if (fabs(U.rho) <= rho_eps) {
        // Low-density: require bounded momentum and bounded internal energy density.
        // Allow tiny signed e_int noise around 0 (|e_int|<=e_eps).
        if (length(U.mom) > rho_eps) {
            float qn = qnan_f();
            rho_safe = qn;
            u = float3(qn);
            p = qn;
            T = qn;
            c = qn;
            speed = qn;
            return;
        }
        if (U.e_int < -e_eps || U.e_int > e_int_max) {
            float qn = qnan_f();
            rho_safe = qn;
            u = float3(qn);
            p = qn;
            T = qn;
            c = qn;
            speed = qn;
            return;
        }
        // Regularize primitive recovery using rho_eps (not U.rho).
        rho_safe = rho_eps;
        u = U.mom / rho_safe;
        float e_used = (U.e_int < 0.0f) ? 0.0f : U.e_int; // only affects primitives
        p = (gamma - 1.0f) * e_used;
        T = e_used / (rho_safe * c_v);
        c = sqrt((gamma * p) / rho_safe);
        speed = length(u) + c;
        return;
    }

    // Positive-density state.
    if (!(U.rho > 0.0f) || !(U.e_int >= 0.0f)) {
        float qn = qnan_f();
        rho_safe = qn;
        u = float3(qn);
        p = qn;
        T = qn;
        c = qn;
        speed = qn;
        return;
    }

    rho_safe = U.rho;
    u = U.mom / rho_safe;
    p = (gamma - 1.0f) * U.e_int;
    // (gamma>1 and e_int>=0) => p>=0
    T = U.e_int / (rho_safe * c_v);
    c = sqrt((gamma * p) / rho_safe);
    speed = length(u) + c;
}

inline F5 inviscid_flux_dir(uint dir, U5 U, float3 u, float p) {
    F5 F;
    float u_d = (dir == 0u) ? u.x : ((dir == 1u) ? u.y : u.z);
    // rho flux = rho * u_d = mom_d (exact for conserved momentum density)
    F.frho = (dir == 0u) ? U.mom.x : ((dir == 1u) ? U.mom.y : U.mom.z);
    // mom flux = mom * u_d + p * e_dir
    F.fmom = U.mom * u_d;
    if (dir == 0u) F.fmom.x += p;
    if (dir == 1u) F.fmom.y += p;
    if (dir == 2u) F.fmom.z += p;
    // internal-energy advective flux
    F.fe_int = U.e_int * u_d;
    return F;
}

inline F5 rusanov_flux(F5 FL, F5 FR, U5 UL, U5 UR, float smax) {
    // F = 0.5*(FL+FR) - 0.5*smax*(UR-UL)
    F5 F;
    float a = 0.5f;
    float d_rho = UR.rho - UL.rho;
    float3 d_mom = UR.mom - UL.mom;
    float d_e = UR.e_int - UL.e_int;
    F.frho = a * (FL.frho + FR.frho) - a * smax * d_rho;
    F.fmom = a * (FL.fmom + FR.fmom) - a * smax * d_mom;
    F.fe_int = a * (FL.fe_int + FR.fe_int) - a * smax * d_e;
    return F;
}

inline bool gas_diffusion_cfl_admissible(constant GasGridParams& p) {
    if (!(p.rho_min > 0.0f) || !(p.c_v > 0.0f) || !(p.k_thermal >= 0.0f) || !(p.dt > 0.0f)) {
        return false;
    }

    float diffusive_number = p.k_thermal * p.dt * p.inv_dx2 / (p.rho_min * p.c_v);
    const float von_neumann_limit = 1.0f / 6.0f;
    const float cfl_epsilon = 1.0e-5f;
    return diffusive_number <= von_neumann_limit + cfl_epsilon;
}

inline bool admissible_U5(
    thread const U5& U,
    float gamma,
    float rho_min,
    float p_min
) {
    // FAIL-FAST admissibility: do not modify state.
    // For rho>0 we require rho finite and positive, mom finite, e_int finite and >=0.
    // In the low-density envelope (|rho|<=rho_eps) we allow small/bounded e_int with
    // bounded momentum so primitives remain finite.
    (void)p_min; // no silent floors; this is not used for admissibility.
    if (!(gamma > 1.0f) || !isfinite(gamma)) return false;
    if (!isfinite(U.rho) || !isfinite(U.e_int) || !isfinite(U.mom.x) || !isfinite(U.mom.y) || !isfinite(U.mom.z)) return false;
    float rho_eps = max(rho_min, 0.0f);
    const float f32_eps = 1.1920929e-7f;
    float e_eps = 4.0f * rho_eps * f32_eps;
    const float e_spec_max = 10.0f;
    float e_int_max = e_spec_max * rho_eps;
    if (fabs(U.rho) <= rho_eps) {
        // Low-density: tolerate tiny signed rho and bounded momentum / bounded e_int.
        if (length(U.mom) > rho_eps) return false;
        if (U.e_int < -e_eps) return false;
        if (U.e_int > e_int_max) return false;
        return true;
    }
    if (!(U.rho > rho_eps)) return false;
    if (!(U.e_int >= 0.0f)) return false;
    return true;
}

struct GasPrim {
    float4 u_p;     // {ux, uy, uz, p}
    float4 thermo;  // {T, c, speed, rho_safe}
};

inline uint gas_cell_gid(
    constant GasGridParams& gas_params,
    uint3 threadgroup_pos,
    uint3 threadgroup_tid,
    uint3 threadgroup_dim
) {
    uint cell_z = threadgroup_pos.x * threadgroup_dim.x + threadgroup_tid.x;
    uint cell_y = threadgroup_pos.y * threadgroup_dim.y + threadgroup_tid.y;
    uint cell_x = threadgroup_pos.z * threadgroup_dim.z + threadgroup_tid.z;

    if (cell_x >= kGridX || cell_y >= kGridY || cell_z >= kGridZ) {
        return gas_params.num_cells;
    }

    return idx3_periodic(cell_x, cell_y, cell_z, kGridX, kGridY, kGridZ);
}

kernel void gas_compute_primitives(
    device const float4* mom_rho  [[buffer(0)]],
    device const float* e0        [[buffer(1)]],
    device GasPrim* prim          [[buffer(2)]],
    constant GasGridParams& p     [[buffer(3)]],
    uint3 threadgroup_pos [[threadgroup_position_in_grid]],
    uint3 threadgroup_tid [[thread_position_in_threadgroup]],
    uint3 threadgroup_dim [[threads_per_threadgroup]]
) {
    uint gid = gas_cell_gid(p, threadgroup_pos, threadgroup_tid, threadgroup_dim);
    if (gid >= p.num_cells) return;

    U5 U = load_U5(mom_rho, e0, gid);

    GasPrim pr;
    if (!admissible_U5(U, p.gamma, p.rho_min, p.p_min)) {
        float qn = qnan_f();
        pr.u_p = float4(qn);
        pr.thermo = float4(qn);
        prim[gid] = pr;
        return;
    }

    float rho_safe, p_c, T_c, c_c, sp_c;
    float3 u;
    primitives_from_U(U, p.gamma, p.c_v, p.rho_min, p.p_min,
                      rho_safe, u, p_c, T_c, c_c, sp_c);

    pr.u_p = float4(u, p_c);
    pr.thermo = float4(T_c, c_c, sp_c, rho_safe);
    prim[gid] = pr;
}

inline void gas_rhs_cell(
    device const float4* mom_rho0,
    device const float* e0,
    device const GasPrim* prim,
    constant GasGridParams& p,
    uint idx,
    thread float& drho,
    thread float3& dmom,
    thread float& de_int
) {
    float inv_dx = p.inv_dx;
    float inv_dx2 = p.inv_dx2;

    if (!gas_diffusion_cfl_admissible(p)) {
        float qn = qnan_f();
        drho = qn;
        dmom = float3(qn);
        de_int = qn;
        return;
    }

    uint x, y, z;
    ijk_from_linear(idx, kGridX, kGridY, kGridZ, x, y, z);

    uint xm = wrap_minus_one(x, kGridX), xp = wrap_plus_one(x, kGridX);
    uint ym = wrap_minus_one(y, kGridY), yp = wrap_plus_one(y, kGridY);
    uint zm = wrap_minus_one(z, kGridZ), zp = wrap_plus_one(z, kGridZ);

    uint idx_xm = idx3_periodic(xm, y, z, kGridX, kGridY, kGridZ);
    uint idx_xp = idx3_periodic(xp, y, z, kGridX, kGridY, kGridZ);
    uint idx_ym = idx3_periodic(x, ym, z, kGridX, kGridY, kGridZ);
    uint idx_yp = idx3_periodic(x, yp, z, kGridX, kGridY, kGridZ);
    uint idx_zm = idx3_periodic(x, y, zm, kGridX, kGridY, kGridZ);
    uint idx_zp = idx3_periodic(x, y, zp, kGridX, kGridY, kGridZ);

    U5 Uc  = load_U5(mom_rho0, e0, idx);
    U5 Uxm = load_U5(mom_rho0, e0, idx_xm);
    U5 Uxp = load_U5(mom_rho0, e0, idx_xp);
    U5 Uym = load_U5(mom_rho0, e0, idx_ym);
    U5 Uyp = load_U5(mom_rho0, e0, idx_yp);
    U5 Uzm = load_U5(mom_rho0, e0, idx_zm);
    U5 Uzp = load_U5(mom_rho0, e0, idx_zp);

    GasPrim Pc  = prim[idx];
    GasPrim Pxm = prim[idx_xm];
    GasPrim Pxp = prim[idx_xp];
    GasPrim Pym = prim[idx_ym];
    GasPrim Pyp = prim[idx_yp];
    GasPrim Pzm = prim[idx_zm];
    GasPrim Pzp = prim[idx_zp];

    if (!isfinite(Pc.thermo.z)  || !isfinite(Pxm.thermo.z) || !isfinite(Pxp.thermo.z) ||
        !isfinite(Pym.thermo.z) || !isfinite(Pyp.thermo.z) || !isfinite(Pzm.thermo.z) ||
        !isfinite(Pzp.thermo.z)) {
        float qn = qnan_f();
        drho = qn; dmom = float3(qn); de_int = qn;
        return;
    }

    float3 u_c  = Pc.u_p.xyz;
    float3 u_xm = Pxm.u_p.xyz;
    float3 u_xp = Pxp.u_p.xyz;
    float3 u_ym = Pym.u_p.xyz;
    float3 u_yp = Pyp.u_p.xyz;
    float3 u_zm = Pzm.u_p.xyz;
    float3 u_zp = Pzp.u_p.xyz;

    float p_c  = Pc.u_p.w;
    float p_xm = Pxm.u_p.w;
    float p_xp = Pxp.u_p.w;
    float p_ym = Pym.u_p.w;
    float p_yp = Pyp.u_p.w;
    float p_zm = Pzm.u_p.w;
    float p_zp = Pzp.u_p.w;

    float speed_c  = Pc.thermo.z;
    float speed_xm = Pxm.thermo.z;
    float speed_xp = Pxp.thermo.z;
    float speed_ym = Pym.thermo.z;
    float speed_yp = Pyp.thermo.z;
    float speed_zm = Pzm.thermo.z;
    float speed_zp = Pzp.thermo.z;

    F5 Fx_m = rusanov_flux(
        inviscid_flux_dir(0u, Uxm, u_xm, p_xm),
        inviscid_flux_dir(0u, Uc,  u_c,  p_c),
        Uxm, Uc, max(speed_xm, speed_c));
    F5 Fx_p = rusanov_flux(
        inviscid_flux_dir(0u, Uc,  u_c,  p_c),
        inviscid_flux_dir(0u, Uxp, u_xp, p_xp),
        Uc, Uxp, max(speed_c, speed_xp));
    F5 Fy_m = rusanov_flux(
        inviscid_flux_dir(1u, Uym, u_ym, p_ym),
        inviscid_flux_dir(1u, Uc,  u_c,  p_c),
        Uym, Uc, max(speed_ym, speed_c));
    F5 Fy_p = rusanov_flux(
        inviscid_flux_dir(1u, Uc,  u_c,  p_c),
        inviscid_flux_dir(1u, Uyp, u_yp, p_yp),
        Uc, Uyp, max(speed_c, speed_yp));
    F5 Fz_m = rusanov_flux(
        inviscid_flux_dir(2u, Uzm, u_zm, p_zm),
        inviscid_flux_dir(2u, Uc,  u_c,  p_c),
        Uzm, Uc, max(speed_zm, speed_c));
    F5 Fz_p = rusanov_flux(
        inviscid_flux_dir(2u, Uc,  u_c,  p_c),
        inviscid_flux_dir(2u, Uzp, u_zp, p_zp),
        Uc, Uzp, max(speed_c, speed_zp));

    float div_frho = ((Fx_p.frho - Fx_m.frho) + (Fy_p.frho - Fy_m.frho) + (Fz_p.frho - Fz_m.frho)) * inv_dx;
    float3 div_fmom = ((Fx_p.fmom - Fx_m.fmom) + (Fy_p.fmom - Fy_m.fmom) + (Fz_p.fmom - Fz_m.fmom)) * inv_dx;
    float div_fe = ((Fx_p.fe_int - Fx_m.fe_int) + (Fy_p.fe_int - Fy_m.fe_int) + (Fz_p.fe_int - Fz_m.fe_int)) * inv_dx;

    drho = -div_frho;
    dmom = -div_fmom;

    float div_u = (u_xp.x - u_xm.x) * (0.5f * inv_dx)
                + (u_yp.y - u_ym.y) * (0.5f * inv_dx)
                + (u_zp.z - u_zm.z) * (0.5f * inv_dx);

    float lap_T = (Pxp.thermo.x + Pxm.thermo.x + Pyp.thermo.x + Pym.thermo.x + Pzp.thermo.x + Pzm.thermo.x - 6.0f * Pc.thermo.x) * inv_dx2;

    de_int = -div_fe + (-p_c * div_u) + (p.k_thermal * lap_T);
}

kernel void gas_rk2_stage1(
    device const float4* mom_rho0  [[buffer(0)]],
    device const float* e0         [[buffer(1)]],
    device const GasPrim* prim0    [[buffer(2)]],
    device float4* mom_rho1        [[buffer(3)]],
    device float* e1               [[buffer(4)]],
    constant GasGridParams& p      [[buffer(5)]],
    device atomic_uint* dbg_head   [[buffer(6)]],
    device uint* dbg_words         [[buffer(7)]],
    constant uint& dbg_cap         [[buffer(8)]],
    uint3 threadgroup_pos [[threadgroup_position_in_grid]],
    uint3 threadgroup_tid [[thread_position_in_threadgroup]],
    uint3 threadgroup_dim [[threads_per_threadgroup]]
) {
    uint gid = gas_cell_gid(p, threadgroup_pos, threadgroup_tid, threadgroup_dim);
    if (gid >= p.num_cells) return;

    float dr; float3 dm; float de;
    gas_rhs_cell(mom_rho0, e0, prim0, p, gid, dr, dm, de);
    if (!isfinite(dr) || !isfinite(dm.x) || !isfinite(dm.y) || !isfinite(dm.z) || !isfinite(de)) {
        U5 Uc_bad = load_U5(mom_rho0, e0, gid);
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x20u, gid, Uc_bad.rho, Uc_bad.e_int, Uc_bad.mom.x, Uc_bad.mom.y);
        float qn = qnan_f();
        mom_rho1[gid] = float4(qn);
        e1[gid] = qn;
        return;
    }

    U5 Uc = load_U5(mom_rho0, e0, gid);
    U5 U1;
    U1.rho = Uc.rho + p.dt * dr;
    U1.mom = Uc.mom + p.dt * dm;
    U1.e_int = Uc.e_int + p.dt * de;
    if (!admissible_U5(U1, p.gamma, p.rho_min, p.p_min)) {
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x12u, gid, Uc.rho, Uc.e_int, U1.rho, U1.e_int);
        float qn = qnan_f();
        mom_rho1[gid] = float4(qn);
        e1[gid] = qn;
        return;
    }

    store_U5(mom_rho1, e1, gid, U1);
}

kernel void gas_rk2_stage2(
    device const float4* mom_rho0  [[buffer(0)]],
    device const float* e0         [[buffer(1)]],
    device const float4* mom_rho1  [[buffer(2)]],
    device const float* e1         [[buffer(3)]],
    device const GasPrim* prim1    [[buffer(4)]],
    device float4* mom_rho_out     [[buffer(5)]],
    device float* e_out            [[buffer(6)]],
    constant GasGridParams& p      [[buffer(7)]],
    device atomic_uint* dbg_head   [[buffer(8)]],
    device uint* dbg_words         [[buffer(9)]],
    constant uint& dbg_cap         [[buffer(10)]],
    uint3 threadgroup_pos [[threadgroup_position_in_grid]],
    uint3 threadgroup_tid [[thread_position_in_threadgroup]],
    uint3 threadgroup_dim [[threads_per_threadgroup]]
) {
    uint gid = gas_cell_gid(p, threadgroup_pos, threadgroup_tid, threadgroup_dim);
    if (gid >= p.num_cells) return;

    float dr2; float3 dm2; float de2;
    gas_rhs_cell(mom_rho1, e1, prim1, p, gid, dr2, dm2, de2);
    if (!isfinite(dr2) || !isfinite(dm2.x) || !isfinite(dm2.y) || !isfinite(dm2.z) || !isfinite(de2)) {
        U5 Uc_bad = load_U5(mom_rho1, e1, gid);
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x21u, gid, Uc_bad.rho, Uc_bad.e_int, Uc_bad.mom.x, Uc_bad.mom.y);
        float qn = qnan_f();
        mom_rho_out[gid] = float4(qn);
        e_out[gid] = qn;
        return;
    }

    U5 Uc = load_U5(mom_rho0, e0, gid);
    U5 U1 = load_U5(mom_rho1, e1, gid);

    U5 U2;
    U2.rho = 0.5f * (Uc.rho + U1.rho + p.dt * dr2);
    U2.mom = 0.5f * (Uc.mom + U1.mom + p.dt * dm2);
    U2.e_int = 0.5f * (Uc.e_int + U1.e_int + p.dt * de2);
    if (!admissible_U5(U2, p.gamma, p.rho_min, p.p_min)) {
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x13u, gid, Uc.rho, Uc.e_int, U2.rho, U2.e_int);
        float qn = qnan_f();
        mom_rho_out[gid] = float4(qn);
        e_out[gid] = qn;
        return;
    }

    store_U5(mom_rho_out, e_out, gid, U2);
}

// -----------------------------------------------------------------------------
// PIC gather: grid (rho,mom,E) → particle (pos,vel,heat) update
// -----------------------------------------------------------------------------
// This is the performance-critical "gather" pairing for the sort-based scatter:
// gather conserved quantities via CIC weights, convert to primitives at particle
// locations, then advect particles by the gathered velocity.
kernel void pic_gather_update_particles(
    device const float* particle_pos_in   [[buffer(0)]],  // N * 3
    device const float* particle_mass     [[buffer(1)]],  // N
    device float* particle_pos_out        [[buffer(2)]],  // N * 3
    device float* particle_vel_out        [[buffer(3)]],  // N * 3
    device const float* particle_heat_in  [[buffer(4)]],  // N
    device float* particle_heat_out       [[buffer(5)]],  // N
    device const float4* mom_rho_field    [[buffer(6)]],  // gx * gy * gz
    device const float* E_field           [[buffer(7)]],  // gx * gy * gz
    device const float* gravity_potential [[buffer(8)]],  // gx * gy * gz (gravitational potential φ)
    constant PicGatherParams& p           [[buffer(9)]],
    device atomic_uint* dbg_head          [[buffer(10)]],
    device uint* dbg_words                [[buffer(11)]],
    constant uint& dbg_cap                [[buffer(12)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_particles) return;

    float3 pos = float3(
        particle_pos_in[gid * 3 + 0],
        particle_pos_in[gid * 3 + 1],
        particle_pos_in[gid * 3 + 2]
    );

    // CIC gather of conserved fields at particle location
    uint3 grid_dims = uint3(kGridX, kGridY, kGridZ);
    uint3 base_idx;
    float3 frac;
    trilinear_coords(pos, p.inv_grid_spacing, grid_dims, base_idx, frac);

    float wx0 = 1.0f - frac.x, wx1 = frac.x;
    float wy0 = 1.0f - frac.y, wy1 = frac.y;
    float wz0 = 1.0f - frac.z, wz1 = frac.z;

    float weights[8] = {
        wx0 * wy0 * wz0,
        wx0 * wy0 * wz1,
        wx0 * wy1 * wz0,
        wx0 * wy1 * wz1,
        wx1 * wy0 * wz0,
        wx1 * wy0 * wz1,
        wx1 * wy1 * wz0,
        wx1 * wy1 * wz1
    };

    uint x0 = base_idx.x, y0 = base_idx.y, z0 = base_idx.z;
    uint x1 = (x0 + 1) % kGridX;
    uint y1 = (y0 + 1) % kGridY;
    uint z1 = (z0 + 1) % kGridZ;

    uint stride_z = 1;
    uint stride_y = kGridZ;
    uint stride_x = kGridY * kGridZ;

    uint idxs[8] = {
        x0 * stride_x + y0 * stride_y + z0 * stride_z,
        x0 * stride_x + y0 * stride_y + z1 * stride_z,
        x0 * stride_x + y1 * stride_y + z0 * stride_z,
        x0 * stride_x + y1 * stride_y + z1 * stride_z,
        x1 * stride_x + y0 * stride_y + z0 * stride_z,
        x1 * stride_x + y0 * stride_y + z1 * stride_z,
        x1 * stride_x + y1 * stride_y + z0 * stride_z,
        x1 * stride_x + y1 * stride_y + z1 * stride_z
    };

    float rho = 0.0f;
    float3 mom = float3(0.0f);
    float E = 0.0f;
    for (uint c = 0; c < 8; c++) {
        float w = weights[c];
        uint idx = idxs[c];
        float4 mr = mom_rho_field[idx];
        rho += w * mr.w;
        mom += w * mr.xyz;
        E += w * E_field[idx];
    }

    // Convert to primitives at particle position (dual-energy; no subtraction).
    //
    // [CHOICE] vacuum semantics for PIC coupling (no numeric floors)
    // [FORMULA] if ρ <= 0: treat as vacuum: u := 0, T := 0
    // [REASON] true vacuum has no well-defined continuum temperature/velocity; attempting
    //          to interpret numerical solver floors as physics injects energy.
    bool vacuum = !(rho > 0.0f);
    float rho_safe = rho; // only valid when !vacuum
    float3 u = vacuum ? float3(0.0f) : (mom / rho_safe);
    // E_field holds internal energy density directly (u_int = ρ c_v T).
    float e_int_density = E;

    // TAG 0x01: baseline sample (first particle only)
    if (gid == 0u) {
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x01u, gid, rho, e_int_density, u.x, u.y);
    }
    // FAIL-FAST: gathered conserved fields must be finite and physically admissible.
    if (!isfinite(rho) || !(rho >= 0.0f) || !isfinite(e_int_density) || !(e_int_density >= 0.0f) ||
        !isfinite(mom.x) || !isfinite(mom.y) || !isfinite(mom.z)) {
        float qn = qnan_f();
        particle_pos_out[gid * 3 + 0] = qn;
        particle_pos_out[gid * 3 + 1] = qn;
        particle_pos_out[gid * 3 + 2] = qn;
        particle_vel_out[gid * 3 + 0] = qn;
        particle_vel_out[gid * 3 + 1] = qn;
        particle_vel_out[gid * 3 + 2] = qn;
        particle_heat_out[gid] = qn;
        return;
    }
    // TAG 0x02: vacuum gather (exact vacuum only: rho==0 & E==0 & mom==0)
    if (rho == 0.0f && e_int_density == 0.0f && mom.x == 0.0f && mom.y == 0.0f && mom.z == 0.0f) {
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x02u, gid, rho, e_int_density, 0.0f, 0.0f);
    }

    float cv = p.c_v;
    if (!(cv > 0.0f) || !isfinite(cv)) {
        // Fail loudly: invalid thermodynamic parameter.
        // TAG 0x03: invalid c_v
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x03u, gid, cv, rho, e_int_density, 0.0f);
        particle_heat_out[gid] = qnan_f();
        particle_pos_out[gid * 3 + 0] = qnan_f();
        particle_pos_out[gid * 3 + 1] = qnan_f();
        particle_pos_out[gid * 3 + 2] = qnan_f();
        particle_vel_out[gid * 3 + 0] = qnan_f();
        particle_vel_out[gid * 3 + 1] = qnan_f();
        particle_vel_out[gid * 3 + 2] = qnan_f();
        return;
    }

    // [CHOICE] temperature from internal energy (no clamps)
    // [FORMULA] e_int = ρ c_v T  =>  T = e_int / (ρ c_v)
    if (!vacuum) {
        // [INVARIANT] internal energy density must be finite and non-negative.
        if (!isfinite(e_int_density) || !(e_int_density >= 0.0f)) {
            // TAG 0x04: invalid internal energy gather
            dbg_log(dbg_head, dbg_words, dbg_cap, 0x04u, gid, rho, e_int_density, mom.x, mom.y);
            particle_heat_out[gid] = qnan_f();
            particle_pos_out[gid * 3 + 0] = qnan_f();
            particle_pos_out[gid * 3 + 1] = qnan_f();
            particle_pos_out[gid * 3 + 2] = qnan_f();
            particle_vel_out[gid * 3 + 0] = qnan_f();
            particle_vel_out[gid * 3 + 1] = qnan_f();
            particle_vel_out[gid * 3 + 2] = qnan_f();
            return;
        }
    }
    float T = vacuum ? 0.0f : (e_int_density / (rho_safe * cv));

    float mass = particle_mass[gid];
    // [CHOICE] vacuum heat: preserve intrinsic particle heat; do not zero on ρ==0 gather.
    // Continuum T and u are undefined in vacuum, but zeroing heat would silently destroy energy.
    float heat = vacuum ? particle_heat_in[gid] : (mass * cv * T);
    if (!isfinite(heat) || !isfinite(T)) {
        // TAG 0x05: non-finite temperature/heat result
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x05u, gid, mass, T, heat, rho);
    }

    // Sample gravity gradient smoothly at particle position.
    // [CHOICE] PIC gravity coupling via interpolated potential gradient
    // [FORMULA] a = -∇φ ; φ from periodic Poisson solve ∇²φ = 4πGρ
    // [REASON] avoids piecewise-constant “cell gravity” (jitter at cell boundaries)
    // [NOTES] Poisson solve already includes G, so we do NOT multiply by G here.
    float3 g_accel = float3(0.0f);
    if (p.gravity_enabled > 0.5f) {
        float3 grad_phi = sample_gradient_trilinear(
            gravity_potential,
            base_idx,
            frac,
            grid_dims,
            p.inv_grid_spacing
        );
        g_accel = -grad_phi;
    }

    // Apply gravity acceleration to velocity
    float3 u_with_gravity = u + g_accel * p.dt;

    // Advect particle with gravity-corrected velocity (PIC)
    float3 pos_next = pos + u_with_gravity * p.dt;
    float3 domain = float3(p.domain_x, p.domain_y, p.domain_z);
    pos_next = pos_next - floor(pos_next / domain) * domain;
    if (!isfinite(pos_next.x) || !isfinite(pos_next.y) || !isfinite(pos_next.z) ||
        !isfinite(u_with_gravity.x) || !isfinite(u_with_gravity.y) || !isfinite(u_with_gravity.z)) {
        // TAG 0x06: non-finite advection state
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x06u, gid, pos_next.x, pos_next.y, u_with_gravity.x, u_with_gravity.y);
    }

    particle_pos_out[gid * 3 + 0] = pos_next.x;
    particle_pos_out[gid * 3 + 1] = pos_next.y;
    particle_pos_out[gid * 3 + 2] = pos_next.z;

    particle_vel_out[gid * 3 + 0] = u_with_gravity.x;
    particle_vel_out[gid * 3 + 1] = u_with_gravity.y;
    particle_vel_out[gid * 3 + 2] = u_with_gravity.z;

    particle_heat_out[gid] = heat;
}

inline void atomic_max_uint_threadgroup(threadgroup atomic_uint* address, uint val) {
    atomic_fetch_max_explicit(address, val, memory_order_relaxed);
}

inline void atomic_max_uint_device(device atomic_uint* address, uint val) {
    atomic_fetch_max_explicit(address, val, memory_order_relaxed);
}

inline void atomic_min_uint_device(device atomic_uint* address, uint val) {
    atomic_fetch_min_explicit(address, val, memory_order_relaxed);
}

inline void atomic_max_offender_threadgroup(
    threadgroup atomic_uint* score_addr,
    threadgroup atomic_uint* idx_addr,
    float score,
    uint idx
) {
    uint new_score = float_to_ordered_u32(score);
    uint previous_score = atomic_fetch_max_explicit(score_addr, new_score, memory_order_relaxed);

    if (new_score > previous_score) {
        atomic_store_explicit(idx_addr, idx, memory_order_relaxed);
    }
}

inline void atomic_max_offender_device(
    device atomic_uint* score_addr,
    device atomic_uint* idx_addr,
    float score,
    uint idx
) {
    uint new_score = float_to_ordered_u32(score);
    uint previous_score = atomic_fetch_max_explicit(score_addr, new_score, memory_order_relaxed);

    if (new_score > previous_score) {
        atomic_store_explicit(idx_addr, idx, memory_order_relaxed);
    }
}

// -----------------------------------------------------------------------------
// Spectral Mode Coupling (Resonance Potential, Langevin Flow)
// -----------------------------------------------------------------------------
// This implements a conservative "resonance potential" view of the spectral layer.
//
// Definitions:
// - Particle phase oscillator:          z_i = A_i e^{iθ_i}
// - Spectral mode (global):             Ψ_k = R_k e^{iψ_k}
//
// Potential (conceptual):
//   U = - Σ_{i,k} T_{ik}(ω_i, ω_k, σ_k) * Re(z_i C_k*)
//       + (λ/2) Σ_k |C_k|^2
//
// where T_{ik} is a Gaussian tuning kernel in frequency space.
//
// Gradients:
// - Mode "force":      ∂(-U)/∂Ψ_k*  = Σ_i T_{ik} z_i  - λ Ψ_k
// - Phase "torque":    θ̇_i += Σ_k T_{ik} (A_i R_k) sin(ψ_k - θ_i)
//
// Langevin flow:
// - Add isotropic noise with temperature T to both mode updates and phase updates.

// -----------------------------------------------------------------------------
// Mode memory (anchored + crystallized)
// -----------------------------------------------------------------------------
// We model "chunks" as long-lived spectral modes (ω-bins) that store a small
// set of anchored particles and their relative phase offsets.
//
// This yields:
// - Storage: crystallized modes stop decaying and stop drifting in ω.
// - Top-down bias: crystallized modes pull anchored particles toward stored
//   phase offsets and can inject energy into anchored particles.
// - Idle compute: same kernels with a mode knob (consolidate/disambiguate/explore).
//
#define MODE_ANCHORS 8u

struct SpectralModeParams {
    // NOTE: Struct layout is stable ABI for host<->Metal.
    // Semantics (preferred vocabulary):
    // - "osc"  → particle source (phase oscillator) i
    // - "mode" → ω-lattice bin k
    // - "gate_width" → linewidth γ_k in the Lorentzian lineshape
    uint32_t num_osc;              // N (particles)
    uint32_t max_carriers;         // capacity of mode arrays (ABI name)
    uint32_t num_carriers;         // current active modes (<= max_carriers) (ABI name)
    float dt;
    float coupling_scale;          // phase torque scale
    float carrier_reg;             // λ (L2 regularization on |Ψ| to prevent blow-up)
    uint32_t rng_seed;             // updated each tick by host
    float conflict_threshold;      // coherence threshold to trigger split (high = stricter)
    float offender_weight_floor;   // ignore tiny weights
    float gate_width_min;
    float gate_width_max;
    float ema_alpha;               // smoothing for conflict
    float recenter_alpha;          // smoothing for ω_k recentering
    // --- Reporting / derived categories (no physics impact) ---
    uint32_t mode;                 // 0=online, 1=consolidate, 2=disambiguate, 3=explore
    float anchor_random_eps;       // ε-greedy anchor refresh probability
    float stable_amp_threshold;    // promote volatile->stable when |C| exceeds this
    float crystallize_amp_threshold;       // stable->crystallized when |C| exceeds this...
    float crystallize_conflict_threshold;  // ...and conflict below this for long enough
    uint32_t crystallize_age;      // consecutive stable frames required
    float crystallized_coupling_boost;     // extra coupling for crystallized modes
    float volatile_decay_mul;      // extra decay factor for volatile modes
    float stable_decay_mul;        // extra decay factor for stable modes
    float crystallized_decay_mul;  // extra decay factor for crystallized modes
    float topdown_phase_scale;     // extra phase pull for anchored particles
    float topdown_energy_scale;    // energy injection scale for crystallized modes
    float topdown_random_energy_eps; // random energy nudge probability (exploration)
    float repulsion_scale;         // mode ω repulsion (disambiguation)
    // --- Geometry → ω-field coupling (physics-derived) ---
    // Domain size for periodic minimum-image distances (torus).
    float domain_x;
    float domain_y;
    float domain_z;
    // Spatial coherence length σ_x (derived from thermal de Broglie wavelength).
    float spatial_sigma;
    // ---------------------------------------------------------------------
    // Homeostasis: "work" metabolic cost
    // ---------------------------------------------------------------------
    // [CHOICE] work budget from particle heat
    // [FORMULA] W_req = metabolic_rate * A_i * dt
    // [REASON] coupling to Ψ(ω) is "work"; heat pays for it, and GPE decay dissipates it.
    float metabolic_rate;
    uint32_t carrier_tile_base;
    uint32_t carrier_tile_count;
};

// Parameter bundle for the ω-field (coherence) layer.
typedef SpectralModeParams CoherenceModeParams;

// =============================================================================
// Complex math helpers (coherence field / GPE)
// =============================================================================
struct Complex {
    float r;
    float i;
};

inline Complex c_add(Complex a, Complex b) { return {a.r + b.r, a.i + b.i}; }
inline Complex c_sub(Complex a, Complex b) { return {a.r - b.r, a.i - b.i}; }
inline Complex c_mul(Complex a, Complex b) { return {a.r * b.r - a.i * b.i, a.r * b.i + a.i * b.r}; }
inline Complex c_scale(Complex a, float s) { return {a.r * s, a.i * s}; }
inline float c_mag2(Complex a) { return a.r * a.r + a.i * a.i; }
inline Complex c_exp_i(float theta) { return {cos(theta), sin(theta)}; }
inline Complex c_i_mul(Complex a) { return {-a.i, a.r}; } // i * (a.r + i a.i)

struct GPEParams {
    float dt;
    float hbar_eff;           // effective ħ in simulation units (must be > 0)
    float mass_eff;           // effective mass in ω-space (>=0); larger = slower tunneling
    float g_interaction;      // nonlinearity strength (can be <0 for self-attraction)
    float energy_decay;       // non-unitary damping (>=0) to allow settling
    float chemical_potential; // μ term (acts like population control / bias)
    float inv_domega2;        // 1/(Δω^2) for discrete Laplacian on a uniform ω lattice
    uint  anchors;            // anchor slots per ω-bin (must match MODE_ANCHORS)
    uint  rng_seed;           // for anchor refresh (deterministic)
    float anchor_eps;         // probability of random anchor refresh per step
};

inline float resonance_from_freq(float omega_i, float omega_k, float gate_width) {
    float d = omega_i - omega_k;
    // [CHOICE] resonance / linewidth kernel (physics-derived)
    // [FORMULA] R(Δω) = γ^2 / (Δω^2 + γ^2), with γ = gate_width > 0
    // [REASON] Lorentzian response from finite coherence time / damping (no Gaussian heuristic)
    // [NOTES] host enforces gate_width_min>0; kernel clamps gate_width into [min,max].
    if (!(gate_width > 0.0f)) return qnan_f();
    float g2 = gate_width * gate_width;
    return g2 / (d * d + g2);
}

inline float3 min_image_delta(float3 d, float3 domain) {
    // [CHOICE] minimum-image convention on a torus
    // [FORMULA] d := d - domain * round(d/domain)
    // [REASON] shortest displacement in periodic domain
    float3 q = d / domain;
    // round(x) = floor(x + 0.5) for x>=0; for negatives, use symmetric form
    float3 r = floor(q + 0.5f);
    return d - domain * r;
}


// -----------------------------------------------------------------------------
// Kernel: Project ω-modes into a spatial complex field Ψ(x)
// -----------------------------------------------------------------------------
//
// For each mode k, we splat its complex coefficient Ψ_k into position space at
// its spatial anchors. This gives us a coarse Ψ(x) that can guide particle motion.
//
// NOTE: This intentionally ignores any separate "carrier" notion — it is a direct
// position-space reconstruction from anchored coefficients.

kernel void project_modes_to_spatial_psi(
    device const float* mode_psi_real           [[buffer(0)]],   // num_modes
    device const float* mode_psi_imag           [[buffer(1)]],   // num_modes
    device const uint*  mode_anchor_idx         [[buffer(2)]],   // num_modes * anchors_per_mode
    device const float* mode_anchor_weight      [[buffer(3)]],   // num_modes * anchors_per_mode
    device const float* particle_pos            [[buffer(4)]],   // num_particles * 3
    device atomic_uint* psi_re_field            [[buffer(5)]],   // grid_numel (float bits)
    device atomic_uint* psi_im_field            [[buffer(6)]],   // grid_numel (float bits)
    constant ModeProjectParams& p               [[buffer(7)]],
    uint gid                                    [[thread_position_in_grid]]
) {
    uint total = p.num_modes * p.anchors_per_mode;
    if (gid >= total) return;

    uint mode = gid / p.anchors_per_mode;
    uint a    = gid - mode * p.anchors_per_mode;

    uint anchor = mode_anchor_idx[mode * p.anchors_per_mode + a];
    if (anchor == 0xFFFFFFFFu || anchor >= p.num_particles) return;

    float w = mode_anchor_weight[mode * p.anchors_per_mode + a];
    if (!(w > 0.0f)) return;

    float re = mode_psi_real[mode] * w;
    float im = mode_psi_imag[mode] * w;

    float3 pos = float3(
        particle_pos[anchor * 3 + 0],
        particle_pos[anchor * 3 + 1],
        particle_pos[anchor * 3 + 2]
    );

    // CIC splat onto the spatial grid (periodic).
    float3 g = pos * p.inv_grid_spacing;

    int ix0 = (int)floor(g.x);
    int iy0 = (int)floor(g.y);
    int iz0 = (int)floor(g.z);

    float fx = g.x - (float)ix0;
    float fy = g.y - (float)iy0;
    float fz = g.z - (float)iz0;

    int ix1 = ix0 + 1;
    int iy1 = iy0 + 1;
    int iz1 = iz0 + 1;

    ix0 = wrap_i32(ix0, (int)p.grid_x);
    iy0 = wrap_i32(iy0, (int)p.grid_y);
    iz0 = wrap_i32(iz0, (int)p.grid_z);

    ix1 = wrap_i32(ix1, (int)p.grid_x);
    iy1 = wrap_i32(iy1, (int)p.grid_y);
    iz1 = wrap_i32(iz1, (int)p.grid_z);

    float wx0 = 1.0f - fx;
    float wy0 = 1.0f - fy;
    float wz0 = 1.0f - fz;

    float wx1 = fx;
    float wy1 = fy;
    float wz1 = fz;

    uint gx = p.grid_x;
    uint gy = p.grid_y;

    uint i000 = (uint)ix0 + gx * ((uint)iy0 + gy * (uint)iz0);
    uint i100 = (uint)ix1 + gx * ((uint)iy0 + gy * (uint)iz0);
    uint i010 = (uint)ix0 + gx * ((uint)iy1 + gy * (uint)iz0);
    uint i110 = (uint)ix1 + gx * ((uint)iy1 + gy * (uint)iz0);
    uint i001 = (uint)ix0 + gx * ((uint)iy0 + gy * (uint)iz1);
    uint i101 = (uint)ix1 + gx * ((uint)iy0 + gy * (uint)iz1);
    uint i011 = (uint)ix0 + gx * ((uint)iy1 + gy * (uint)iz1);
    uint i111 = (uint)ix1 + gx * ((uint)iy1 + gy * (uint)iz1);

    float w000 = wx0 * wy0 * wz0;
    float w100 = wx1 * wy0 * wz0;
    float w010 = wx0 * wy1 * wz0;
    float w110 = wx1 * wy1 * wz0;
    float w001 = wx0 * wy0 * wz1;
    float w101 = wx1 * wy0 * wz1;
    float w011 = wx0 * wy1 * wz1;
    float w111 = wx1 * wy1 * wz1;

    atomic_add_float_device(&psi_re_field[i000], re * w000);
    atomic_add_float_device(&psi_im_field[i000], im * w000);

    atomic_add_float_device(&psi_re_field[i100], re * w100);
    atomic_add_float_device(&psi_im_field[i100], im * w100);

    atomic_add_float_device(&psi_re_field[i010], re * w010);
    atomic_add_float_device(&psi_im_field[i010], im * w010);

    atomic_add_float_device(&psi_re_field[i110], re * w110);
    atomic_add_float_device(&psi_im_field[i110], im * w110);

    atomic_add_float_device(&psi_re_field[i001], re * w001);
    atomic_add_float_device(&psi_im_field[i001], im * w001);

    atomic_add_float_device(&psi_re_field[i101], re * w101);
    atomic_add_float_device(&psi_im_field[i101], im * w101);

    atomic_add_float_device(&psi_re_field[i011], re * w011);
    atomic_add_float_device(&psi_im_field[i011], im * w011);

    atomic_add_float_device(&psi_re_field[i111], re * w111);
    atomic_add_float_device(&psi_im_field[i111], im * w111);

}

// -----------------------------------------------------------------------------
// Kernel: Pilot-wave gather — advect particles by the probability current
// -----------------------------------------------------------------------------
//
// Guidance velocity:
//   v = (ħ/m) * (Ψ_re ∇Ψ_im - Ψ_im ∇Ψ_re) / (|Ψ|^2 + ε)

kernel void pic_gather_update_particles_pilot_wave(
    device const float* particle_pos_in         [[buffer(0)]],   // N*3
    device const float* particle_mass           [[buffer(1)]],   // N
    device float*       particle_pos_out        [[buffer(2)]],   // N*3
    device float*       particle_vel_out        [[buffer(3)]],   // N*3
    device const float* psi_re_field            [[buffer(4)]],   // grid_numel
    device const float* psi_im_field            [[buffer(5)]],   // grid_numel
    constant PilotWaveParams& p                 [[buffer(6)]],
    uint gid                                    [[thread_position_in_grid]]
) {
    if (gid >= p.num_particles) return;

    float3 pos = float3(
        particle_pos_in[gid * 3 + 0],
        particle_pos_in[gid * 3 + 1],
        particle_pos_in[gid * 3 + 2]
    );

    uint3 grid_dims = uint3(p.grid_x, p.grid_y, p.grid_z);
    uint3 base_idx; float3 frac;
    trilinear_coords(pos, p.inv_grid_spacing, grid_dims, base_idx, frac);

    float4 re = sample_value_and_gradient_trilinear(
        psi_re_field, base_idx, frac, grid_dims, p.inv_grid_spacing);
    float4 im = sample_value_and_gradient_trilinear(
        psi_im_field, base_idx, frac, grid_dims, p.inv_grid_spacing);

    float psi_re = re.w;
    float psi_im = im.w;
    float3 grad_re = re.xyz;
    float3 grad_im = im.xyz;

    float denom = psi_re * psi_re + psi_im * psi_im + p.eps_denom;

    // Im(conj(Ψ) ∇Ψ) = Ψ_re ∇Ψ_im - Ψ_im ∇Ψ_re
    float3 current = (psi_re * grad_im - psi_im * grad_re) / denom;

    float m = particle_mass[gid];
    float inv_m = 1.0f / max(m, p.mass_min);

    float3 v = current * (p.hbar_eff * inv_m);

    float3 pos_new = pos + v * p.dt;
    float3 domain = float3(p.domain_x, p.domain_y, p.domain_z);
    pos_new = pos_new - floor(pos_new / domain) * domain;

    particle_pos_out[gid * 3 + 0] = pos_new.x;
    particle_pos_out[gid * 3 + 1] = pos_new.y;
    particle_pos_out[gid * 3 + 2] = pos_new.z;

    particle_vel_out[gid * 3 + 0] = v.x;
    particle_vel_out[gid * 3 + 1] = v.y;
    particle_vel_out[gid * 3 + 2] = v.z;
}

inline float spatial_overlap_from_anchors(
    float3 pos_i,
    device const float* particle_pos,          // N*3
    device const uint* anchor_idx,             // maxM * MODE_ANCHORS (UINT_MAX=empty)
    device const float* anchor_weight,         // maxM * MODE_ANCHORS
    uint mode_k,
    constant SpectralModeParams& p
) {
    // [CHOICE] real-space overlap integral proxy (Gaussian wavepackets)
    // [FORMULA] O = Σ_a w_a exp(-|Δx|^2/(4σ_x^2)) / Σ_a w_a
    // [REASON] overlap of localized wavefunctions (anchors represent carrier support)
    // [NOTES] σ_x is physics-derived from thermal de Broglie coherence length.
    float sigma = p.spatial_sigma;
    if (!(sigma > 0.0f)) return 0.0f;
    float inv_4s2 = 1.0f / (4.0f * sigma * sigma);
    float sum_w = 0.0f;
    float sum_ov = 0.0f;
    float3 domain = float3(p.domain_x, p.domain_y, p.domain_z);
    uint base = mode_k * MODE_ANCHORS;
    for (uint a = 0; a < MODE_ANCHORS; a++) {
        uint idx = anchor_idx[base + a];
        if (idx == 0xFFFFFFFFu) continue;
        float w = anchor_weight[base + a];
        if (!(w > 0.0f)) continue;
        float3 pos_a = float3(
            particle_pos[idx * 3 + 0],
            particle_pos[idx * 3 + 1],
            particle_pos[idx * 3 + 2]
        );
        float3 d = min_image_delta(pos_i - pos_a, domain);
        float r2 = dot(d, d);
        float r2_cutoff = 4.0f * sigma * sigma * kFp32ExpUnderflowX0;

        if (r2 >= r2_cutoff) {
            continue;
        }

        float ov = exp(-r2 * inv_4s2);
        sum_w += w;
        sum_ov += w * ov;
    }
    if (!(sum_w > 0.0f)) return 0.0f;
    return sum_ov / sum_w;
}

inline float spatial_overlap_from_anchor_pos(
    float3 pos_i,
    device const float* anchor_pos,            // maxM * MODE_ANCHORS * 3
    device const float* anchor_weight,         // maxM * MODE_ANCHORS
    uint mode_k,
    float3 domain,
    float spatial_sigma
) {
    float sigma = spatial_sigma;
    if (!(sigma > 0.0f)) return 0.0f;
    float inv_4s2 = 1.0f / (4.0f * sigma * sigma);
    float sum_w = 0.0f;
    float sum_ov = 0.0f;
    uint base = mode_k * MODE_ANCHORS;
    uint pos_base = base * 3u;
    for (uint a = 0; a < MODE_ANCHORS; a++) {
        float w = anchor_weight[base + a];
        if (!(w > 0.0f)) continue;
        float3 pos_a = float3(
            anchor_pos[pos_base + a * 3u + 0u],
            anchor_pos[pos_base + a * 3u + 1u],
            anchor_pos[pos_base + a * 3u + 2u]
        );
        float3 d = min_image_delta(pos_i - pos_a, domain);
        float r2 = dot(d, d);
        float r2_cutoff = 4.0f * sigma * sigma * kFp32ExpUnderflowX0;

        if (r2 >= r2_cutoff) {
            continue;
        }

        float ov = exp(-r2 * inv_4s2);
        sum_w += w;
        sum_ov += w * ov;
    }
    if (!(sum_w > 0.0f)) return 0.0f;
    return sum_ov / sum_w;
}

kernel void precompute_carrier_anchor_positions(
    device const float* particle_pos         [[buffer(0)]],
    device const uint* anchor_idx            [[buffer(1)]],
    device float* anchor_pos                 [[buffer(2)]],
    device float* anchor_weight              [[buffer(3)]],
    constant uint& num_carriers              [[buffer(4)]],
    uint gid [[thread_position_in_grid]]
) {
    uint total = num_carriers * MODE_ANCHORS;
    if (gid >= total) return;

    uint mode = gid / MODE_ANCHORS;
    uint anchor = gid - mode * MODE_ANCHORS;
    uint weight_slot = mode * MODE_ANCHORS + anchor;
    uint pos_slot = gid * 3u;
    uint idx = anchor_idx[weight_slot];

    if (idx == 0xFFFFFFFFu) {
        anchor_pos[pos_slot + 0u] = 0.0f;
        anchor_pos[pos_slot + 1u] = 0.0f;
        anchor_pos[pos_slot + 2u] = 0.0f;
        anchor_weight[weight_slot] = 0.0f;
        return;
    }

    anchor_pos[pos_slot + 0u] = particle_pos[idx * 3u + 0u];
    anchor_pos[pos_slot + 1u] = particle_pos[idx * 3u + 1u];
    anchor_pos[pos_slot + 2u] = particle_pos[idx * 3u + 2u];
}

// One-shot per-step coupling prep: heat payment, eff_amp, (zr, zi), and bin index.
kernel void coherence_prep_oscillator_coupling(
    device const float* osc_phase           [[buffer(0)]],
    device const float* osc_omega           [[buffer(1)]],
    device const float* osc_amp             [[buffer(2)]],
    device float* particle_heat             [[buffer(3)]],
    device float* osc_coupling_prep         [[buffer(4)]],  // N * 4: eff_amp, zr, zi, bin_f
    constant CoherenceModeParams& p         [[buffer(5)]],
    device const CoherenceBinParams* bin_p    [[buffer(6)]],
    constant uint& num_bins                 [[buffer(7)]],
    uint gid [[thread_position_in_grid]]
) {
    uint prep_base = gid * 4u;

    if (gid >= p.num_osc) {
        return;
    }

    float amp_i = osc_amp[gid];
    float phi_i = osc_phase[gid];
    float omega_i = osc_omega[gid];
    float Q = particle_heat[gid];
    float energy_i = amp_i * amp_i;
    float work_required = p.metabolic_rate * energy_i * p.dt;
    float coupling_factor = 1.0f;
    float work_done = 0.0f;

    if (work_required > 1e-8f) {
        if (Q >= work_required) {
            work_done = work_required;
            coupling_factor = 1.0f;
        } else {
            work_done = Q;
            coupling_factor = Q / work_required;
        }
    }

    particle_heat[gid] = Q - work_done;

    float eff_amp = amp_i * sqrt(max(0.0f, coupling_factor));
    float zr = eff_amp * cos(phi_i);
    float zi = eff_amp * sin(phi_i);
    float bin_f = 0.0f;

    if (num_bins > 0u) {
        float fbin = (omega_i - bin_p[0].omega_min) * bin_p[0].inv_bin_width;

        if (isfinite(fbin)) {
            bin_f = floor(fbin);
        } else {
            bin_f = -1.0e9f;
        }
    }

    osc_coupling_prep[prep_base + 0u] = eff_amp;
    osc_coupling_prep[prep_base + 1u] = zr;
    osc_coupling_prep[prep_base + 2u] = zi;
    osc_coupling_prep[prep_base + 3u] = bin_f;
}

// -----------------------------------------------------------------------------
// Kernel: Parallel Force Accumulation (Oscillator-Centric, Threadgroup Reduction)
// -----------------------------------------------------------------------------
// At scale (55M+ oscillators), direct global atomics cause severe contention.
// This version uses threadgroup-local accumulators:
// 1. Each threadgroup maintains local carrier accumulators in shared memory
// 2. Threads accumulate to threadgroup memory (fast local atomics)
// 3. After barrier, one flush to global per carrier per threadgroup
//
// Threadgroup memory: num_carriers * 8 atomic_uint fields (32 bytes each).
// Host sets threadgroup memory length to num_carriers * 32; capacity is capped at
// min(maxThreadgroupMemoryLength / 32, maxThreadsPerThreadgroup, 1024).

constant uint kMaxCarriersForTG = 256u;

struct CarrierAccumulators {
    atomic_float force_r;
    atomic_float force_i;
    atomic_float w_sum;
    atomic_float w_omega_sum;
    atomic_float w_omega2_sum;
    atomic_float w_amp_sum;
    atomic_uint offender_score;
    atomic_uint offender_idx;
};

kernel void clear_carrier_accums(
    device CarrierAccumulators* accums [[buffer(0)]],
    constant uint& num_carriers        [[buffer(1)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= num_carriers) return;
    device CarrierAccumulators& accum = accums[gid];
    atomic_store_explicit(&accum.force_r, 0.0f, memory_order_relaxed);
    atomic_store_explicit(&accum.force_i, 0.0f, memory_order_relaxed);
    atomic_store_explicit(&accum.w_sum, 0.0f, memory_order_relaxed);
    atomic_store_explicit(&accum.w_omega_sum, 0.0f, memory_order_relaxed);
    atomic_store_explicit(&accum.w_omega2_sum, 0.0f, memory_order_relaxed);
    atomic_store_explicit(&accum.w_amp_sum, 0.0f, memory_order_relaxed);
    atomic_store_explicit(&accum.offender_score, 0u, memory_order_relaxed);
    atomic_store_explicit(&accum.offender_idx, 0xFFFFFFFFu, memory_order_relaxed);
}

struct TGCarrierAccum {
    atomic_uint force_r;
    atomic_uint force_i;
    atomic_uint w_sum;
    atomic_uint w_omega_sum;
    atomic_uint w_omega2_sum;
    atomic_uint w_amp_sum;
    atomic_uint offender_score;
    atomic_uint offender_idx;
};

kernel void coherence_accumulate_forces(
    // Oscillator state
    device const float* osc_omega           [[buffer(0)]],  // N
    // Geometric state (for overlap integrals)
    device const float* particle_pos        [[buffer(1)]],  // N * 3
    // Mode state (read-only)
    device const float* carrier_omega       [[buffer(2)]],  // maxM
    device const float* carrier_gate_width  [[buffer(3)]],  // maxM
    device const float* carrier_anchor_w    [[buffer(4)]],  // maxM * MODE_ANCHORS
    // Output accumulators
    device CarrierAccumulators* accums      [[buffer(5)]],  // maxM
    // Parameters
    constant CoherenceModeParams& p         [[buffer(6)]],
    device const uint* num_carriers_in      [[buffer(7)]], // (1,) uint32/int32
    // Sparse binning inputs
    device const uint* bin_starts           [[buffer(8)]],  // num_bins + 1
    device const uint* carrier_binned_idx   [[buffer(9)]], // maxM (indices in [0,num_carriers))
    constant uint& num_bins                 [[buffer(10)]],
    device const float* carrier_anchor_pos [[buffer(11)]], // maxM * MODE_ANCHORS * 3
    device const float* osc_coupling_prep   [[buffer(12)]], // N * 4: eff_amp, zr, zi, bin_f
    uint gid [[thread_position_in_grid]],
    uint tid [[thread_index_in_threadgroup]],
    uint tg_size [[threads_per_threadgroup]],
    // Threadgroup memory for local accumulation
    threadgroup TGCarrierAccum* tg_accums   [[threadgroup(0)]]  // kMaxCarriersForTG
) {
    uint num_carriers = (num_carriers_in != nullptr) ? num_carriers_in[0] : 0u;
    uint tile_base = p.carrier_tile_base;
    uint tile_count = min(p.carrier_tile_count, kMaxCarriersForTG);
    if (tile_count == 0u || tile_base >= num_carriers) {
        return;
    }
    if (tile_base + tile_count > num_carriers) {
        tile_count = num_carriers - tile_base;
    }

    float3 domain = float3(p.domain_x, p.domain_y, p.domain_z);

    for (uint local_k = tid; local_k < tile_count; local_k += tg_size) {
        uint zero_bits = as_type<uint>(0.0f);
        atomic_store_explicit(&tg_accums[local_k].force_r, zero_bits, memory_order_relaxed);
        atomic_store_explicit(&tg_accums[local_k].force_i, zero_bits, memory_order_relaxed);
        atomic_store_explicit(&tg_accums[local_k].w_sum, zero_bits, memory_order_relaxed);
        atomic_store_explicit(&tg_accums[local_k].w_omega_sum, zero_bits, memory_order_relaxed);
        atomic_store_explicit(&tg_accums[local_k].w_omega2_sum, zero_bits, memory_order_relaxed);
        atomic_store_explicit(&tg_accums[local_k].w_amp_sum, zero_bits, memory_order_relaxed);
        atomic_store_explicit(&tg_accums[local_k].offender_score, 0u, memory_order_relaxed);
        atomic_store_explicit(&tg_accums[local_k].offender_idx, 0xFFFFFFFFu, memory_order_relaxed);
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);

    // Phase 2: Accumulate to threadgroup memory
    if (gid < p.num_osc && num_carriers > 0u && num_bins > 0u) {
        uint prep_base = gid * 4u;
        float eff_amp = osc_coupling_prep[prep_base + 0u];
        float zr = osc_coupling_prep[prep_base + 1u];
        float zi = osc_coupling_prep[prep_base + 2u];
        int bin_i = (int)osc_coupling_prep[prep_base + 3u];

        if (eff_amp <= p.offender_weight_floor) {
            eff_amp = 0.0f;
        }

        float omega_i = osc_omega[gid];
        float3 pos_i = float3(
            particle_pos[gid * 3 + 0],
            particle_pos[gid * 3 + 1],
            particle_pos[gid * 3 + 2]
        );

        const int rad = 2;
        int b0 = bin_i - rad;
        int b1 = bin_i + rad;

        if (eff_amp > p.offender_weight_floor) {
            for (int b = b0; b <= b1; b++) {
                if (b < 0 || b >= (int)num_bins) continue;
                uint start = bin_starts[(uint)b];
                uint end = bin_starts[(uint)b + 1u];
                for (uint j = start; j < end; j++) {
                    uint k = carrier_binned_idx[j];
                    if (k < tile_base || k >= tile_base + tile_count) continue;

                    float omega_k = carrier_omega[k];
                    float gate_w = carrier_gate_width[k];
                    float r = resonance_from_freq(omega_i, omega_k, gate_w);

                    if (r * eff_amp <= p.offender_weight_floor) continue;

                    float s = spatial_overlap_from_anchor_pos(
                        pos_i, carrier_anchor_pos, carrier_anchor_w, k, domain, p.spatial_sigma);
                    float w = (r * s) * eff_amp;

                    if (w <= p.offender_weight_floor) continue;

                    uint local_k = k - tile_base;
                    threadgroup TGCarrierAccum& tg_acc = tg_accums[local_k];
                    atomic_add_float_threadgroup(&tg_acc.force_r, w * zr);
                    atomic_add_float_threadgroup(&tg_acc.force_i, w * zi);
                    atomic_add_float_threadgroup(&tg_acc.w_sum, w);
                    atomic_add_float_threadgroup(&tg_acc.w_omega_sum, w * omega_i);
                    atomic_add_float_threadgroup(&tg_acc.w_omega2_sum, w * omega_i * omega_i);
                    atomic_add_float_threadgroup(&tg_acc.w_amp_sum, w * eff_amp);

                    atomic_max_offender_threadgroup(&tg_acc.offender_score, &tg_acc.offender_idx, w, gid);
                }
            }
        }
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);

    // Phase 3: Flush threadgroup accumulators to global (one atomic per carrier per threadgroup)
    for (uint local_k = tid; local_k < tile_count; local_k += tg_size) {
        uint k = tile_base + local_k;
        threadgroup TGCarrierAccum& tg_acc = tg_accums[local_k];
        device CarrierAccumulators& g_acc = accums[k];

        float fr = as_type<float>(atomic_load_explicit(&tg_acc.force_r, memory_order_relaxed));
        float fi = as_type<float>(atomic_load_explicit(&tg_acc.force_i, memory_order_relaxed));
        float ws = as_type<float>(atomic_load_explicit(&tg_acc.w_sum, memory_order_relaxed));
        float wos = as_type<float>(atomic_load_explicit(&tg_acc.w_omega_sum, memory_order_relaxed));
        float wo2s = as_type<float>(atomic_load_explicit(&tg_acc.w_omega2_sum, memory_order_relaxed));
        float was = as_type<float>(atomic_load_explicit(&tg_acc.w_amp_sum, memory_order_relaxed));

        atomic_store_explicit(&g_acc.force_r, fr + atomic_load_explicit(&g_acc.force_r, memory_order_relaxed), memory_order_relaxed);
        atomic_store_explicit(&g_acc.force_i, fi + atomic_load_explicit(&g_acc.force_i, memory_order_relaxed), memory_order_relaxed);
        atomic_store_explicit(&g_acc.w_sum, ws + atomic_load_explicit(&g_acc.w_sum, memory_order_relaxed), memory_order_relaxed);
        atomic_store_explicit(&g_acc.w_omega_sum, wos + atomic_load_explicit(&g_acc.w_omega_sum, memory_order_relaxed), memory_order_relaxed);
        atomic_store_explicit(&g_acc.w_omega2_sum, wo2s + atomic_load_explicit(&g_acc.w_omega2_sum, memory_order_relaxed), memory_order_relaxed);
        atomic_store_explicit(&g_acc.w_amp_sum, was + atomic_load_explicit(&g_acc.w_amp_sum, memory_order_relaxed), memory_order_relaxed);

        uint tg_off_score_u = atomic_load_explicit(&tg_acc.offender_score, memory_order_relaxed);
        uint tg_off_idx = atomic_load_explicit(&tg_acc.offender_idx, memory_order_relaxed);
        float tg_off_score = ordered_u32_to_float(tg_off_score_u);
        atomic_max_offender_device(&g_acc.offender_score, &g_acc.offender_idx, tg_off_score, tg_off_idx);
    }
}

// =============================================================================
// Quantum Coherence Layer (dissipative Gross–Pitaevskii-style update)
// =============================================================================
// Evolves a complex coherence field Ψ(ω_k) stored in (mode_real, mode_imag).
//
// This replaces conflict-driven splitting with continuous field dynamics:
// - Potential term from observations (here: -w_sum)
// - Nonlinear self-interaction g|Ψ|^2
// - Kinetic/tunneling via a 1D Laplacian on the ω lattice
// - Optional dissipation for settling
//
kernel void coherence_gpe_step(
    // Oscillator state (for anchor refresh)
    device const float* osc_phase           [[buffer(0)]],  // N
    device const float* osc_omega           [[buffer(1)]],  // N
    device const float* osc_amp             [[buffer(2)]],  // N
    // Coherence field state (in/out): Ψ = real + i imag
    device float* mode_real                 [[buffer(3)]],  // maxM
    device float* mode_imag                 [[buffer(4)]],  // maxM
    // ω lattice + coupling width (read-only)
    device const float* mode_omega          [[buffer(5)]],  // maxM
    device const float* mode_gate_width     [[buffer(6)]],  // maxM
    // Anchors (in/out) used for spatial overlap approximation
    device uint* mode_anchor_idx            [[buffer(7)]],  // maxM * MODE_ANCHORS (UINT_MAX=empty)
    device float* mode_anchor_weight        [[buffer(8)]],  // maxM * MODE_ANCHORS
    // Pre-accumulated observations / support
    device CarrierAccumulators* accums      [[buffer(9)]], // maxM
    device const uint* num_modes_in         [[buffer(10)]], // (1,) snapshot
    // Geometric state (for overlap integrals)
    device const float* particle_pos        [[buffer(11)]], // N * 3
    // Shared coherence parameters (overlap, bounds, domain sizes)
    constant CoherenceModeParams& p         [[buffer(12)]],
    // GPE parameters
    constant GPEParams& gp                  [[buffer(13)]],
    uint gid [[thread_position_in_grid]]
) {
    uint current = (num_modes_in != nullptr) ? num_modes_in[0] : 0u;
    if (current > p.max_carriers) {
        // Fail loudly rather than silently clamping (host/kernel mismatch).
        if (gid == 0u && p.max_carriers > 0u) {
            mode_real[0] = qnan_f();
            mode_imag[0] = qnan_f();
        }
        return;
    }
    if (gid >= current) return;

    // --- load Ψ_k ---
    Complex psi = {mode_real[gid], mode_imag[gid]};

    // --- local potential + drive from observations ---
    device CarrierAccumulators& acc = accums[gid];
    float w_sum = atomic_load_explicit(&acc.w_sum, memory_order_relaxed);
    float V_ext = -w_sum;

    // Coherent drive term from oscillators:
    // - acc.force_r/i is the weighted sum of oscillator phasors in this ω-bin
    // - acc.w_amp_sum is the corresponding weighted amplitude mass (normalizer)
    //
    // Without an explicit source term, Ψ initialized at 0 stays 0 forever under
    // pure unitary rotation + kinetic terms. This drive is the mechanism by which
    // oscillator superposition writes into the coherence field.
    float fr = atomic_load_explicit(&acc.force_r, memory_order_relaxed);
    float fi = atomic_load_explicit(&acc.force_i, memory_order_relaxed);
    float was = atomic_load_explicit(&acc.w_amp_sum, memory_order_relaxed);
    float denom_drive = (was > p.offender_weight_floor) ? was : 0.0f;
    Complex drive = {0.0f, 0.0f};
    if (denom_drive > 0.0f && isfinite(fr) && isfinite(fi)) {
        drive = c_scale((Complex){fr, fi}, 1.0f / denom_drive);
    }

    // --- Strang-style split-step (local potential/nonlinear ↔ kinetic) ---
    //
    // We treat this as an open system (observations + optional dissipation), but we
    // still want phase evolution to be stable enough for interference to persist.
    // The symmetric split reduces phase drift versus a single forward-Euler blend.
    //
    // 1) half-step potential/nonlinear rotation at k
    // 2) full-step kinetic/tunneling using half-rotated neighbors
    // 3) half-step potential/nonlinear rotation at k (recompute density after kinetic)
    float hbar = gp.hbar_eff;
    if (!(hbar > 0.0f)) {
        mode_real[gid] = qnan_f();
        mode_imag[gid] = qnan_f();
        return;
    }
    float half_dt = 0.5f * gp.dt;

    // half-step at k
    {
        float density = c_mag2(psi);
        float H_local = V_ext + (gp.g_interaction * density) - gp.chemical_potential;
        float theta = -(H_local * half_dt) / hbar;
        theta = clamp(theta, -M_PI_F, M_PI_F);
        psi = c_mul(psi, c_exp_i(theta));
    }

    // kinetic/tunneling (1D Laplacian on ω lattice index space)
    if (gp.mass_eff > 0.0f && gp.inv_domega2 > 0.0f) {
        uint left = (gid > 0u) ? (gid - 1u) : 0u;
        uint right = (gid + 1u < current) ? (gid + 1u) : (current - 1u);

        // Load neighbors and apply the same half-step potential rotation locally
        // so the Laplacian is consistent with the split-step ordering.
        Complex psi_l = {mode_real[left], mode_imag[left]};
        Complex psi_r = {mode_real[right], mode_imag[right]};

        device CarrierAccumulators& acc_l = accums[left];
        float w_sum_l = atomic_load_explicit(&acc_l.w_sum, memory_order_relaxed);
        float V_ext_l = -w_sum_l;
        float dens_l = c_mag2(psi_l);
        float H_l = V_ext_l + (gp.g_interaction * dens_l) - gp.chemical_potential;
        float theta_l = -(H_l * half_dt) / hbar;
        theta_l = clamp(theta_l, -M_PI_F, M_PI_F);
        psi_l = c_mul(psi_l, c_exp_i(theta_l));

        device CarrierAccumulators& acc_r = accums[right];
        float w_sum_r = atomic_load_explicit(&acc_r.w_sum, memory_order_relaxed);
        float V_ext_r = -w_sum_r;
        float dens_r = c_mag2(psi_r);
        float H_r = V_ext_r + (gp.g_interaction * dens_r) - gp.chemical_potential;
        float theta_r = -(H_r * half_dt) / hbar;
        theta_r = clamp(theta_r, -M_PI_F, M_PI_F);
        psi_r = c_mul(psi_r, c_exp_i(theta_r));

        Complex lap = c_add(c_sub(psi_l, c_scale(psi, 2.0f)), psi_r); // ψ_{k-1} - 2ψ_k + ψ_{k+1}
        // i * (ħ / 2m) ∇²ψ
        float kin = (hbar * gp.dt) / (2.0f * gp.mass_eff);
        psi = c_add(psi, c_scale(c_i_mul(lap), kin * gp.inv_domega2));
    }

    // second half-step at k (recompute density after kinetic)
    {
        float density = c_mag2(psi);
        float H_local = V_ext + (gp.g_interaction * density) - gp.chemical_potential;
        float theta = -(H_local * half_dt) / hbar;
        theta = clamp(theta, -M_PI_F, M_PI_F);
        psi = c_mul(psi, c_exp_i(theta));
    }

    // Open-system terms (explicit, no hidden clamps):
    // - linear damping (energy_decay): prevents unbounded growth under sustained drive
    // - additive drive from oscillator superposition
    if (gp.energy_decay > 0.0f) {
        float damp = exp(-gp.energy_decay * gp.dt);
        psi = c_scale(psi, damp);
    }
    psi = c_add(psi, c_scale(drive, gp.dt));

    // --- write back Ψ_k ---
    mode_real[gid] = psi.r;
    mode_imag[gid] = psi.i;
}

kernel void coherence_update_oscillator_phases(
    device float* particle_phase               [[buffer(0)]],  // N (in/out)
    device const float* particle_omega         [[buffer(1)]],  // N
    device const float* particle_amp           [[buffer(2)]],  // N
    device const float* mode_real              [[buffer(3)]],  // maxM
    device const float* mode_imag              [[buffer(4)]],  // maxM
    device const float* mode_omega             [[buffer(5)]],  // maxM
    device const float* mode_gate_width        [[buffer(6)]],  // maxM
    device const float* mode_anchor_weight     [[buffer(7)]],  // maxM * MODE_ANCHORS
    device const uint* num_carriers_in    [[buffer(8)]], // (1,) uint32/int32 snapshot
    constant CoherenceModeParams& p      [[buffer(9)]],
    // Sparse binning inputs
    device const uint* bin_starts         [[buffer(10)]],  // num_bins + 1
    device const uint* carrier_binned_idx [[buffer(11)]],  // maxM
    device const CoherenceBinParams* bin_p [[buffer(12)]],  // (1,)
    constant uint& num_bins               [[buffer(13)]],
    device const float* particle_pos      [[buffer(14)]],  // N * 3
    device const float* mode_anchor_pos   [[buffer(15)]],  // maxM * MODE_ANCHORS * 3
    device const float* osc_coupling_prep [[buffer(16)]],  // N * 4: eff_amp, zr, zi, bin_f
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_osc) return;
    uint num_carriers = (num_carriers_in != nullptr) ? num_carriers_in[0] : 0u;
    if (num_carriers > p.max_carriers) {
        particle_phase[gid] = qnan_f();
        return;
    }

    float phi = particle_phase[gid];
    float omega_i = particle_omega[gid];
    float amp_i = particle_amp[gid];
    float cphi = 0.0f;
    float sphi = sincos(phi, cphi);
    float3 pos_i = float3(
        particle_pos[gid * 3 + 0],
        particle_pos[gid * 3 + 1],
        particle_pos[gid * 3 + 2]
    );
    float3 domain = float3(p.domain_x, p.domain_y, p.domain_z);

    // Torque from resonance potential:
    //   θ̇_i += Σ_k T_ik (A_i R_k) sin(ψ_k - θ_i)
    // with R*sin(ψ-φ) = ci*cos(φ) - cr*sin(φ) when cr=R*cos(ψ), ci=R*sin(ψ).
    float torque = 0.0f;
    const int rad = 2;
    if (num_carriers > 0u && num_bins > 0u) {
        int bin_i = (int)osc_coupling_prep[gid * 4u + 3u];
        int b0 = bin_i - rad;
        int b1 = bin_i + rad;
        for (int b = b0; b <= b1; b++) {
            if (b < 0 || b >= (int)num_bins) continue;
            uint start = bin_starts[(uint)b];
            uint end = bin_starts[(uint)b + 1u];
            for (uint jj = start; jj < end; jj++) {
                uint k = carrier_binned_idx[jj];
                if (k >= num_carriers) continue;

                float cr = mode_real[k];
                float ci = mode_imag[k];

                if (cr == 0.0f && ci == 0.0f) continue;

                float omega_k = mode_omega[k];
                float gate_w = mode_gate_width[k];
                float r = resonance_from_freq(omega_i, omega_k, gate_w);
                float s = spatial_overlap_from_anchor_pos(
                    pos_i, mode_anchor_pos, mode_anchor_weight, k, domain, p.spatial_sigma);
                float t = r * s;
                torque += t * amp_i * (ci * cphi - cr * sphi);
            }
        }
    }

    float dphi = omega_i + p.coupling_scale * torque;
    phi += dphi * p.dt;

    // Wrap phase to [0, 2π)
    phi = phi - 2.0f * M_PI_F * floor(phi / (2.0f * M_PI_F));
    particle_phase[gid] = phi;
}

// =============================================================================
// Particle Generation Kernels
// =============================================================================
// Move synthetic data generation patterns to GPU for faster file injection.

struct ParticleGenParams {
    uint32_t num_particles;
    float grid_x;
    float grid_y;
    float grid_z;
    float energy_scale;
    uint32_t pattern;  // 0=cluster, 1=line, 2=sphere, 3=random, 4=grid
    float center_x;
    float center_y;
    float center_z;
    float spread;      // Cluster spread or sphere radius
    float dir_x;       // Line direction
    float dir_y;
    float dir_z;
};

// -----------------------------------------------------------------------------
// Kernel: Generate particle positions based on pattern
// -----------------------------------------------------------------------------

kernel void generate_particle_positions(
    device float* positions           [[buffer(0)]],  // N * 3
    device const float* random_vals   [[buffer(1)]],  // N * 3 (pre-generated uniform [0,1])
    constant ParticleGenParams& p     [[buffer(2)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_particles) return;
    
    float3 center = float3(p.center_x, p.center_y, p.center_z);
    float3 r = float3(
        random_vals[gid * 3 + 0],
        random_vals[gid * 3 + 1],
        random_vals[gid * 3 + 2]
    );
    
    float3 pos;
    
    if (p.pattern == 0) {
        // Cluster: Gaussian around center
        // Convert uniform to Gaussian using Box-Muller (approximate)
        float3 gauss = (r - 0.5f) * 2.0f * 2.0f;  // Rough approximation
        pos = center + gauss * p.spread;
    }
    else if (p.pattern == 1) {
        // Line: along direction from start
        float t = float(gid) / float(p.num_particles) * p.spread;
        float3 dir = float3(p.dir_x, p.dir_y, p.dir_z);
        pos = center + dir * t + (r - 0.5f) * 0.5f;
    }
    else if (p.pattern == 2) {
        // Sphere: points on shell
        float theta = r.x * 2.0f * M_PI_F;
        float phi = acos(2.0f * r.y - 1.0f);
        float x = sin(phi) * cos(theta);
        float y = sin(phi) * sin(theta);
        float z = cos(phi);
        pos = center + float3(x, y, z) * p.spread;
    }
    else if (p.pattern == 4) {
        // Grid: regular lattice
        uint side = uint(pow(float(p.num_particles), 1.0f / 3.0f)) + 1;
        uint ix = gid % side;
        uint iy = (gid / side) % side;
        uint iz = gid / (side * side);
        float spacing = min(p.grid_x, min(p.grid_y, p.grid_z)) * 0.8f / float(side);
        pos = float3(
            2.0f + float(ix) * spacing,
            2.0f + float(iy) * spacing,
            2.0f + float(iz) * spacing
        ) + (r - 0.5f) * 0.3f;
    }
    else {
        // Random
        pos = float3(
            r.x * (p.grid_x - 2.0f) + 1.0f,
            r.y * (p.grid_y - 2.0f) + 1.0f,
            r.z * (p.grid_z - 2.0f) + 1.0f
        );
    }
 
    // Periodic wrap into domain (no boundary clamping).
    float3 extent = float3(p.grid_x, p.grid_y, p.grid_z);
    pos = pos - extent * floor(pos / extent);
    
    positions[gid * 3 + 0] = pos.x;
    positions[gid * 3 + 1] = pos.y;
    positions[gid * 3 + 2] = pos.z;
}

// -----------------------------------------------------------------------------
// Kernel: Initialize particle properties (velocity, energy, etc.)
// -----------------------------------------------------------------------------

kernel void initialize_particle_properties(
    device const float* positions      [[buffer(0)]],  // N * 3
    device float* velocities           [[buffer(1)]],  // N * 3
    device float* energies             [[buffer(2)]],  // N
    device float* heats                [[buffer(3)]],  // N
    device float* excitations          [[buffer(4)]],  // N
    device float* masses               [[buffer(5)]],  // N
    device const float* random_vals    [[buffer(6)]],  // N * 4 (for vel_scale, energy, exc, unused)
    constant ParticleGenParams& p      [[buffer(7)]],
    constant float& center_x           [[buffer(8)]],  // Mean position x
    constant float& center_y           [[buffer(9)]],  // Mean position y  
    constant float& center_z           [[buffer(10)]], // Mean position z
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_particles) return;
    
    float3 pos = float3(
        positions[gid * 3 + 0],
        positions[gid * 3 + 1],
        positions[gid * 3 + 2]
    );
    
    float3 center = float3(center_x, center_y, center_z);
    
    // Velocity: toward center with small random component
    float3 vel = (center - pos) * 0.01f;
    vel += (float3(random_vals[gid * 4 + 0], random_vals[gid * 4 + 1], random_vals[gid * 4 + 2]) - 0.5f) * 0.1f;
    
    // Energy: distance-based for cluster/sphere, random otherwise
    float energy;
    if (p.pattern == 0 || p.pattern == 2) {
        float dist = length(pos - center);
        float max_dist = p.spread + 1.0f;
        energy = (1.0f - dist / max_dist) * p.energy_scale + 0.1f;
    } else {
        energy = random_vals[gid * 4 + 3] * p.energy_scale * 0.5f + 0.5f;
    }
    
    // Heat: preserve host-provided entropic store from SetOscillators
    float heat = heats[gid];
    float mass;
    
    if (heat > 0.0f) {
        energy = heat;
        mass = heat;
    } else {
        heat = 0.0f;
        mass = energy;
    }
    
    // Excitation: small random
    float exc = random_vals[gid * 4 + 2] * 0.1f;
    
    velocities[gid * 3 + 0] = vel.x;
    velocities[gid * 3 + 1] = vel.y;
    velocities[gid * 3 + 2] = vel.z;
    energies[gid] = energy;
    heats[gid] = heat;
    excitations[gid] = exc;
    masses[gid] = mass;
}
