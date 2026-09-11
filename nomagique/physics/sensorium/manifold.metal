// Physics integration note: the v2 host API below supplies a persistent total+auxiliary
// energy solver, a compliant Hertz material solver, and a spatial Hamiltonian wave solver.
// Legacy entry points remain for ABI compatibility; they do NOT automatically use v2.
// See COUPLED_INTEGRATION.md before mixing legacy and v2 state updates.
#include <metal_stdlib>
using namespace metal;
#include "shared/coupled_core.h"

#if __METAL_VERSION__ < 410
#error "Manifold requires Metal Shading Language 4.1 (native threadgroup float atomics)."
#endif


// =============================================================================
// Manifold Physics Kernels
// =============================================================================
// Implements the core physics simulation for the thermo-manifold:
// - Particle-to-field scatter (gravity, heat)
// - Field-to-particle gather + integrated state update
// - Carrier-oscillator coupling
// - Eulerian pilot-wave (Bohmian) guidance
// - Periodic minimum-image particle collisions
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

    // Parallel reduction in shared memory.
    for (uint offset = kReduceThreads / 2; offset > 0; offset >>= 1) {
        if (tid < offset) {
            scratch[tid] += scratch[tid + offset];
        }
        threadgroup_barrier(mem_flags::mem_threadgroup);
    }

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

    for (uint offset = kReduceThreads / 2; offset > 0; offset >>= 1) {
        if (tid < offset) {
            scratch[tid] += scratch[tid + offset];
        }
        threadgroup_barrier(mem_flags::mem_threadgroup);
    }

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
        float std = sqrt(max(var, 0.0f));

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
// Utility: Minimum-Image Displacement & Trilinear Interpolation
// -----------------------------------------------------------------------------

// Return the shortest displacement vector on a periodic rectangular torus.
//
// [CHOICE] minimum-image convention
// [FORMULA] d_min = d - L * floor(d/L + 1/2)
// [REASON] particles near opposite faces of a periodic box are physically nearby.
// [NOTES] callers must ensure every component of domain is strictly positive.
inline float3 min_image_delta(float3 d, float3 domain) {
    float3 q = d / domain;
    float3 nearest_image = floor(q + 0.5f);
    return d - domain * nearest_image;
}

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

    // The periodic reduction above should already give g in [0,dims). The min()
    // is a final floating-point edge guard against an index equal to dims.
    base_idx = min(uint3(floor(g)), grid_dims - 1u); // 0..dims-1
    frac = g - float3(base_idx);                    // [0,1)
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
    uint x1 = (x0 + 1u) % grid_dims.x;
    uint y1 = (y0 + 1u) % grid_dims.y;
    uint z1 = (z0 + 1u) % grid_dims.z;

    auto idx3 = [&](uint x, uint y, uint z) -> uint {
        return x * stride_x + y * stride_y + z * stride_z;
    };

    float c000 = field[idx3(x0, y0, z0)];
    float c001 = field[idx3(x0, y0, z1)];
    float c010 = field[idx3(x0, y1, z0)];
    float c011 = field[idx3(x0, y1, z1)];
    float c100 = field[idx3(x1, y0, z0)];
    float c101 = field[idx3(x1, y0, z1)];
    float c110 = field[idx3(x1, y1, z0)];
    float c111 = field[idx3(x1, y1, z1)];
    
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

// Exact analytic gradient of the periodic trilinear interpolant
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
    
    // Differentiate the eight-corner interpolant analytically. No displaced
    // sampling is used; its continuum accuracy follows the interpolation error.
    
    // Periodic corner sampling (same as sample_field_trilinear)
    uint x0 = base_idx.x;
    uint y0 = base_idx.y;
    uint z0 = base_idx.z;
    uint x1 = (x0 + 1u) % grid_dims.x;
    uint y1 = (y0 + 1u) % grid_dims.y;
    uint z1 = (z0 + 1u) % grid_dims.z;

    auto idx3 = [&](uint x, uint y, uint z) -> uint {
        return x * stride_x + y * stride_y + z * stride_z;
    };

    float c000 = field[idx3(x0, y0, z0)];
    float c001 = field[idx3(x0, y0, z1)];
    float c010 = field[idx3(x0, y1, z0)];
    float c011 = field[idx3(x0, y1, z1)];
    float c100 = field[idx3(x1, y0, z0)];
    float c101 = field[idx3(x1, y0, z1)];
    float c110 = field[idx3(x1, y1, z0)];
    float c111 = field[idx3(x1, y1, z1)];
    
    // Gradient in each direction (using trilinear interpolation of face values)
    float fy = frac.y;
    float fz = frac.z;
    
    // dF/dx: difference between x=1 and x=0 faces
    float face_x0 = c000 * (1-fy) * (1-fz) + c010 * fy * (1-fz) + c001 * (1-fy) * fz + c011 * fy * fz;
    float face_x1 = c100 * (1-fy) * (1-fz) + c110 * fy * (1-fz) + c101 * (1-fy) * fz + c111 * fy * fz;
    float grad_x = (face_x1 - face_x0) * inv_spacing;
    
    float fx = frac.x;
    // dF/dy
    // IMPORTANT: the y=1 face must use {c010,c110,c011,c111}. Mixing c100/c001
    // into this face corrupts the Bohmian Y-current because these gradients feed ∇Ψ.
    float face_y0 = c000 * (1-fx) * (1-fz) + c100 * fx * (1-fz) + c001 * (1-fx) * fz + c101 * fx * fz;
    float face_y1 = c010 * (1-fx) * (1-fz) + c110 * fx * (1-fz) + c011 * (1-fx) * fz + c111 * fx * fz;
    float grad_y = (face_y1 - face_y0) * inv_spacing;
    
    // dF/dz
    float face_z0 = c000 * (1-fx) * (1-fy) + c100 * fx * (1-fy) + c010 * (1-fx) * fy + c110 * fx * fy;
    float face_z1 = c001 * (1-fx) * (1-fy) + c101 * fx * (1-fy) + c011 * (1-fx) * fy + c111 * fx * fy;
    float grad_z = (face_z1 - face_z0) * inv_spacing;
    
    return float3(grad_x, grad_y, grad_z);
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
    // Periodic domain extents. If all three are >0, minimum-image collision
    // distances are used; otherwise this kernel falls back to non-periodic distance.
    float domain_x;
    float domain_y;
    float domain_z;
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
        particle_vel_in[gid * 3 + 0],
        particle_vel_in[gid * 3 + 1],
        particle_vel_in[gid * 3 + 2]
    );
    float mass_i = particle_mass[gid];
    float heat_i = particle_heat_in[gid];
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
    
    // Particle radius (from material property)
    float r_i = p.particle_radius;
    float3 domain = float3(p.domain_x, p.domain_y, p.domain_z);
    bool periodic_domain = (domain.x > 0.0f) && (domain.y > 0.0f) && (domain.z > 0.0f);
    
    // Accumulate impulses and heat changes
    float3 impulse_total = float3(0.0f);
    float heat_delta = 0.0f;
    
    // Loop over all other particles
    for (uint j = 0; j < p.num_particles; j++) {
        if (j == gid) continue;
        
        float3 pos_j = float3(
            particle_pos[j * 3 + 0],
            particle_pos[j * 3 + 1],
            particle_pos[j * 3 + 2]
        );
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
        
        float r_j = p.particle_radius;
        float combined_radius = r_i + r_j;
        
        // Distance vector from j to i. In periodic mode use the minimum image so
        // particles on opposite box faces collide exactly as neighboring particles do.
        float3 raw_delta = pos_i - pos_j;
        float3 delta = periodic_domain ? min_image_delta(raw_delta, domain) : raw_delta;
        float dist = length(delta);
        
        if (dist < combined_radius && dist > 1e-6f) {
            // =====================================================
            // COLLISION DETECTED
            // =====================================================
            float3 n = delta / dist;  // Normal from j to i
            float overlap = combined_radius - dist;
            
            // Relative velocity (i relative to j)
            float3 v_rel = vel_i - vel_j;
            float v_n = dot(v_rel, n);  // Normal component
            
            // -------------------------------------------------
            // IMPULSE-BASED COLLISION (momentum conservation)
            // -------------------------------------------------
            if (v_n < 0.0f) {  // Only if approaching
                // Coefficient of restitution e: v'_rel = -e * v_rel
                float e = p.restitution;
                
                // Reduced mass: m_eff = m_i * m_j / (m_i + m_j)
                float m_eff = (mass_i * mass_j) / (mass_i + mass_j);
                
                // Impulse magnitude: J = (1 + e) * m_eff * |v_n|
                float J = (1.0f + e) * m_eff * (-v_n);
                
                // Impulse on particle i: Δv_i = J/m_i * n
                // Each thread updates ONLY its own particle: apply the full own-side impulse.
                impulse_total += n * J / mass_i;
                
                // -------------------------------------------------
                // ENERGY CONSERVATION: KE_lost becomes heat
                // -------------------------------------------------
                // KE_before = 0.5 * m_eff * v_n^2
                // KE_after  = 0.5 * m_eff * (e * v_n)^2
                // ΔKE = 0.5 * m_eff * v_n^2 * (1 - e^2)
                float ke_lost = 0.5f * m_eff * v_n * v_n * (1.0f - e * e);
                heat_delta += ke_lost * 0.5f;  // Half to each particle
            }
            
            // No penalty spring after a hard-sphere restitution impulse.
            // Compliant material contact uses manifold_contact_step_v2.
            
            // -------------------------------------------------
            // HEAT CONDUCTION ON CONTACT (Fourier's law)
            // -------------------------------------------------
            // Q = k * A * (T_j - T_i) / d, where d ≈ overlap
            // Approximate: dQ/dt ∝ k * (T_j - T_i) * contact_area
            // Temperature consistency: T = Q / (m * c_v)
            // [CHOICE] contact conduction temperature mapping
            // [FORMULA] T = Q / (m c_v)
            // [REASON] consistent with particle internal energy definition
            // [NOTES] invariants m>0, c_v>0 enforced above (no silent eps clamps).
            float cv = p.specific_heat;
            float T_i = heat_i / (mass_i * cv);
            float T_j = heat_j / (mass_j * cv);
            float contact_area = overlap * overlap;  // Approximate circular contact
            float dQ_conduction = p.thermal_conductivity * contact_area * (T_j - T_i) * p.dt;
            heat_delta += dQ_conduction;
            
            // Note: Excitation (oscillator frequency) is an INTRINSIC property
            // and does NOT equilibrate on contact. Each particle maintains its
            // unique frequency throughout the simulation.
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
    atomic_fetch_add_explicit(&cell_counts[linear_idx], 1u, memory_order_relaxed);
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

// Parallel callers must use the complete host-orchestrated
// manifold_exclusive_scan_u32 API (block scans plus inter-block offsets).

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
#define MS_SCAN_BARRIER() threadgroup_barrier(mem_flags::mem_threadgroup)
#include "shared/exclusive_scan_block.inc"
#undef MS_SCAN_BARRIER
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
    uint slot = atomic_fetch_add_explicit(&cell_offsets[cell_idx], 1u, memory_order_relaxed);
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

// Reduce only finite active frequencies. The caller initializes min to UINT_MAX
// and max to 0 before each reduction, and orders any consumer after this dispatch.
// Direct native atomics avoid dependence on helper declarations later in the file.
kernel void coherence_reduce_omega_minmax_keys(
    device const float* carrier_omega       [[buffer(0)]],
    device const uint* num_carriers_in      [[buffer(1)]],
    device atomic_uint* omega_min_key       [[buffer(2)]],
    device atomic_uint* omega_max_key       [[buffer(3)]],
    uint gid [[thread_position_in_grid]]
) {
    uint n = (num_carriers_in != nullptr) ? num_carriers_in[0] : 0u;
    if (gid >= n) return;
    float w = carrier_omega[gid];
    if (!isfinite(w)) return;
    uint key = float_to_ordered_u32(w);
    atomic_fetch_min_explicit(&omega_min_key[0], key, memory_order_relaxed);
    atomic_fetch_max_explicit(&omega_max_key[0], key, memory_order_relaxed);
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
        particle_vel_in[gid * 3 + 0],
        particle_vel_in[gid * 3 + 1],
        particle_vel_in[gid * 3 + 2]
    );
    float mass_i = particle_mass[gid];
    float heat_i = particle_heat_in[gid];
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
    
    float r_i = p.particle_radius;
    uint3 grid_dims = uint3(p.grid_x, p.grid_y, p.grid_z);

    // Exact periodic extent represented by the hash grid. Neighbor-cell indices already
    // wrap below; this domain is also required to minimum-image the particle displacement.
    float3 domain = float3(
        (float)p.grid_x * p.cell_size,
        (float)p.grid_y * p.cell_size,
        (float)p.grid_z * p.cell_size
    );
    
    // Get this particle's cell
    float3 domain_min = float3(p.domain_min_x, p.domain_min_y, p.domain_min_z);
    uint3 cell_i = position_to_cell(pos_i, p.inv_cell_size, domain_min, grid_dims);
    
    // Accumulate impulses and changes
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
                    
                    // The neighbor cell may have wrapped across the domain boundary.
                    // Therefore the geometric displacement must use the same periodic topology.
                    float3 delta = min_image_delta(pos_i - pos_j, domain);
                    float dist_sq = dot(delta, delta);
                    float r_j = p.particle_radius;
                    float combined_radius = r_i + r_j;
                    
                    // Early exit with squared distance check (avoid sqrt)
                    if (dist_sq >= combined_radius * combined_radius || dist_sq < 1e-12f) {
                        continue;
                    }
                    
                    float dist = sqrt(dist_sq);
                    
                    // =====================================================
                    // COLLISION DETECTED
                    // =====================================================
                    float3 n = delta / dist;
                    float overlap = combined_radius - dist;
                    
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
                    
                    float3 v_rel = vel_i - vel_j;
                    float v_n = dot(v_rel, n);
                    
                    // IMPULSE-BASED COLLISION
                    if (v_n < 0.0f) {
                        float e = p.restitution;
                        float m_eff = (mass_i * mass_j) / (mass_i + mass_j);
                        float J = (1.0f + e) * m_eff * (-v_n);
                        impulse_total += n * J / mass_i;
                        
                        // Energy conservation
                        float ke_lost = 0.5f * m_eff * v_n * v_n * (1.0f - e * e);
                        heat_delta += ke_lost * 0.5f;
                    }
                    
                    // HERTZIAN CONTACT FORCE

                    // No additional spring impulse: use the compliant v2 contact API.
                    
                    // HEAT CONDUCTION
                    // Temperature consistency: T = Q / (m * c_v)
                    float cv = p.specific_heat;
                    float T_i = heat_i / (mass_i * cv);
                    float T_j = heat_j / (mass_j * cv);
                    float contact_area = overlap * overlap;
                    float dQ_conduction = p.thermal_conductivity * contact_area * (T_j - T_i) * p.dt;
                    heat_delta += dQ_conduction;
                    
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

// -----------------------------------------------------------------------------
// Native typed float atomics; no integer-bit CAS emulation.
inline void atomic_add_float_threadgroup(threadgroup atomic_float* address, float val) {
    atomic_fetch_add_explicit(address, val, memory_order_relaxed);
}
inline void atomic_add_float_device(device atomic_float* address, float val) {
    atomic_fetch_add_explicit(address, val, memory_order_relaxed);
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
// 3. manifold_exclusive_scan_u32: Compute exclusive cell starts and total
// 4. scatter_reorder: Move particles to sorted positions
// 5. scatter_sorted: Process sorted particles (main scatter)

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

    // Compute base cell (floor of position / grid_spacing, with periodic wrap)
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

// Step 3: manifold_exclusive_scan_u32 uses the hierarchical exclusive scan.

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
    device uint* sorted_original_idx      [[buffer(13)]], // N (optional: track original indices)
    constant SortScatterParams& p         [[buffer(14)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_particles) return;

    uint cell = particle_cell_idx[gid];
    uint base = cell_starts[cell];
    uint offset = atomic_fetch_add_explicit(&cell_offsets[cell], 1u, memory_order_relaxed);
    uint dest = base + offset;

    // Copy particle data to sorted position
    particle_pos_out[dest * 3 + 0] = particle_pos_in[gid * 3 + 0];
    particle_pos_out[dest * 3 + 1] = particle_pos_in[gid * 3 + 1];
    particle_pos_out[dest * 3 + 2] = particle_pos_in[gid * 3 + 2];
    particle_vel_out[dest * 3 + 0] = particle_vel_in[gid * 3 + 0];
    particle_vel_out[dest * 3 + 1] = particle_vel_in[gid * 3 + 1];
    particle_vel_out[dest * 3 + 2] = particle_vel_in[gid * 3 + 2];
    particle_mass_out[dest] = particle_mass_in[gid];
    particle_heat_out[dest] = particle_heat_in[gid];
    particle_energy_out[dest] = particle_energy_in[gid];
    sorted_original_idx[dest] = gid;
}

// Step 5: Scatter from sorted particles (main kernel)
// Each particle writes to 8 neighboring cells via trilinear interpolation.
// Because particles are sorted by primary cell, nearby threads tend to write
// to nearby cells, improving cache behavior even with global atomics.
kernel void scatter_sorted(
    device const float* particle_pos      [[buffer(0)]],  // N * 3 (sorted)
    device const float* particle_vel      [[buffer(1)]],  // N * 3 (sorted)
    device const float* particle_mass     [[buffer(2)]],  // N (sorted)
    device const float* particle_heat     [[buffer(3)]],  // N (sorted)
    device const float* particle_energy   [[buffer(4)]],  // N (sorted)
    device atomic_float* rho_field         [[buffer(5)]],  // gx * gy * gz (float bits)
    device atomic_float* mom_field         [[buffer(6)]],  // gx * gy * gz * 3 (float bits)
    device atomic_float* E_field           [[buffer(7)]],  // gx * gy * gz (float bits)
    constant SortScatterParams& p         [[buffer(8)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_particles) return;

    float3 pos = float3(
        particle_pos[gid * 3 + 0],
        particle_pos[gid * 3 + 1],
        particle_pos[gid * 3 + 2]
    );
    float3 vel = float3(
        particle_vel[gid * 3 + 0],
        particle_vel[gid * 3 + 1],
        particle_vel[gid * 3 + 2]
    );
    float mass = particle_mass[gid];
    float heat = particle_heat[gid];
    // [CHOICE] dual-energy PIC scatter (thermal energy only)
    // [FORMULA] u_int := Q  (no kinetic energy; oscillator energy not deposited)
    // [REASON] prevents positive feedback where a constant oscillator store is
    //          repeatedly re-deposited into the thermal field each step.
    // [NOTES] If/when we model oscillator↔thermal exchange, it must be explicit
    //         and locally energy-conserving (not implicit re-scatter).
    float e_int = heat;

    // Trilinear interpolation weights
    uint3 grid_dims = uint3(p.grid_x, p.grid_y, p.grid_z);
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

    uint gx = p.grid_x, gy = p.grid_y, gz = p.grid_z;
    uint x0 = base_idx.x, y0 = base_idx.y, z0 = base_idx.z;
    uint x1 = (x0 + 1u) % gx;
    uint y1 = (y0 + 1u) % gy;
    uint z1 = (z0 + 1u) % gz;

    uint stride_z = 1;
    uint stride_y = gz;
    uint stride_x = gy * gz;

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

    // Deposit to all 8 corners (global atomics, but with good locality)
    float inv_vol = p.inv_grid_spacing * p.inv_grid_spacing * p.inv_grid_spacing;
    for (uint c = 0; c < 8; c++) {
        float w = weights[c] * inv_vol;
        uint idx = idxs[c];
        atomic_add_float_device(&rho_field[idx], mass * w);
        atomic_add_float_device(&E_field[idx], e_int * w);
        uint mbase = idx * 3u;
        atomic_add_float_device(&mom_field[mbase + 0u], (mass * vel.x) * w);
        atomic_add_float_device(&mom_field[mbase + 1u], (mass * vel.y) * w);
        atomic_add_float_device(&mom_field[mbase + 2u], (mass * vel.z) * w);
    }
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
    device const float* rho,
    device const float* mom,
    device const float* e_int,
    uint idx
) {
    U5 U;
    U.rho = rho[idx];
    uint m = idx * 3u;
    U.mom = float3(mom[m + 0u], mom[m + 1u], mom[m + 2u]);
    U.e_int = e_int[idx];
    return U;
}

inline void store_U5(
    device float* rho,
    device float* mom,
    device float* e_int,
    uint idx,
    U5 U
) {
    rho[idx] = U.rho;
    uint m = idx * 3u;
    mom[m + 0u] = U.mom.x;
    mom[m + 1u] = U.mom.y;
    mom[m + 2u] = U.mom.z;
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

inline void gas_rhs_cell(
    device const float* rho0,
    device const float* mom0,
    device const float* e0,
    constant GasGridParams& p,
    uint idx,
    thread float& drho,
    thread float3& dmom,
    thread float& de_int
) {
    uint gx = p.grid_x, gy = p.grid_y, gz = p.grid_z;
    float dx = p.dx;
    float inv_dx = 1.0f / dx;
    float inv_dx2 = 1.0f / (dx * dx);
    float gamma = p.gamma;

    uint x, y, z;
    ijk_from_linear(idx, gx, gy, gz, x, y, z);

    uint xm = wrap_minus_one(x, gx), xp = wrap_plus_one(x, gx);
    uint ym = wrap_minus_one(y, gy), yp = wrap_plus_one(y, gy);
    uint zm = wrap_minus_one(z, gz), zp = wrap_plus_one(z, gz);

    uint idx_c  = idx;
    uint idx_xm = idx3_periodic(xm, y, z, gx, gy, gz);
    uint idx_xp = idx3_periodic(xp, y, z, gx, gy, gz);
    uint idx_ym = idx3_periodic(x, ym, z, gx, gy, gz);
    uint idx_yp = idx3_periodic(x, yp, z, gx, gy, gz);
    uint idx_zm = idx3_periodic(x, y, zm, gx, gy, gz);
    uint idx_zp = idx3_periodic(x, y, zp, gx, gy, gz);

    U5 Uc  = load_U5(rho0, mom0, e0, idx_c);
    U5 Uxm = load_U5(rho0, mom0, e0, idx_xm);
    U5 Uxp = load_U5(rho0, mom0, e0, idx_xp);
    U5 Uym = load_U5(rho0, mom0, e0, idx_ym);
    U5 Uyp = load_U5(rho0, mom0, e0, idx_yp);
    U5 Uzm = load_U5(rho0, mom0, e0, idx_zm);
    U5 Uzp = load_U5(rho0, mom0, e0, idx_zp);

    // FAIL-FAST: stencil must already be admissible.
    if (!admissible_U5(Uc,  p.gamma, p.rho_min, p.p_min) ||
        !admissible_U5(Uxm, p.gamma, p.rho_min, p.p_min) ||
        !admissible_U5(Uxp, p.gamma, p.rho_min, p.p_min) ||
        !admissible_U5(Uym, p.gamma, p.rho_min, p.p_min) ||
        !admissible_U5(Uyp, p.gamma, p.rho_min, p.p_min) ||
        !admissible_U5(Uzm, p.gamma, p.rho_min, p.p_min) ||
        !admissible_U5(Uzp, p.gamma, p.rho_min, p.p_min)) {
        float qn = qnan_f();
        drho = qn;
        dmom = float3(qn);
        de_int = qn;
        return;
    }

    // Primitives for center and 6 neighbors (floors for wave speeds).
    float rho_c, p_c, T_c, c_c, sp_c;
    float3 u_c;
    primitives_from_U(Uc, gamma, p.c_v, p.rho_min, p.p_min, rho_c, u_c, p_c, T_c, c_c, sp_c);

    float rho_xm, p_xm, T_xm, c_xm, sp_xm; float3 u_xm;
    float rho_xp, p_xp, T_xp, c_xp, sp_xp; float3 u_xp;
    float rho_ym, p_ym, T_ym, c_ym, sp_ym; float3 u_ym;
    float rho_yp, p_yp, T_yp, c_yp, sp_yp; float3 u_yp;
    float rho_zm, p_zm, T_zm, c_zm, sp_zm; float3 u_zm;
    float rho_zp, p_zp, T_zp, c_zp, sp_zp; float3 u_zp;

    primitives_from_U(Uxm, gamma, p.c_v, p.rho_min, p.p_min, rho_xm, u_xm, p_xm, T_xm, c_xm, sp_xm);
    primitives_from_U(Uxp, gamma, p.c_v, p.rho_min, p.p_min, rho_xp, u_xp, p_xp, T_xp, c_xp, sp_xp);
    primitives_from_U(Uym, gamma, p.c_v, p.rho_min, p.p_min, rho_ym, u_ym, p_ym, T_ym, c_ym, sp_ym);
    primitives_from_U(Uyp, gamma, p.c_v, p.rho_min, p.p_min, rho_yp, u_yp, p_yp, T_yp, c_yp, sp_yp);
    primitives_from_U(Uzm, gamma, p.c_v, p.rho_min, p.p_min, rho_zm, u_zm, p_zm, T_zm, c_zm, sp_zm);
    primitives_from_U(Uzp, gamma, p.c_v, p.rho_min, p.p_min, rho_zp, u_zp, p_zp, T_zp, c_zp, sp_zp);

    // Face fluxes (Rusanov/LLF), minus = (i-1,i), plus = (i,i+1)
    F5 Fx_m = rusanov_flux(
        inviscid_flux_dir(0u, Uxm, u_xm, p_xm),
        inviscid_flux_dir(0u, Uc,  u_c,  p_c),
        Uxm, Uc,
        max(fabs(u_xm.x) + c_xm, fabs(u_c.x) + c_c)
    );
    F5 Fx_p = rusanov_flux(
        inviscid_flux_dir(0u, Uc,  u_c,  p_c),
        inviscid_flux_dir(0u, Uxp, u_xp, p_xp),
        Uc, Uxp,
        max(fabs(u_xp.x) + c_xp, fabs(u_c.x) + c_c)
    );
    F5 Fy_m = rusanov_flux(
        inviscid_flux_dir(1u, Uym, u_ym, p_ym),
        inviscid_flux_dir(1u, Uc,  u_c,  p_c),
        Uym, Uc,
        max(fabs(u_ym.y) + c_ym, fabs(u_c.y) + c_c)
    );
    F5 Fy_p = rusanov_flux(
        inviscid_flux_dir(1u, Uc,  u_c,  p_c),
        inviscid_flux_dir(1u, Uyp, u_yp, p_yp),
        Uc, Uyp,
        max(fabs(u_yp.y) + c_yp, fabs(u_c.y) + c_c)
    );
    F5 Fz_m = rusanov_flux(
        inviscid_flux_dir(2u, Uzm, u_zm, p_zm),
        inviscid_flux_dir(2u, Uc,  u_c,  p_c),
        Uzm, Uc,
        max(fabs(u_zm.z) + c_zm, fabs(u_c.z) + c_c)
    );
    F5 Fz_p = rusanov_flux(
        inviscid_flux_dir(2u, Uc,  u_c,  p_c),
        inviscid_flux_dir(2u, Uzp, u_zp, p_zp),
        Uc, Uzp,
        max(fabs(u_zp.z) + c_zp, fabs(u_c.z) + c_c)
    );

    // Conservative divergences for rho and mom; internal energy gets an extra pressure-work source.
    float div_frho = ((Fx_p.frho - Fx_m.frho) + (Fy_p.frho - Fy_m.frho) + (Fz_p.frho - Fz_m.frho)) * inv_dx;
    float3 div_fmom = ((Fx_p.fmom - Fx_m.fmom) + (Fy_p.fmom - Fy_m.fmom) + (Fz_p.fmom - Fz_m.fmom)) * inv_dx;
    float div_fe = ((Fx_p.fe_int - Fx_m.fe_int) + (Fy_p.fe_int - Fy_m.fe_int) + (Fz_p.fe_int - Fz_m.fe_int)) * inv_dx;

    drho = -div_frho;
    dmom = -div_fmom;

    // Pressure work term: -p * div(u)
    float dux_dx = (u_xp.x - u_xm.x) * (0.5f * inv_dx);
    float duy_dy = (u_yp.y - u_ym.y) * (0.5f * inv_dx);
    float duz_dz = (u_zp.z - u_zm.z) * (0.5f * inv_dx);
    float div_u = dux_dx + duy_dy + duz_dz;

    // Heat conduction: ∇·(k ∇T) = k ∇²T (constant k)
    float lap_T = (T_xp + T_xm + T_yp + T_ym + T_zp + T_zm - 6.0f * T_c) * inv_dx2;

    de_int = -div_fe + (-p_c * div_u) + (p.k_thermal * lap_T);
}

kernel void gas_rk2_stage1(
    device const float* rho0      [[buffer(0)]],  // (N,)
    device const float* mom0      [[buffer(1)]],  // (N*3,)
    device const float* e0        [[buffer(2)]],  // (N,)
    device float* rho1            [[buffer(3)]],  // (N,)
    device float* mom1            [[buffer(4)]],  // (N*3,)
    device float* e1              [[buffer(5)]],  // (N,)
    device float* k1_rho          [[buffer(6)]],  // (N,)
    device float* k1_mom          [[buffer(7)]],  // (N*3,)
    device float* k1_e            [[buffer(8)]],  // (N,)
    constant GasGridParams& p     [[buffer(9)]],
    device atomic_uint* dbg_head  [[buffer(10)]],
    device uint* dbg_words        [[buffer(11)]],
    constant uint& dbg_cap        [[buffer(12)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_cells) return;

    float dr; float3 dm; float de;
    gas_rhs_cell(rho0, mom0, e0, p, gid, dr, dm, de);
    if (!isfinite(dr) || !isfinite(dm.x) || !isfinite(dm.y) || !isfinite(dm.z) || !isfinite(de)) {
        // TAG 0x20: gas RHS produced non-finite (likely inadmissible stencil)
        U5 Uc_bad = load_U5(rho0, mom0, e0, gid);
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x20u, gid, Uc_bad.rho, Uc_bad.e_int, Uc_bad.mom.x, Uc_bad.mom.y);
        float qn = qnan_f();
        rho1[gid] = qn;
        e1[gid] = qn;
        uint m = gid * 3u;
        mom1[m + 0u] = qn;
        mom1[m + 1u] = qn;
        mom1[m + 2u] = qn;
        k1_rho[gid] = qn;
        k1_e[gid] = qn;
        k1_mom[m + 0u] = qn;
        k1_mom[m + 1u] = qn;
        k1_mom[m + 2u] = qn;
        return;
    }

    // Stage1 state
    U5 Uc = load_U5(rho0, mom0, e0, gid);
    U5 U1;
    U1.rho = Uc.rho + p.dt * dr;
    U1.mom = Uc.mom + p.dt * dm;
    U1.e_int = Uc.e_int + p.dt * de;
    if (!admissible_U5(U1, p.gamma, p.rho_min, p.p_min)) {
        // TAG 0x12: stage1 produced inadmissible state
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x12u, gid, Uc.rho, Uc.e_int, U1.rho, U1.e_int);
        float qn = qnan_f();
        rho1[gid] = qn;
        e1[gid] = qn;
        uint m = gid * 3u;
        mom1[m + 0u] = qn;
        mom1[m + 1u] = qn;
        mom1[m + 2u] = qn;
        k1_rho[gid] = qn;
        k1_e[gid] = qn;
        k1_mom[m + 0u] = qn;
        k1_mom[m + 1u] = qn;
        k1_mom[m + 2u] = qn;
        return;
    }

    store_U5(rho1, mom1, e1, gid, U1);

    // Store k1
    k1_rho[gid] = dr;
    uint m = gid * 3u;
    k1_mom[m + 0u] = dm.x;
    k1_mom[m + 1u] = dm.y;
    k1_mom[m + 2u] = dm.z;
    k1_e[gid] = de;
}

kernel void gas_rk2_stage2(
    device const float* rho0      [[buffer(0)]],  // (N,)
    device const float* mom0      [[buffer(1)]],  // (N*3,)
    device const float* e0        [[buffer(2)]],  // (N,)
    device const float* rho1      [[buffer(3)]],  // (N,)
    device const float* mom1      [[buffer(4)]],  // (N*3,)
    device const float* e1        [[buffer(5)]],  // (N,)
    device const float* k1_rho    [[buffer(6)]],  // (N,)
    device const float* k1_mom    [[buffer(7)]],  // (N*3,)
    device const float* k1_e      [[buffer(8)]],  // (N,)
    device float* rho_out         [[buffer(9)]],  // (N,)
    device float* mom_out         [[buffer(10)]], // (N*3,)
    device float* e_out           [[buffer(11)]], // (N,)
    constant GasGridParams& p     [[buffer(12)]],
    device atomic_uint* dbg_head  [[buffer(13)]],
    device uint* dbg_words        [[buffer(14)]],
    constant uint& dbg_cap        [[buffer(15)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_cells) return;

    float dr2; float3 dm2; float de2;
    gas_rhs_cell(rho1, mom1, e1, p, gid, dr2, dm2, de2);
    if (!isfinite(dr2) || !isfinite(dm2.x) || !isfinite(dm2.y) || !isfinite(dm2.z) || !isfinite(de2)) {
        // TAG 0x21: gas RHS2 produced non-finite (likely inadmissible stencil)
        U5 Uc_bad = load_U5(rho1, mom1, e1, gid);
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x21u, gid, Uc_bad.rho, Uc_bad.e_int, Uc_bad.mom.x, Uc_bad.mom.y);
        float qn = qnan_f();
        rho_out[gid] = qn;
        e_out[gid] = qn;
        uint m = gid * 3u;
        mom_out[m + 0u] = qn;
        mom_out[m + 1u] = qn;
        mom_out[m + 2u] = qn;
        return;
    }

    U5 Uc = load_U5(rho0, mom0, e0, gid);
    float dr1 = k1_rho[gid];
    uint m = gid * 3u;
    float3 dm1 = float3(k1_mom[m + 0u], k1_mom[m + 1u], k1_mom[m + 2u]);
    float de1 = k1_e[gid];

    U5 U2;
    U2.rho = Uc.rho + 0.5f * p.dt * (dr1 + dr2);
    U2.mom = Uc.mom + 0.5f * p.dt * (dm1 + dm2);
    U2.e_int = Uc.e_int + 0.5f * p.dt * (de1 + de2);
    if (!admissible_U5(U2, p.gamma, p.rho_min, p.p_min)) {
        // TAG 0x13: stage2 produced inadmissible state
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x13u, gid, Uc.rho, Uc.e_int, U2.rho, U2.e_int);
        float qn = qnan_f();
        rho_out[gid] = qn;
        e_out[gid] = qn;
        uint m = gid * 3u;
        mom_out[m + 0u] = qn;
        mom_out[m + 1u] = qn;
        mom_out[m + 2u] = qn;
        return;
    }

    store_U5(rho_out, mom_out, e_out, gid, U2);
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
    device float* particle_heat_out       [[buffer(4)]],  // N
    device const float* rho_field         [[buffer(5)]],  // gx * gy * gz
    device const float* mom_field         [[buffer(6)]],  // gx * gy * gz * 3
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
    uint3 grid_dims = uint3(p.grid_x, p.grid_y, p.grid_z);
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

    uint gx = p.grid_x, gy = p.grid_y, gz = p.grid_z;
    uint x0 = base_idx.x, y0 = base_idx.y, z0 = base_idx.z;
    uint x1 = (x0 + 1u) % gx;
    uint y1 = (y0 + 1u) % gy;
    uint z1 = (z0 + 1u) % gz;

    uint stride_z = 1;
    uint stride_y = gz;
    uint stride_x = gy * gz;

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
        rho += w * rho_field[idx];
        E += w * E_field[idx];
        uint mbase = idx * 3u;
        mom.x += w * mom_field[mbase + 0u];
        mom.y += w * mom_field[mbase + 1u];
        mom.z += w * mom_field[mbase + 2u];
    }

    // Convert to primitives at particle position (dual-energy; no subtraction).
    //
    // [CHOICE] primitive recovery uses the *same* low-density envelope as the gas solver
    // [REASON] avoids u=mom/rho and T=e_int/(rho c_v) blow-ups in near-vacuum cells,
    //          and keeps particle advection consistent with the CFL estimator.
    //
    // E_field holds internal energy density directly: e_int = ρ c_v T.
    float e_int_density = E;

    // FAIL-FAST: gathered conserved fields must be finite.
    if (!isfinite(rho) || !isfinite(e_int_density) ||
        !isfinite(mom.x) || !isfinite(mom.y) || !isfinite(mom.z)) {
        // TAG 0x07: invalid gathered conserved fields
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x07u, gid, rho, e_int_density, mom.x, mom.y);
        float qn = qnan_f();
        particle_heat_out[gid] = qn;
        particle_pos_out[gid * 3 + 0] = qn;
        particle_pos_out[gid * 3 + 1] = qn;
        particle_pos_out[gid * 3 + 2] = qn;
        particle_vel_out[gid * 3 + 0] = qn;
        particle_vel_out[gid * 3 + 1] = qn;
        particle_vel_out[gid * 3 + 2] = qn;
        return;
    }

    float cv = p.c_v;
    if (!(cv > 0.0f) || !isfinite(cv)) {
        // Fail loudly: invalid thermodynamic parameter.
        // TAG 0x03: invalid c_v
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x03u, gid, cv, rho, e_int_density, 0.0f);
        float qn = qnan_f();
        particle_heat_out[gid] = qn;
        particle_pos_out[gid * 3 + 0] = qn;
        particle_pos_out[gid * 3 + 1] = qn;
        particle_pos_out[gid * 3 + 2] = qn;
        particle_vel_out[gid * 3 + 0] = qn;
        particle_vel_out[gid * 3 + 1] = qn;
        particle_vel_out[gid * 3 + 2] = qn;
        return;
    }

    // Numerical low-density envelope (must match primitives_from_U()).
    float rho_eps = max(p.rho_min, 0.0f);
    const float f32_eps = 1.1920929e-7f;
    float e_eps = 4.0f * rho_eps * f32_eps;
    const float e_spec_max = 10.0f;
    float e_int_max = e_spec_max * rho_eps;

    bool vacuum_exact = (rho == 0.0f && e_int_density == 0.0f && mom.x == 0.0f && mom.y == 0.0f && mom.z == 0.0f);

    float rho_safe = 0.0f;
    float3 u = float3(0.0f);
    float e_used = 0.0f;

    if (vacuum_exact) {
        // True vacuum: no well-defined continuum temperature/velocity.
        rho_safe = rho_eps; // may be 0; safe because e_used=0
        u = float3(0.0f);
        e_used = 0.0f;
        // Exact vacuum is admissible; the debug buffer records rejections only.
    } else if (fabs(rho) <= rho_eps) {
        // Low-density: require bounded momentum and bounded internal energy density.
        float mom2 = dot(mom, mom);
        float rho_eps2 = rho_eps * rho_eps;
        if (mom2 > rho_eps2 || e_int_density < -e_eps || e_int_density > e_int_max) {
            // TAG 0x08: low-density envelope violated
            dbg_log(dbg_head, dbg_words, dbg_cap, 0x08u, gid, rho, e_int_density, mom.x, mom.y);
            float qn = qnan_f();
            particle_heat_out[gid] = qn;
            particle_pos_out[gid * 3 + 0] = qn;
            particle_pos_out[gid * 3 + 1] = qn;
            particle_pos_out[gid * 3 + 2] = qn;
            particle_vel_out[gid * 3 + 0] = qn;
            particle_vel_out[gid * 3 + 1] = qn;
            particle_vel_out[gid * 3 + 2] = qn;
            return;
        }

        // Regularize primitive recovery using rho_eps (not rho).
        rho_safe = rho_eps;
        if (rho_safe > 0.0f) {
            u = mom / rho_safe;
        } else {
            u = float3(0.0f);
        }
        e_used = (e_int_density < 0.0f) ? 0.0f : e_int_density; // affects primitives only
    } else {
        // Outside envelope: require physical positive-density state.
        if (!(rho > 0.0f)) {
            // TAG 0x07: invalid gathered conserved fields (negative rho)
            dbg_log(dbg_head, dbg_words, dbg_cap, 0x07u, gid, rho, e_int_density, mom.x, mom.y);
            float qn = qnan_f();
            particle_heat_out[gid] = qn;
            particle_pos_out[gid * 3 + 0] = qn;
            particle_pos_out[gid * 3 + 1] = qn;
            particle_pos_out[gid * 3 + 2] = qn;
            particle_vel_out[gid * 3 + 0] = qn;
            particle_vel_out[gid * 3 + 1] = qn;
            particle_vel_out[gid * 3 + 2] = qn;
            return;
        }
        if (!(e_int_density >= 0.0f)) {
            // TAG 0x04: invalid internal energy gather (negative e_int)
            dbg_log(dbg_head, dbg_words, dbg_cap, 0x04u, gid, rho, e_int_density, mom.x, mom.y);
            float qn = qnan_f();
            particle_heat_out[gid] = qn;
            particle_pos_out[gid * 3 + 0] = qn;
            particle_pos_out[gid * 3 + 1] = qn;
            particle_pos_out[gid * 3 + 2] = qn;
            particle_vel_out[gid * 3 + 0] = qn;
            particle_vel_out[gid * 3 + 1] = qn;
            particle_vel_out[gid * 3 + 2] = qn;
            return;
        }

        rho_safe = rho;
        u = mom / rho_safe;
        e_used = e_int_density;
    }

    // Temperature (diagnostic only; cancels out in heat computation).
    float T = ((e_used > 0.0f) && (rho_safe > 0.0f)) ? (e_used / (rho_safe * cv)) : 0.0f;

    float mass = particle_mass[gid];
    if (!isfinite(mass) || !(mass >= 0.0f)) {
        // TAG 0x07: invalid particle mass
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x07u, gid, mass, 0.0f, 0.0f, 0.0f);
        float qn = qnan_f();
        particle_heat_out[gid] = qn;
        particle_pos_out[gid * 3 + 0] = qn;
        particle_pos_out[gid * 3 + 1] = qn;
        particle_pos_out[gid * 3 + 2] = qn;
        particle_vel_out[gid * 3 + 0] = qn;
        particle_vel_out[gid * 3 + 1] = qn;
        particle_vel_out[gid * 3 + 2] = qn;
        return;
    }

    // Heat per particle: Q = m c_v T = m * e_int / ρ  (c_v cancels).
    float heat = ((e_used > 0.0f) && (rho_safe > 0.0f)) ? (mass * e_used / rho_safe) : 0.0f;

    if (!isfinite(heat) || !isfinite(T) || !isfinite(u.x) || !isfinite(u.y) || !isfinite(u.z)) {
        // TAG 0x05: non-finite temperature/heat/velocity result
        dbg_log(dbg_head, dbg_words, dbg_cap, 0x05u, gid, mass, T, heat, rho_safe);
        float qn = qnan_f();
        particle_heat_out[gid] = qn;
        particle_pos_out[gid * 3 + 0] = qn;
        particle_pos_out[gid * 3 + 1] = qn;
        particle_pos_out[gid * 3 + 2] = qn;
        particle_vel_out[gid * 3 + 0] = qn;
        particle_vel_out[gid * 3 + 1] = qn;
        particle_vel_out[gid * 3 + 2] = qn;
        return;
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

// -----------------------------------------------------------------------------
// Native Integer Min/Max Atomics
// -----------------------------------------------------------------------------
// Device packed offender keys use ulong max; no threadgroup ulong is required.


// -----------------------------------------------------------------------------
inline void atomic_max_uint_threadgroup(threadgroup atomic_uint* address, uint val) {
    atomic_fetch_max_explicit(address, val, memory_order_relaxed);
}

inline void atomic_max_uint_device(device atomic_uint* address, uint val) {
    atomic_fetch_max_explicit(address, val, memory_order_relaxed);
}

inline void atomic_min_uint_device(device atomic_uint* address, uint val) {
    atomic_fetch_min_explicit(address, val, memory_order_relaxed);
}

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
    float offender_weight_floor;   // attribution only; never a physical force cutoff
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
    float energy_decay; /* legacy ABI name: amplitude gamma; norm decays exp(-2*gamma*dt) */       // non-unitary damping (>=0) to allow settling
    float chemical_potential; // μ term (acts like population control / bias)
    float inv_domega2;        // 1/(Δω^2) for discrete Laplacian on a uniform ω lattice
    uint  anchors;            // anchor slots per ω-bin (must match MODE_ANCHORS)
    uint  rng_seed;           // for anchor refresh (deterministic)
    float anchor_eps;         // probability of random anchor refresh per step
};

inline float resonance_from_freq(float omega_i, float omega_k, float gate_width) {
    return mc_resonance(omega_i,omega_k,gate_width);
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
    device atomic_float* psi_re_field            [[buffer(5)]],   // grid_numel (float bits)
    device atomic_float* psi_im_field            [[buffer(6)]],   // grid_numel (float bits)
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

    // IMPORTANT: use the same z-fastest / x-major flattening convention used by
    // sample_field_trilinear(): idx = x*(gy*gz) + y*gz + z. A different flattening
    // convention here writes Ψ into a permuted field and corrupts pilot-wave guidance.
    uint gy = p.grid_y;
    uint gz = p.grid_z;
    uint stride_z = 1u;
    uint stride_y = gz;
    uint stride_x = gy * gz;

    auto idx3 = [&](uint x, uint y, uint z) -> uint {
        return x * stride_x + y * stride_y + z * stride_z;
    };

    uint i000 = idx3((uint)ix0, (uint)iy0, (uint)iz0);
    uint i100 = idx3((uint)ix1, (uint)iy0, (uint)iz0);
    uint i010 = idx3((uint)ix0, (uint)iy1, (uint)iz0);
    uint i110 = idx3((uint)ix1, (uint)iy1, (uint)iz0);
    uint i001 = idx3((uint)ix0, (uint)iy0, (uint)iz1);
    uint i101 = idx3((uint)ix1, (uint)iy0, (uint)iz1);
    uint i011 = idx3((uint)ix0, (uint)iy1, (uint)iz1);
    uint i111 = idx3((uint)ix1, (uint)iy1, (uint)iz1);

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

    bool valid = p.grid_x>0u && p.grid_y>0u && p.grid_z>0u &&
        isfinite(p.grid_spacing) && p.grid_spacing>0.0f && isfinite(p.inv_grid_spacing) && p.inv_grid_spacing>0.0f &&
        isfinite(p.dt) && p.dt>=0.0f && isfinite(p.hbar_eff) && p.hbar_eff>0.0f &&
        isfinite(p.eps_denom) && p.eps_denom>=0.0f && isfinite(p.mass_min) && p.mass_min>0.0f &&
        isfinite(p.domain_x) && p.domain_x>0.0f && isfinite(p.domain_y) && p.domain_y>0.0f && isfinite(p.domain_z) && p.domain_z>0.0f &&
        isfinite(particle_mass[gid]) && particle_mass[gid]>0.0f;
    for(uint a=0;a<3u;++a) valid = valid && isfinite(particle_pos_in[3u*gid+a]);
    if(!valid) {
        for(uint a=0;a<3u;++a){particle_pos_out[3u*gid+a]=qnan_f();particle_vel_out[3u*gid+a]=qnan_f();}
        return;
    }
    float3 pos = float3(
        particle_pos_in[gid * 3 + 0],
        particle_pos_in[gid * 3 + 1],
        particle_pos_in[gid * 3 + 2]
    );

    pos.x=mc_wrap(pos.x,p.domain_x);pos.y=mc_wrap(pos.y,p.domain_y);pos.z=mc_wrap(pos.z,p.domain_z);

    float psi_re = sample_trilinear(psi_re_field, pos, p.grid_x, p.grid_y, p.grid_z, p.grid_spacing, p.inv_grid_spacing);
    float psi_im = sample_trilinear(psi_im_field, pos, p.grid_x, p.grid_y, p.grid_z, p.grid_spacing, p.inv_grid_spacing);

    float3 grad_re = sample_gradient_trilinear(psi_re_field, pos, p.grid_x, p.grid_y, p.grid_z, p.grid_spacing, p.inv_grid_spacing);
    float3 grad_im = sample_gradient_trilinear(psi_im_field, pos, p.grid_x, p.grid_y, p.grid_z, p.grid_spacing, p.inv_grid_spacing);

    float denom = psi_re * psi_re + psi_im * psi_im + p.eps_denom;

    // Im(conj(Ψ) ∇Ψ) = Ψ_re ∇Ψ_im - Ψ_im ∇Ψ_re
    float3 current = (psi_re * grad_im - psi_im * grad_re) / denom;

    float m = particle_mass[gid];
    float inv_m = 1.0f / max(m, p.mass_min);

    float3 v = current * (p.hbar_eff * inv_m);

    float3 pos_new = pos + v * p.dt;

    // Periodic wrap in domain. Modulo-style wrapping remains correct even when one
    // timestep crosses more than one full box length; one-shot +/-L corrections do not.
    float3 domain = float3(p.domain_x, p.domain_y, p.domain_z);
    if (!(domain.x > 0.0f) || !(domain.y > 0.0f) || !(domain.z > 0.0f)) {
        float qn = qnan_f();
        particle_pos_out[gid * 3 + 0] = qn;
        particle_pos_out[gid * 3 + 1] = qn;
        particle_pos_out[gid * 3 + 2] = qn;
        particle_vel_out[gid * 3 + 0] = qn;
        particle_vel_out[gid * 3 + 1] = qn;
        particle_vel_out[gid * 3 + 2] = qn;
        return;
    }
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
    // Normalized periodic free-particle thermal kernel at the declared bath scale.
    // O is the anchor-weighted periodized Gaussian, not a minimum-image truncation.
    // [REASON] overlap of localized wavefunctions (anchors represent carrier support)
    // [NOTES] σ_x is physics-derived from thermal de Broglie coherence length.
    float sigma = p.spatial_sigma;
    if (!isfinite(sigma) || sigma<0) return qnan_f();
    float sum_w = 0.0f;
    float sum_ov = 0.0f;
    float3 domain = float3(p.domain_x, p.domain_y, p.domain_z);
    uint base = mode_k * MODE_ANCHORS;
    for (uint a = 0; a < MODE_ANCHORS; a++) {
        uint idx = anchor_idx[base + a];
        if (idx == 0xFFFFFFFFu) continue;
        if (idx>=p.num_osc) return qnan_f();
        float w = anchor_weight[base + a];
        if(!isfinite(w)||w<0)return qnan_f();
        if(w==0)continue;
        float3 pos_a = float3(
            particle_pos[idx * 3 + 0],
            particle_pos[idx * 3 + 1],
            particle_pos[idx * 3 + 2]
        );
        float3 d = min_image_delta(pos_i - pos_a, domain);
        float ov=mc_periodic_gaussian(d.x,domain.x,sigma)*mc_periodic_gaussian(d.y,domain.y,sigma)*mc_periodic_gaussian(d.z,domain.z,sigma);
        sum_w += w;
        sum_ov += w * ov;
    }
    if (!(sum_w > 0.0f)) return 0.0f;
    return sum_ov / sum_w;
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
// Memory layout: min(max_carriers,256) * 6 float atomics per threadgroup
// For 64 carriers: 64 * 6 * 4 = 1536 bytes of threadgroup memory

constant uint kMaxCarriersForTG = 256u;  // Max carriers for threadgroup reduction

// Record is 32 bytes, 8-byte aligned; the last eight bytes are ONE atomic key.
struct CarrierAccumulators {
    atomic_float force_r;
    atomic_float force_i;
    atomic_float w_sum;
    atomic_float w_omega_sum;
    atomic_float w_omega2_sum;
    atomic_float w_amp_sum;
    atomic_ulong packed_offender;
};

// Six native float atomics per local carrier; offender publication is a
// device-space atomic max, avoiding unsupported threadgroup ulong assumptions.
struct TGCarrierAccum {
    atomic_float force_r;
    atomic_float force_i;
    atomic_float w_sum;
    atomic_float w_omega_sum;
    atomic_float w_omega2_sum;
    atomic_float w_amp_sum;
};
static_assert(sizeof(CarrierAccumulators) == 32, "carrier ABI");
static_assert(alignof(CarrierAccumulators) == 8, "packed key alignment");
static_assert(sizeof(TGCarrierAccum) == 24, "shared layout");

kernel void coherence_accumulate_forces(
    // Oscillator state
    device const float* osc_phase           [[buffer(0)]],  // N
    device const float* osc_omega           [[buffer(1)]],  // N
    device const float* osc_amp             [[buffer(2)]],  // N
    // Geometric state (for overlap integrals)
    device const float* particle_pos        [[buffer(3)]],  // N * 3
    // Mode state (read-only)
    device const float* carrier_omega       [[buffer(4)]],  // maxM
    device const float* carrier_gate_width  [[buffer(5)]],  // maxM
    device const uint* carrier_anchor_idx   [[buffer(6)]],  // maxM * MODE_ANCHORS (UINT_MAX=empty)
    device const float* carrier_anchor_w    [[buffer(7)]],  // maxM * MODE_ANCHORS
    // Output accumulators
    device CarrierAccumulators* accums      [[buffer(8)]],  // maxM
    // Parameters
    constant CoherenceModeParams& p         [[buffer(9)]],
    device const uint* num_carriers_in      [[buffer(10)]], // (1,) uint32/int32
    // Sparse binning inputs
    device const uint* bin_starts           [[buffer(11)]],  // num_bins + 1
    device const uint* carrier_binned_idx   [[buffer(12)]], // maxM (indices in [0,num_carriers))
    device const CoherenceBinParams* bin_p  [[buffer(13)]], // (1,) {omega_min, inv_bin_width}
    constant uint& num_bins                 [[buffer(14)]],
    // Heat (read-write): pays for phase alignment work.
    device float* particle_heat             [[buffer(15)]], // N
    // Threadgroup indexing
    uint gid [[thread_position_in_grid]],
    uint tid [[thread_index_in_threadgroup]],
    uint tg_size [[threads_per_threadgroup]],
    // Threadgroup memory for local accumulation
    threadgroup TGCarrierAccum* tg_accums   [[threadgroup(0)]]  // kMaxCarriersForTG
) {
    uint num_carriers = (num_carriers_in != nullptr) ? num_carriers_in[0] : 0u;
    uint capacity = p.max_carriers;
    bool local_accum = num_carriers <= kMaxCarriersForTG;
    if (num_carriers > capacity) {
        // Fail loudly rather than silently clamping counts (would mask host/kernel mismatch).
        if (gid == 0u && capacity > 0u) {
            float qn = qnan_f();
            atomic_store_explicit(&accums[0].force_r, qn, memory_order_relaxed);
            atomic_store_explicit(&accums[0].force_i, qn, memory_order_relaxed);
            atomic_store_explicit(&accums[0].w_sum, qn, memory_order_relaxed);
            atomic_store_explicit(&accums[0].w_omega_sum, qn, memory_order_relaxed);
            atomic_store_explicit(&accums[0].w_omega2_sum, qn, memory_order_relaxed);
            atomic_store_explicit(&accums[0].w_amp_sum, qn, memory_order_relaxed);
        }
        return;
    }

    // Phase 1: Initialize native threadgroup float accumulators
    float zero_value = 0.0f;
    for (uint k = tid; local_accum && k < num_carriers; k += tg_size) {
        atomic_store_explicit(&tg_accums[k].force_r, zero_value, memory_order_relaxed);
        atomic_store_explicit(&tg_accums[k].force_i, zero_value, memory_order_relaxed);
        atomic_store_explicit(&tg_accums[k].w_sum, zero_value, memory_order_relaxed);
        atomic_store_explicit(&tg_accums[k].w_omega_sum, zero_value, memory_order_relaxed);
        atomic_store_explicit(&tg_accums[k].w_omega2_sum, zero_value, memory_order_relaxed);
        atomic_store_explicit(&tg_accums[k].w_amp_sum, zero_value, memory_order_relaxed);
    }
    threadgroup_barrier(mem_flags::mem_threadgroup);

    // Phase 2: Accumulate to threadgroup memory
    if (gid < p.num_osc && num_carriers > 0u) {
        float omega_i = osc_omega[gid];
        float amp_i = osc_amp[gid];
        float phi_i = osc_phase[gid];

        // Explicit open-system coupling heat export. This is not asserted to
        // equal the independently measured Hamiltonian work of the field drive.
        MFCouplingBudget budget=mc_coupling_budget(particle_heat[gid],amp_i,p.metabolic_rate,p.dt);
        if(budget.status || !isfinite(phi_i) || !isfinite(omega_i) ||
           !isfinite(particle_pos[3*gid]) || !isfinite(particle_pos[3*gid+1]) || !isfinite(particle_pos[3*gid+2])) {
            particle_heat[gid]=qnan_f();
        } else {
        particle_heat[gid]=budget.heat;
        float eff_amp=budget.effective_amp;
        float3 pos_i = float3(
            particle_pos[gid * 3 + 0],
            particle_pos[gid * 3 + 1],
            particle_pos[gid * 3 + 2]
        );

        float zr = eff_amp * cos(phi_i);
        float zi = eff_amp * sin(phi_i);

        // A Lorentzian has nonzero tails: evaluate every active mode. Sparse
        // approximation needs an explicit error bound before reintroduction.
        for(uint k=0;k<num_carriers;++k) {
                float omega_k = carrier_omega[k];
                float gate_w = carrier_gate_width[k];
                float r = resonance_from_freq(omega_i, omega_k, gate_w);
                float s = spatial_overlap_from_anchors(pos_i, particle_pos, carrier_anchor_idx, carrier_anchor_w, k, p);
                float w = (r * s) * eff_amp;
                // The attribution threshold must never truncate the physical sums.
                if (w == 0.0f) continue;

                if (local_accum) {
                    threadgroup TGCarrierAccum& tg_acc = tg_accums[k];
                    atomic_add_float_threadgroup(&tg_acc.force_r, w * zr);
                    atomic_add_float_threadgroup(&tg_acc.force_i, w * zi);
                    atomic_add_float_threadgroup(&tg_acc.w_sum, w);
                    atomic_add_float_threadgroup(&tg_acc.w_omega_sum, w * omega_i);
                    atomic_add_float_threadgroup(&tg_acc.w_omega2_sum, w * omega_i * omega_i);
                    atomic_add_float_threadgroup(&tg_acc.w_amp_sum, w * eff_amp);
                } else {
                    atomic_fetch_add_explicit(&accums[k].force_r, w * zr, memory_order_relaxed);
                    atomic_fetch_add_explicit(&accums[k].force_i, w * zi, memory_order_relaxed);
                    atomic_fetch_add_explicit(&accums[k].w_sum, w, memory_order_relaxed);
                    atomic_fetch_add_explicit(&accums[k].w_omega_sum, w * omega_i, memory_order_relaxed);
                    atomic_fetch_add_explicit(&accums[k].w_omega2_sum, w * omega_i * omega_i, memory_order_relaxed);
                    atomic_fetch_add_explicit(&accums[k].w_amp_sum, w * eff_amp, memory_order_relaxed);
                }

                // One atomic transaction owns score AND index. Smaller index wins ties.
                if (isfinite(w) && w > p.offender_weight_floor) {
                    ulong key = (ulong(float_to_ordered_u32(w)) << 32) | ulong(~gid);
                    atomic_fetch_max_explicit(&accums[k].packed_offender, key, memory_order_relaxed);
                }
        }
    }
    } // valid particle budget
    threadgroup_barrier(mem_flags::mem_threadgroup);

    // Phase 3: Flush threadgroup accumulators to global (one atomic per carrier per threadgroup)
    for (uint k = tid; local_accum && k < num_carriers; k += tg_size) {
        threadgroup TGCarrierAccum& tg_acc = tg_accums[k];
        device CarrierAccumulators& g_acc = accums[k];

        // Read native threadgroup float accumulators after the barrier
        float fr = atomic_load_explicit(&tg_acc.force_r, memory_order_relaxed);
        float fi = atomic_load_explicit(&tg_acc.force_i, memory_order_relaxed);
        float ws = atomic_load_explicit(&tg_acc.w_sum, memory_order_relaxed);
        float wos = atomic_load_explicit(&tg_acc.w_omega_sum, memory_order_relaxed);
        float wo2s = atomic_load_explicit(&tg_acc.w_omega2_sum, memory_order_relaxed);
        float was = atomic_load_explicit(&tg_acc.w_amp_sum, memory_order_relaxed);

        // Only flush if there's something to add
        if (fr != 0.0f) atomic_fetch_add_explicit(&g_acc.force_r, fr, memory_order_relaxed);
        if (fi != 0.0f) atomic_fetch_add_explicit(&g_acc.force_i, fi, memory_order_relaxed);
        if (ws != 0.0f) atomic_fetch_add_explicit(&g_acc.w_sum, ws, memory_order_relaxed);
        if (wos != 0.0f) atomic_fetch_add_explicit(&g_acc.w_omega_sum, wos, memory_order_relaxed);
        if (wo2s != 0.0f) atomic_fetch_add_explicit(&g_acc.w_omega2_sum, wo2s, memory_order_relaxed);
        if (was != 0.0f) atomic_fetch_add_explicit(&g_acc.w_amp_sum, was, memory_order_relaxed);

        // Offender keys were published atomically during accumulation.

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


// A fused radix-2/Bluestein step preserves the exact periodic discrete
// Laplacian eigenvalues. No artificial extension of the physical lattice.


#include "coherence_fft_metal.inc"

kernel void coherence_update_oscillator_phases(
    device float* particle_phase               [[buffer(0)]],  // N (in/out)
    device const float* particle_omega         [[buffer(1)]],  // N
    device const float* particle_amp           [[buffer(2)]],  // N
    device const float* mode_real              [[buffer(3)]],  // maxM
    device const float* mode_imag              [[buffer(4)]],  // maxM
    device const float* mode_omega             [[buffer(5)]],  // maxM
    device const float* mode_gate_width        [[buffer(6)]],  // maxM
    device const uint* mode_anchor_idx         [[buffer(7)]],  // maxM * MODE_ANCHORS
    device const float* mode_anchor_weight     [[buffer(8)]],  // maxM * MODE_ANCHORS
    device const uint* num_carriers_in    [[buffer(9)]], // (1,) uint32/int32 snapshot
    constant CoherenceModeParams& p      [[buffer(10)]],
    // Sparse binning inputs
    device const uint* bin_starts         [[buffer(11)]],  // num_bins + 1
    device const uint* carrier_binned_idx [[buffer(12)]],  // maxM
    device const CoherenceBinParams* bin_p [[buffer(13)]],  // (1,)
    constant uint& num_bins               [[buffer(14)]],
    device const float* particle_pos      [[buffer(15)]],  // N * 3
    device float* phase_ledger [[buffer(16)]],
    uint gid [[thread_position_in_grid]]
) {
    if (gid >= p.num_osc) return;
    uint num_carriers = (num_carriers_in != nullptr) ? num_carriers_in[0] : 0u;
    if (num_carriers > p.max_carriers) {
        particle_phase[gid] = qnan_f();
        for(uint j=0;j<6u;++j)phase_ledger[6u*gid+j]=qnan_f();
        return;
    }

    float phi = particle_phase[gid];
    float omega_i = particle_omega[gid];
    float amp_i = particle_amp[gid];
    float3 pos_i = float3(
        particle_pos[gid * 3 + 0],
        particle_pos[gid * 3 + 1],
        particle_pos[gid * 3 + 2]
    );

    if(!isfinite(phi)||!isfinite(omega_i)||!isfinite(amp_i)||amp_i<0.0f||
       !isfinite(pos_i.x)||!isfinite(pos_i.y)||!isfinite(pos_i.z)){
        particle_phase[gid]=qnan_f();for(uint j=0;j<6u;++j)phase_ledger[6u*gid+j]=qnan_f();return;
    }

    // Torque from resonance potential:
    //   θ̇_i += Σ_k T_ik (A_i R_k) sin(ψ_k - θ_i)
    float field_real=0.0f,field_imag=0.0f;
    for(uint k=0;k<num_carriers;++k) {
                float omega_k = mode_omega[k];
                float gate_w = mode_gate_width[k];
                float r = resonance_from_freq(omega_i, omega_k, gate_w);
                float s = spatial_overlap_from_anchors(pos_i, particle_pos, mode_anchor_idx, mode_anchor_weight, k, p);
                float t = r * s;
                float cr = mode_real[k];
                float ci = mode_imag[k];
                field_real+=t*amp_i*cr;field_imag+=t*amp_i*ci;
    }

    MFPhaseFlow flow=mc_phase_flow(phi,omega_i,p.coupling_scale*field_real,p.coupling_scale*field_imag,p.dt);
    particle_phase[gid]=flow.status?qnan_f():flow.phase;
    float values[6]={flow.u0,flow.u1,flow.u2,flow.u3,flow.rate,flow.phase};
    for(uint j=0;j<6;++j)phase_ledger[6*gid+j]=flow.status?qnan_f():values[j];
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
    
    // Heat: starts at zero
    float heat = 0.0f;
    
    // Excitation: small random
    float exc = random_vals[gid * 4 + 2] * 0.1f;
    
    // Mass: proportional to energy
    float mass = energy;
    
    velocities[gid * 3 + 0] = vel.x;
    velocities[gid * 3 + 1] = vel.y;
    velocities[gid * 3 + 2] = vel.z;
    energies[gid] = energy;
    heats[gid] = heat;
    excitations[gid] = exc;
    masses[gid] = mass;
}

// Physics ABI v2 implementations.
#include "physics_v2_metal.inc"

#include "coupled_metal.inc"
