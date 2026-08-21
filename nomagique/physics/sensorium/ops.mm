#include <torch/extension.h>

#include <ATen/mps/MPSDevice.h>
#include <ATen/mps/MPSStream.h>

#include <dlfcn.h>
#include <filesystem>
#include <mutex>
#include <string>
#include <vector>

#import <Foundation/Foundation.h>
#import <Metal/Metal.h>
#include <dispatch/dispatch.h>

namespace fs = std::filesystem;

namespace {

// Must match `ParticleGenParams` in `manifold_physics.metal`.
struct ParticleGenParams {
  uint32_t num_particles;
  float grid_x;
  float grid_y;
  float grid_z;
  float energy_scale;
  uint32_t pattern;
  float center_x;
  float center_y;
  float center_z;
  float spread;
  float dir_x;
  float dir_y;
  float dir_z;
};

// Must match `ParticleInteractionParams` in `manifold_physics.metal`.
struct ParticleInteractionParams {
  uint32_t num_particles;
  float dt;
  float particle_radius;        // r: particle radius for collision detection
  float young_modulus;          // E: Young's modulus for contact stiffness
  float thermal_conductivity;   // k: heat transfer on contact
  float specific_heat;          // c_v: heat capacity per unit mass
  float restitution;            // e: coefficient of restitution (0-1)
};

// Must match `SpectralModeParams` in `manifold_physics.metal`.
struct SpectralModeParams {
  uint32_t num_osc;
  uint32_t max_carriers;
  uint32_t num_carriers;
  float dt;
  float coupling_scale;
  float carrier_reg;
  uint32_t rng_seed;
  float conflict_threshold;
  float offender_weight_floor;
  float gate_width_min;
  float gate_width_max;
  float ema_alpha;
  float recenter_alpha;
  uint32_t mode;
  float anchor_random_eps;
  float stable_amp_threshold;
  float crystallize_amp_threshold;
  float crystallize_conflict_threshold;
  uint32_t crystallize_age;
  float crystallized_coupling_boost;
  float volatile_decay_mul;
  float stable_decay_mul;
  float crystallized_decay_mul;
  float topdown_phase_scale;
  float topdown_energy_scale;
  float topdown_random_energy_eps;
  float repulsion_scale;
  float domain_x;
  float domain_y;
  float domain_z;
  float spatial_sigma;
  float metabolic_rate;
};

// Must match `GPEParams` in `manifold_physics.metal`.
struct GPEParams {
  float dt;
  float hbar_eff;
  float mass_eff;
  float g_interaction;
  float energy_decay;
  float chemical_potential;
  float inv_domega2;
  uint32_t anchors;
  uint32_t rng_seed;
  float anchor_eps;
  float metric_coupling;
};

// Must match `SpatialHashParams` in `manifold_physics.metal`.
struct SpatialHashParams {
  uint32_t num_particles;
  uint32_t grid_x;
  uint32_t grid_y;
  uint32_t grid_z;
  float cell_size;
  float inv_cell_size;
  float domain_min_x;
  float domain_min_y;
  float domain_min_z;
};

// Must match `SpatialCollisionParams` in `manifold_physics.metal`.
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

// Must match `SortScatterParams` in `manifold_physics.metal`.
struct SortScatterParams {
  uint32_t num_particles;
  uint32_t num_cells;
  uint32_t grid_x;
  uint32_t grid_y;
  uint32_t grid_z;
  float grid_spacing;
  float inv_grid_spacing;
};

// Must match `PicGatherParams` in `manifold_physics.metal`.
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

// ----------------------------------------------------------------------------
// Quantum Flow / Pilot-Wave coupling
// ----------------------------------------------------------------------------

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


// Must match `GasGridParams` in `manifold_physics.metal`.
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
  float mu;        // reserved (viscosity) – not used by kernels yet
  float k_thermal; // thermal conductivity (constant)
};

constexpr NSUInteger kThreadsPerThreadgroup = 256;

static id<MTLLibrary> g_lib = nil;
// Manifold physics pipelines
static id<MTLComputePipelineState> g_pipeline_manifold_clear_field = nil;
static id<MTLComputePipelineState> g_pipeline_particle_interactions = nil;
// Adaptive thermodynamics (global reduction) pipelines
static id<MTLComputePipelineState> g_pipeline_reduce_float_stats_pass1 = nil;
static id<MTLComputePipelineState> g_pipeline_reduce_float_stats_finalize = nil;
// Coherence field (Ψ(ω)) pipelines
static id<MTLComputePipelineState> g_pipeline_coherence_gpe_step = nil;
static id<MTLComputePipelineState> g_pipeline_coherence_update_osc_phases = nil;
// Spatial hash grid pipelines (O(N) collisions)
static id<MTLComputePipelineState> g_pipeline_spatial_hash_assign = nil;
static id<MTLComputePipelineState> g_pipeline_spatial_hash_prefix_sum = nil;
static id<MTLComputePipelineState> g_pipeline_spatial_hash_prefix_sum_parallel = nil;
static id<MTLComputePipelineState> g_pipeline_spatial_hash_scatter = nil;
static id<MTLComputePipelineState> g_pipeline_spatial_hash_collisions = nil;
// Generic u32 exclusive scan pipelines (used by spatial hash + spectral bins)
static id<MTLComputePipelineState> g_pipeline_exclusive_scan_u32_pass1 = nil;
static id<MTLComputePipelineState> g_pipeline_exclusive_scan_u32_add_block_offsets = nil;
static id<MTLComputePipelineState> g_pipeline_exclusive_scan_u32_finalize_total = nil;
// Coherence ω-binning pipelines
static id<MTLComputePipelineState> g_pipeline_coherence_reduce_omega_minmax_keys = nil;
static id<MTLComputePipelineState> g_pipeline_coherence_compute_bin_params = nil;
static id<MTLComputePipelineState> g_pipeline_coherence_bin_count_carriers = nil;
static id<MTLComputePipelineState> g_pipeline_coherence_bin_scatter_carriers = nil;
// Sort-based scatter pipelines (deterministic, no hash collisions)
static id<MTLComputePipelineState> g_pipeline_scatter_compute_cell_idx = nil;
static id<MTLComputePipelineState> g_pipeline_scatter_count_cells = nil;
static id<MTLComputePipelineState> g_pipeline_scatter_reorder_particles = nil;
static id<MTLComputePipelineState> g_pipeline_scatter_sorted = nil;
static id<MTLComputePipelineState> g_pipeline_pic_gather_update_particles = nil;
static id<MTLComputePipelineState> g_pipeline_project_modes_to_spatial_psi = nil;
static id<MTLComputePipelineState> g_pipeline_pic_gather_update_particles_pilot_wave = nil;
// Gas (Eulerian grid update) pipelines
static id<MTLComputePipelineState> g_pipeline_gas_rk2_stage1 = nil;
static id<MTLComputePipelineState> g_pipeline_gas_rk2_stage2 = nil;
// Particle generation pipelines
static id<MTLComputePipelineState> g_pipeline_generate_particle_positions = nil;
static id<MTLComputePipelineState> g_pipeline_initialize_particle_properties = nil;
static std::mutex g_pipeline_mutex;

static std::string metallib_path_for_this_module() {
  Dl_info info;
  if (dladdr((void*)&metallib_path_for_this_module, &info) == 0 || info.dli_fname == nullptr) {
    return std::string();
  }
  fs::path so_path(info.dli_fname);
  fs::path lib_path = so_path.parent_path() / "manifold_ops.metallib";
  return lib_path.string();
}

static void ensure_library_locked(id<MTLDevice> device) {
  if (g_lib != nil) {
    return;
  }

  const std::string lib_path = metallib_path_for_this_module();
  TORCH_CHECK(!lib_path.empty(), "manifold_metal_ops: failed to locate extension path via dladdr()");

  NSString* ns_path = [NSString stringWithUTF8String:lib_path.c_str()];
  NSURL* url = [NSURL fileURLWithPath:ns_path];
  NSError* err = nil;
  // newLibraryWithFile:error: is deprecated; use URL variant on newer macOS.
  g_lib = [device newLibraryWithURL:url error:&err];
  if (g_lib == nil) {
    const char* msg = err ? [[err localizedDescription] UTF8String] : "unknown error";
    TORCH_CHECK(false, "manifold_metal_ops: failed to load metallib at ", lib_path, ": ", msg);
  }
}

static id<MTLComputePipelineState> ensure_pipeline(
    id<MTLDevice> device,
    // Important: this is a STRONG out-parameter. If left un-annotated under ARC,
    // Clang treats it as __autoreleasing and rejects passing addresses of globals.
    id<MTLComputePipelineState> __strong* pipeline,
    const char* fn_name) {
  std::lock_guard<std::mutex> lock(g_pipeline_mutex);
  ensure_library_locked(device);

  if (*pipeline != nil) {
    return *pipeline;
  }

  NSString* ns_fn = [NSString stringWithUTF8String:fn_name];
  id<MTLFunction> fn = [g_lib newFunctionWithName:ns_fn];
  TORCH_CHECK(fn != nil, "manifold_metal_ops: function `", fn_name, "` not found in metallib");

  NSError* err = nil;
  *pipeline = [device newComputePipelineStateWithFunction:fn error:&err];
  if (*pipeline == nil) {
    const char* msg = err ? [[err localizedDescription] UTF8String] : "unknown error";
    TORCH_CHECK(false, "manifold_metal_ops: failed to create compute pipeline: ", msg);
  }

  // Basic sanity check against accidental dispatch mismatch.
  TORCH_CHECK(
      (*pipeline).maxTotalThreadsPerThreadgroup >= kThreadsPerThreadgroup,
      "manifold_metal_ops: pipeline maxTotalThreadsPerThreadgroup (",
      (int)(*pipeline).maxTotalThreadsPerThreadgroup,
      ") < expected threads (",
      (int)kThreadsPerThreadgroup,
      ")");

  return *pipeline;
}

static inline id<MTLBuffer> storage_as_mtlbuffer(const at::Tensor& t) {
  // MPS tensors are backed by MTLBuffer allocations; the storage base pointer is
  // an opaque handle that is compatible with `id<MTLBuffer>` in practice.
  //
  // NOTE: This is intentionally low-level to avoid CPU staging/copies.
  const auto& dp = t.storage().data_ptr();
  void* ctx = dp.get_context();
  TORCH_CHECK(
      ctx != nullptr,
      "manifold_metal_ops: expected MPS storage to provide an MTLBuffer context (got null). "
      "This usually indicates a non-standard tensor storage backend.");
  // Under ARC we must use a bridged cast from void* to ObjC object.
  return (__bridge id<MTLBuffer>)ctx;
}

static inline NSUInteger storage_offset_bytes(const at::Tensor& t) {
  return (NSUInteger)(t.storage_offset() * (int64_t)t.element_size());
}

// ----------------------------------------------------------------------------
// Helper: reduce boilerplate for compute kernel dispatch
// ----------------------------------------------------------------------------
// This wraps the standard pattern used throughout this file:
// - get MTLDevice + current MPS stream
// - endKernelCoalescing (barrier between kernels)
// - acquire command encoder
// - ensure pipeline + bind buffers/bytes
// - dispatch 1D threadgroups
// - endKernelCoalescing (finish encoder section)
//
// NOTE: This does not change semantics; it is purely structural refactoring.
struct ComputeKernel {
  id<MTLDevice> device;
  at::mps::MPSStream* stream;
  id<MTLComputeCommandEncoder> encoder;
  id<MTLComputePipelineState> pipeline;

  ComputeKernel(id<MTLComputePipelineState> __strong* pipeline_cache, const char* fn_name) {
    device = (id<MTLDevice>)at::mps::MPSDevice::getInstance()->device();
    stream = at::mps::getCurrentMPSStream();
    stream->endKernelCoalescing();
    encoder = (id<MTLComputeCommandEncoder>)stream->commandEncoder();
    pipeline = ensure_pipeline(device, pipeline_cache, fn_name);
    [encoder setComputePipelineState:pipeline];
  }

  inline void set_tensor(const at::Tensor& t, int idx) {
    id<MTLBuffer> buf = storage_as_mtlbuffer(t);
    const NSUInteger off = storage_offset_bytes(t);
    [encoder setBuffer:buf offset:off atIndex:(NSUInteger)idx];
  }

  template <class T>
  inline void set_bytes(const T* ptr, size_t nbytes, int idx) {
    [encoder setBytes:ptr length:(NSUInteger)nbytes atIndex:(NSUInteger)idx];
  }

  template <class T>
  inline void set_bytes(const T& v, int idx) {
    set_bytes(&v, sizeof(T), idx);
  }

  inline void set_threadgroup_memory_length(NSUInteger nbytes, int idx) {
    [encoder setThreadgroupMemoryLength:nbytes atIndex:(NSUInteger)idx];
  }

  inline void dispatch_groups(NSUInteger num_groups, NSUInteger tg_threads) {
    const MTLSize tg = MTLSizeMake(tg_threads, 1, 1);
    const MTLSize grid = MTLSizeMake(num_groups, 1, 1);
    [encoder dispatchThreadgroups:grid threadsPerThreadgroup:tg];
  }

  inline void dispatch_1d(int64_t n_threads, NSUInteger tg_threads = (NSUInteger)kThreadsPerThreadgroup) {
    const NSUInteger num_groups = (n_threads + (int64_t)tg_threads - 1) / (int64_t)tg_threads;
    dispatch_groups(num_groups, tg_threads);
  }

  ~ComputeKernel() { stream->endKernelCoalescing(); }
};

static void check_contig_3d(const at::Tensor& t, const char* name) {
  TORCH_CHECK(t.is_contiguous(), name, ": must be contiguous");
  TORCH_CHECK(t.dim() == 3, name, ": must be 3D");
  TORCH_CHECK(t.stride(2) == 1, name, ": last dim must be contiguous (stride==1)");
}

static void check_contig_2d(const at::Tensor& t, const char* name) {
  TORCH_CHECK(t.is_contiguous(), name, ": must be contiguous");
  TORCH_CHECK(t.dim() == 2, name, ": must be 2D");
  TORCH_CHECK(t.stride(1) == 1, name, ": last dim must be contiguous (stride==1)");
}

static void check_contig_1d(const at::Tensor& t, const char* name) {
  TORCH_CHECK(t.is_contiguous(), name, ": must be contiguous");
  TORCH_CHECK(t.dim() == 1, name, ": must be 1D");
  TORCH_CHECK(t.stride(0) == 1, name, ": must be contiguous (stride==1)");
}

// ============================================================================
// Manifold Physics Kernels
// ============================================================================

void manifold_clear_field(at::Tensor field) {
  TORCH_CHECK(field.device().is_mps(), "manifold_clear_field: field must be on MPS");
  TORCH_CHECK(field.is_contiguous(), "manifold_clear_field: field must be contiguous");

  const int64_t num_elements = field.numel();
  ComputeKernel k(&g_pipeline_manifold_clear_field, "clear_field");
  k.set_tensor(field, 0);
  const uint32_t n = (uint32_t)num_elements;
  k.set_bytes(n, 1);
  k.dispatch_1d(num_elements);
}

void particle_interactions(
    at::Tensor particle_pos,       // (N, 3) fp32 MPS
    at::Tensor particle_vel,       // (N, 3) fp32 MPS in/out
    at::Tensor particle_excitation,// (N,) fp32 MPS in/out
    at::Tensor particle_mass,      // (N,) fp32 MPS
    at::Tensor particle_heat,      // (N,) fp32 MPS in/out - heat from collisions
    at::Tensor particle_vel_in,    // (N, 3) fp32 MPS (snapshot)
    at::Tensor particle_heat_in,   // (N,) fp32 MPS (snapshot)
    double dt,
    double particle_radius,
    double young_modulus,
    double thermal_conductivity,
    double specific_heat,
    double restitution) {
  
  const int64_t N = particle_pos.size(0);
  if (N == 0) return;
  
  TORCH_CHECK(particle_pos.device().is_mps(), "particle_interactions: particle_pos must be on MPS");
  TORCH_CHECK(particle_pos.is_contiguous() && particle_pos.dim() == 2 && particle_pos.size(1) == 3,
              "particle_interactions: particle_pos must be contiguous (N, 3)");
  TORCH_CHECK(particle_vel.is_contiguous() && particle_vel.dim() == 2 && particle_vel.size(1) == 3,
              "particle_interactions: particle_vel must be contiguous (N, 3)");
  TORCH_CHECK(particle_excitation.is_contiguous() && particle_excitation.dim() == 1,
              "particle_interactions: particle_excitation must be contiguous 1D");
  TORCH_CHECK(particle_heat.is_contiguous() && particle_heat.dim() == 1,
              "particle_interactions: particle_heat must be contiguous 1D");

  ComputeKernel k(&g_pipeline_particle_interactions, "particle_interactions");
  k.set_tensor(particle_pos, 0);
  k.set_tensor(particle_vel, 1);
  k.set_tensor(particle_excitation, 2);
  k.set_tensor(particle_mass, 3);
  k.set_tensor(particle_heat, 4);
  k.set_tensor(particle_vel_in, 5);
  k.set_tensor(particle_heat_in, 6);
  
  ParticleInteractionParams prm;
  prm.num_particles = (uint32_t)N;
  prm.dt = (float)dt;
  prm.particle_radius = (float)particle_radius;
  prm.young_modulus = (float)young_modulus;
  prm.thermal_conductivity = (float)thermal_conductivity;
  prm.specific_heat = (float)specific_heat;
  prm.restitution = (float)restitution;
  k.set_bytes(prm, 7);
  k.dispatch_1d(N);
}

// ============================================================================
// Adaptive Thermodynamics: reduction for global energy statistics
// ============================================================================
// Computes out_stats (4 floats): [mean_abs, mean, std, count]
//
// This stays entirely on the GPU. No CPU sync required.
void thermo_reduce_energy_stats(
    at::Tensor x,         // (N,) fp32 MPS
    at::Tensor out_stats  // (4,) fp32 MPS (output)
) {
  TORCH_CHECK(x.device().is_mps(), "thermo_reduce_energy_stats: x must be on MPS");
  TORCH_CHECK(out_stats.device().is_mps(), "thermo_reduce_energy_stats: out_stats must be on MPS");
  TORCH_CHECK(x.dtype() == at::kFloat, "thermo_reduce_energy_stats: x must be fp32");
  TORCH_CHECK(out_stats.dtype() == at::kFloat, "thermo_reduce_energy_stats: out_stats must be fp32");
  check_contig_1d(x, "thermo_reduce_energy_stats: x");
  check_contig_1d(out_stats, "thermo_reduce_energy_stats: out_stats");
  TORCH_CHECK(out_stats.numel() == 4, "thermo_reduce_energy_stats: out_stats must have 4 elements");

  const int64_t N = x.numel();
  if (N <= 0) {
    // Define a sane empty result: zeros + count=0
    out_stats.zero_();
    return;
  }

  const NSUInteger num_groups = (N + (int64_t)kThreadsPerThreadgroup - 1) / kThreadsPerThreadgroup;

  // group_stats: (num_groups, 4) contiguous fp32 on MPS
  at::Tensor group_stats = at::empty({(int64_t)num_groups, 4}, x.options());

  // Pass 1: per-threadgroup partial sums.
  {
    ComputeKernel k(&g_pipeline_reduce_float_stats_pass1, "reduce_float_stats_pass1");
    k.set_tensor(x, 0);
    k.set_tensor(group_stats, 1);
    const uint32_t n_u = (uint32_t)N;
    k.set_bytes(n_u, 2);
    k.dispatch_groups((NSUInteger)num_groups, (NSUInteger)kThreadsPerThreadgroup);
  }

  // Pass 2: finalize to a single stats vector.
  {
    ComputeKernel k(&g_pipeline_reduce_float_stats_finalize, "reduce_float_stats_finalize");
    k.set_tensor(group_stats, 0);
    k.set_tensor(out_stats, 1);
    const uint32_t g_u = (uint32_t)num_groups;
    k.set_bytes(g_u, 2);
    k.dispatch_groups(1u, (NSUInteger)kThreadsPerThreadgroup);
  }
}

// ============================================================================
// Sort-Based Scatter (Deterministic, No Hash Collisions)
// ============================================================================
// Pre-sorts particles by primary grid cell, then scatters in sorted order.
// Benefits: no warp divergence, coalesced reads, constant-time performance.

void scatter_compute_cell_idx(
    at::Tensor particle_pos,       // (N, 3) fp32 MPS
    at::Tensor particle_cell_idx,  // (N,) uint32 MPS out
    int64_t grid_x,
    int64_t grid_y,
    int64_t grid_z,
    double grid_spacing) {

  const int64_t N = particle_pos.size(0);
  if (N == 0) return;

  TORCH_CHECK(particle_pos.device().is_mps(), "scatter_compute_cell_idx: particle_pos must be on MPS");
  ComputeKernel k(&g_pipeline_scatter_compute_cell_idx, "scatter_compute_cell_idx");
  k.set_tensor(particle_pos, 0);
  k.set_tensor(particle_cell_idx, 1);

  SortScatterParams prm;
  prm.num_particles = (uint32_t)N;
  prm.num_cells = (uint32_t)(grid_x * grid_y * grid_z);
  prm.grid_x = (uint32_t)grid_x;
  prm.grid_y = (uint32_t)grid_y;
  prm.grid_z = (uint32_t)grid_z;
  prm.grid_spacing = (float)grid_spacing;
  prm.inv_grid_spacing = 1.0f / (float)grid_spacing;
  k.set_bytes(prm, 2);
  k.dispatch_1d(N);
}

void scatter_count_cells(
    at::Tensor particle_cell_idx,  // (N,) uint32 MPS
    at::Tensor cell_counts,        // (num_cells,) uint32 MPS atomic out
    int64_t grid_x,
    int64_t grid_y,
    int64_t grid_z,
    double grid_spacing) {

  const int64_t N = particle_cell_idx.size(0);
  if (N == 0) return;

  TORCH_CHECK(particle_cell_idx.device().is_mps(), "scatter_count_cells: particle_cell_idx must be on MPS");
  ComputeKernel k(&g_pipeline_scatter_count_cells, "scatter_count_cells");
  k.set_tensor(particle_cell_idx, 0);
  k.set_tensor(cell_counts, 1);

  SortScatterParams prm;
  prm.num_particles = (uint32_t)N;
  prm.num_cells = (uint32_t)(grid_x * grid_y * grid_z);
  prm.grid_x = (uint32_t)grid_x;
  prm.grid_y = (uint32_t)grid_y;
  prm.grid_z = (uint32_t)grid_z;
  prm.grid_spacing = (float)grid_spacing;
  prm.inv_grid_spacing = 1.0f / (float)grid_spacing;
  k.set_bytes(prm, 2);
  k.dispatch_1d(N);
}

void scatter_reorder_particles(
    at::Tensor particle_pos_in,    // (N, 3) fp32 MPS
    at::Tensor particle_vel_in,    // (N, 3) fp32 MPS
    at::Tensor particle_mass_in,   // (N,) fp32 MPS
    at::Tensor particle_heat_in,   // (N,) fp32 MPS
    at::Tensor particle_energy_in, // (N,) fp32 MPS
    at::Tensor particle_cell_idx,  // (N,) uint32 MPS
    at::Tensor cell_starts,        // (num_cells,) uint32 MPS (prefix sum)
    at::Tensor cell_offsets,       // (num_cells,) uint32 MPS atomic (working copy)
    at::Tensor particle_pos_out,   // (N, 3) fp32 MPS out
    at::Tensor particle_vel_out,   // (N, 3) fp32 MPS out
    at::Tensor particle_mass_out,  // (N,) fp32 MPS out
    at::Tensor particle_heat_out,  // (N,) fp32 MPS out
    at::Tensor particle_energy_out,// (N,) fp32 MPS out
    at::Tensor sorted_original_idx,// (N,) uint32 MPS out
    int64_t grid_x,
    int64_t grid_y,
    int64_t grid_z,
    double grid_spacing) {

  const int64_t N = particle_pos_in.size(0);
  if (N == 0) return;

  TORCH_CHECK(particle_pos_in.device().is_mps(), "scatter_reorder_particles: particle_pos_in must be on MPS");
  ComputeKernel k(&g_pipeline_scatter_reorder_particles, "scatter_reorder_particles");
  k.set_tensor(particle_pos_in, 0);
  k.set_tensor(particle_vel_in, 1);
  k.set_tensor(particle_mass_in, 2);
  k.set_tensor(particle_heat_in, 3);
  k.set_tensor(particle_energy_in, 4);
  k.set_tensor(particle_cell_idx, 5);
  k.set_tensor(cell_starts, 6);
  k.set_tensor(cell_offsets, 7);
  k.set_tensor(particle_pos_out, 8);
  k.set_tensor(particle_vel_out, 9);
  k.set_tensor(particle_mass_out, 10);
  k.set_tensor(particle_heat_out, 11);
  k.set_tensor(particle_energy_out, 12);
  k.set_tensor(sorted_original_idx, 13);

  SortScatterParams prm;
  prm.num_particles = (uint32_t)N;
  prm.num_cells = (uint32_t)(grid_x * grid_y * grid_z);
  prm.grid_x = (uint32_t)grid_x;
  prm.grid_y = (uint32_t)grid_y;
  prm.grid_z = (uint32_t)grid_z;
  prm.grid_spacing = (float)grid_spacing;
  prm.inv_grid_spacing = 1.0f / (float)grid_spacing;
  k.set_bytes(prm, 14);
  k.dispatch_1d(N);
}

void scatter_sorted(
    at::Tensor particle_pos,       // (N, 3) fp32 MPS sorted
    at::Tensor particle_vel,       // (N, 3) fp32 MPS sorted
    at::Tensor particle_mass,      // (N,) fp32 MPS sorted
    at::Tensor particle_heat,      // (N,) fp32 MPS sorted
    at::Tensor particle_energy,    // (N,) fp32 MPS sorted
    at::Tensor rho_field,          // (X, Y, Z) fp32 MPS atomic
    at::Tensor mom_field,          // (X, Y, Z, 3) fp32 MPS atomic
    at::Tensor E_field,            // (X, Y, Z) fp32 MPS atomic
    int64_t grid_x,
    int64_t grid_y,
    int64_t grid_z,
    double grid_spacing) {

  const int64_t N = particle_pos.size(0);
  if (N == 0) return;

  TORCH_CHECK(particle_pos.device().is_mps(), "scatter_sorted: particle_pos must be on MPS");
  ComputeKernel k(&g_pipeline_scatter_sorted, "scatter_sorted");
  k.set_tensor(particle_pos, 0);
  k.set_tensor(particle_vel, 1);
  k.set_tensor(particle_mass, 2);
  k.set_tensor(particle_heat, 3);
  k.set_tensor(particle_energy, 4);
  k.set_tensor(rho_field, 5);
  k.set_tensor(mom_field, 6);
  k.set_tensor(E_field, 7);

  SortScatterParams prm;
  prm.num_particles = (uint32_t)N;
  prm.num_cells = (uint32_t)(grid_x * grid_y * grid_z);
  prm.grid_x = (uint32_t)grid_x;
  prm.grid_y = (uint32_t)grid_y;
  prm.grid_z = (uint32_t)grid_z;
  prm.grid_spacing = (float)grid_spacing;
  prm.inv_grid_spacing = 1.0f / (float)grid_spacing;
  k.set_bytes(prm, 8);
  k.dispatch_1d(N);
}

void pic_gather_update_particles(
    at::Tensor particle_pos_in,    // (N, 3) fp32 MPS
    at::Tensor particle_mass,      // (N,) fp32 MPS
    at::Tensor particle_pos_out,   // (N, 3) fp32 MPS out
    at::Tensor particle_vel_out,   // (N, 3) fp32 MPS out
    at::Tensor particle_heat_out,  // (N,) fp32 MPS out
    at::Tensor rho_field,          // (X, Y, Z) fp32 MPS
    at::Tensor mom_field,          // (X, Y, Z, 3) fp32 MPS
    at::Tensor E_field,            // (X, Y, Z) fp32 MPS
    at::Tensor gravity_potential,  // (X, Y, Z) fp32 MPS (gravitational potential φ)
    at::Tensor dbg_head_u32,       // (1,) int32/u32 MPS (atomic counter)
    at::Tensor dbg_words_u32,      // (cap*6,) int32/u32 MPS (debug words)
    int64_t dbg_capacity,          // number of debug events (0 disables)
    int64_t grid_x,
    int64_t grid_y,
    int64_t grid_z,
    double grid_spacing,
    double dt,
    double domain_x,
    double domain_y,
    double domain_z,
    double gamma,
    double R_specific,
    double c_v,
    double rho_min,
    double p_min,
    double gravity_enabled) {

  const int64_t N = particle_pos_in.size(0);
  if (N == 0) return;

  TORCH_CHECK(particle_pos_in.device().is_mps(), "pic_gather_update_particles: particle_pos_in must be on MPS");
  TORCH_CHECK(dbg_capacity >= 0, "pic_gather_update_particles: dbg_capacity must be >= 0");
  TORCH_CHECK(dbg_head_u32.device().is_mps(), "pic_gather_update_particles: dbg_head_u32 must be on MPS");
  TORCH_CHECK(dbg_words_u32.device().is_mps(), "pic_gather_update_particles: dbg_words_u32 must be on MPS");
  TORCH_CHECK(dbg_head_u32.is_contiguous(), "pic_gather_update_particles: dbg_head_u32 must be contiguous");
  TORCH_CHECK(dbg_words_u32.is_contiguous(), "pic_gather_update_particles: dbg_words_u32 must be contiguous");
  TORCH_CHECK(dbg_head_u32.numel() >= 1, "pic_gather_update_particles: dbg_head_u32 must have at least 1 element");
  if (dbg_capacity > 0) {
    TORCH_CHECK(dbg_words_u32.numel() >= dbg_capacity * 6, "pic_gather_update_particles: dbg_words_u32 too small for dbg_capacity");
  }

  ComputeKernel k(&g_pipeline_pic_gather_update_particles, "pic_gather_update_particles");
  k.set_tensor(particle_pos_in, 0);
  k.set_tensor(particle_mass, 1);
  k.set_tensor(particle_pos_out, 2);
  k.set_tensor(particle_vel_out, 3);
  k.set_tensor(particle_heat_out, 4);
  k.set_tensor(rho_field, 5);
  k.set_tensor(mom_field, 6);
  k.set_tensor(E_field, 7);
  k.set_tensor(gravity_potential, 8);
  k.set_tensor(dbg_head_u32, 10);
  k.set_tensor(dbg_words_u32, 11);

  PicGatherParams prm;
  prm.num_particles = (uint32_t)N;
  prm.grid_x = (uint32_t)grid_x;
  prm.grid_y = (uint32_t)grid_y;
  prm.grid_z = (uint32_t)grid_z;
  prm.grid_spacing = (float)grid_spacing;
  prm.inv_grid_spacing = 1.0f / (float)grid_spacing;
  prm.dt = (float)dt;
  prm.domain_x = (float)domain_x;
  prm.domain_y = (float)domain_y;
  prm.domain_z = (float)domain_z;
  prm.gamma = (float)gamma;
  prm.R_specific = (float)R_specific;
  prm.c_v = (float)c_v;
  prm.rho_min = (float)rho_min;
  prm.p_min = (float)p_min;
  prm.gravity_enabled = (float)gravity_enabled;
  k.set_bytes(prm, 9);

  const uint32_t cap_u32 = (dbg_capacity > 0) ? (uint32_t)dbg_capacity : 0u;
  k.set_bytes(cap_u32, 12);
  k.dispatch_1d(N);
}


// ----------------------------------------------------------------------------
// Quantum Flow / Pilot-Wave coupling
// ----------------------------------------------------------------------------

void project_modes_to_spatial_psi(
    torch::Tensor mode_psi_real,
    torch::Tensor mode_psi_imag,
    torch::Tensor mode_anchor_idx,
    torch::Tensor mode_anchor_weight,
    torch::Tensor particle_pos,
    torch::Tensor psi_re_field,
    torch::Tensor psi_im_field,
    int64_t anchors_per_mode,
    int64_t grid_x,
    int64_t grid_y,
    int64_t grid_z,
    double grid_spacing
) {
    TORCH_CHECK(mode_psi_real.device().is_mps(), "mode_psi_real must be an MPS tensor");
    TORCH_CHECK(mode_psi_imag.device().is_mps(), "mode_psi_imag must be an MPS tensor");
    TORCH_CHECK(mode_anchor_idx.device().is_mps(), "mode_anchor_idx must be an MPS tensor");
    TORCH_CHECK(mode_anchor_weight.device().is_mps(), "mode_anchor_weight must be an MPS tensor");
    TORCH_CHECK(particle_pos.device().is_mps(), "particle_pos must be an MPS tensor");
    TORCH_CHECK(psi_re_field.device().is_mps(), "psi_re_field must be an MPS tensor");
    TORCH_CHECK(psi_im_field.device().is_mps(), "psi_im_field must be an MPS tensor");

    TORCH_CHECK(mode_psi_real.scalar_type() == torch::kFloat32, "mode_psi_real must be float32");
    TORCH_CHECK(mode_psi_imag.scalar_type() == torch::kFloat32, "mode_psi_imag must be float32");
    TORCH_CHECK(mode_anchor_weight.scalar_type() == torch::kFloat32, "mode_anchor_weight must be float32");
    TORCH_CHECK(particle_pos.scalar_type() == torch::kFloat32, "particle_pos must be float32");
    TORCH_CHECK(psi_re_field.scalar_type() == torch::kFloat32, "psi_re_field must be float32");
    TORCH_CHECK(psi_im_field.scalar_type() == torch::kFloat32, "psi_im_field must be float32");

    int64_t num_modes = mode_psi_real.numel();
    TORCH_CHECK(mode_psi_imag.numel() == num_modes, "mode_psi_imag length mismatch");
    TORCH_CHECK(mode_anchor_idx.numel() == num_modes * anchors_per_mode, "mode_anchor_idx length mismatch");
    TORCH_CHECK(mode_anchor_weight.numel() == num_modes * anchors_per_mode, "mode_anchor_weight length mismatch");

    int64_t num_particles = particle_pos.numel() / 3;
    TORCH_CHECK(particle_pos.numel() == num_particles * 3, "particle_pos must have shape (N,3) or flat length 3N");

    ModeProjectParams p;
    p.num_modes = (uint32_t)num_modes;
    p.num_particles = (uint32_t)num_particles;
    p.anchors_per_mode = (uint32_t)anchors_per_mode;
    p.grid_x = (uint32_t)grid_x;
    p.grid_y = (uint32_t)grid_y;
    p.grid_z = (uint32_t)grid_z;
    p.grid_spacing = (float)grid_spacing;
    p.inv_grid_spacing = (float)(1.0 / grid_spacing);

    int64_t total = num_modes * anchors_per_mode;

    ComputeKernel k(&g_pipeline_project_modes_to_spatial_psi, "project_modes_to_spatial_psi");
    k.set_tensor(mode_psi_real, 0);
    k.set_tensor(mode_psi_imag, 1);
    k.set_tensor(mode_anchor_idx, 2);
    k.set_tensor(mode_anchor_weight, 3);
    k.set_tensor(particle_pos, 4);
    k.set_tensor(psi_re_field, 5);
    k.set_tensor(psi_im_field, 6);
    k.set_bytes(p, 7);
    k.dispatch_1d(total);
}

void pic_gather_update_particles_pilot_wave(
    torch::Tensor particle_pos_in,
    torch::Tensor particle_mass,
    torch::Tensor particle_pos_out,
    torch::Tensor particle_vel_out,
    torch::Tensor psi_re_field,
    torch::Tensor psi_im_field,
    int64_t grid_x,
    int64_t grid_y,
    int64_t grid_z,
    double grid_spacing,
    double dt,
    double domain_x,
    double domain_y,
    double domain_z,
    double hbar_eff,
    double eps_denom,
    double mass_min
) {
    TORCH_CHECK(particle_pos_in.device().is_mps(), "particle_pos_in must be an MPS tensor");
    TORCH_CHECK(particle_mass.device().is_mps(), "particle_mass must be an MPS tensor");
    TORCH_CHECK(particle_pos_out.device().is_mps(), "particle_pos_out must be an MPS tensor");
    TORCH_CHECK(particle_vel_out.device().is_mps(), "particle_vel_out must be an MPS tensor");
    TORCH_CHECK(psi_re_field.device().is_mps(), "psi_re_field must be an MPS tensor");
    TORCH_CHECK(psi_im_field.device().is_mps(), "psi_im_field must be an MPS tensor");

    TORCH_CHECK(particle_pos_in.scalar_type() == torch::kFloat32, "particle_pos_in must be float32");
    TORCH_CHECK(particle_mass.scalar_type() == torch::kFloat32, "particle_mass must be float32");
    TORCH_CHECK(particle_pos_out.scalar_type() == torch::kFloat32, "particle_pos_out must be float32");
    TORCH_CHECK(particle_vel_out.scalar_type() == torch::kFloat32, "particle_vel_out must be float32");
    TORCH_CHECK(psi_re_field.scalar_type() == torch::kFloat32, "psi_re_field must be float32");
    TORCH_CHECK(psi_im_field.scalar_type() == torch::kFloat32, "psi_im_field must be float32");

    int64_t N = particle_pos_in.numel() / 3;
    TORCH_CHECK(particle_pos_in.numel() == N * 3, "particle_pos_in must have length 3N");
    TORCH_CHECK(particle_mass.numel() == N, "particle_mass must have length N");
    TORCH_CHECK(particle_pos_out.numel() == N * 3, "particle_pos_out must have length 3N");
    TORCH_CHECK(particle_vel_out.numel() == N * 3, "particle_vel_out must have length 3N");

    PilotWaveParams p;
    p.num_particles = (uint32_t)N;
    p.grid_x = (uint32_t)grid_x;
    p.grid_y = (uint32_t)grid_y;
    p.grid_z = (uint32_t)grid_z;
    p.grid_spacing = (float)grid_spacing;
    p.inv_grid_spacing = (float)(1.0 / grid_spacing);
    p.dt = (float)dt;
    p.domain_x = (float)domain_x;
    p.domain_y = (float)domain_y;
    p.domain_z = (float)domain_z;
    p.hbar_eff = (float)hbar_eff;
    p.eps_denom = (float)eps_denom;
    p.mass_min = (float)mass_min;

    ComputeKernel k(&g_pipeline_pic_gather_update_particles_pilot_wave, "pic_gather_update_particles_pilot_wave");
    k.set_tensor(particle_pos_in, 0);
    k.set_tensor(particle_mass, 1);
    k.set_tensor(particle_pos_out, 2);
    k.set_tensor(particle_vel_out, 3);
    k.set_tensor(psi_re_field, 4);
    k.set_tensor(psi_im_field, 5);
    k.set_bytes(p, 6);
    k.dispatch_1d(N);
}

// ============================================================================
// Compressible ideal-gas grid update (RK2, dual-energy internal e_int field)
// ============================================================================

void gas_rk2_stage1(
    at::Tensor rho0,     // (X, Y, Z) fp32 MPS
    at::Tensor mom0,     // (X, Y, Z, 3) fp32 MPS
    at::Tensor e0,       // (X, Y, Z) fp32 MPS (internal energy density)
    at::Tensor rho1,     // (X, Y, Z) fp32 MPS out
    at::Tensor mom1,     // (X, Y, Z, 3) fp32 MPS out
    at::Tensor e1,       // (X, Y, Z) fp32 MPS out
    at::Tensor k1_rho,   // (X, Y, Z) fp32 MPS out
    at::Tensor k1_mom,   // (X, Y, Z, 3) fp32 MPS out
    at::Tensor k1_e,     // (X, Y, Z) fp32 MPS out
    at::Tensor dbg_head_u32,  // (1,) int32/u32 MPS (atomic counter)
    at::Tensor dbg_words_u32, // (cap*6,) int32/u32 MPS (debug words)
    int64_t dbg_capacity,     // number of debug events (0 disables)
    int64_t grid_x,
    int64_t grid_y,
    int64_t grid_z,
    double dx,
    double dt,
    double gamma,
    double c_v,
    double rho_min,
    double p_min,
    double mu,
    double k_thermal) {

  TORCH_CHECK(rho0.device().is_mps(), "gas_rk2_stage1: rho0 must be on MPS");
  TORCH_CHECK(rho0.scalar_type() == at::kFloat, "gas_rk2_stage1: rho0 must be float32");
  TORCH_CHECK(dbg_capacity >= 0, "gas_rk2_stage1: dbg_capacity must be >= 0");
  TORCH_CHECK(dbg_head_u32.device().is_mps(), "gas_rk2_stage1: dbg_head_u32 must be on MPS");
  TORCH_CHECK(dbg_words_u32.device().is_mps(), "gas_rk2_stage1: dbg_words_u32 must be on MPS");
  TORCH_CHECK(dbg_head_u32.is_contiguous(), "gas_rk2_stage1: dbg_head_u32 must be contiguous");
  TORCH_CHECK(dbg_words_u32.is_contiguous(), "gas_rk2_stage1: dbg_words_u32 must be contiguous");
  TORCH_CHECK(dbg_head_u32.numel() >= 1, "gas_rk2_stage1: dbg_head_u32 must have at least 1 element");
  if (dbg_capacity > 0) {
    TORCH_CHECK(dbg_words_u32.numel() >= dbg_capacity * 6, "gas_rk2_stage1: dbg_words_u32 too small for dbg_capacity");
  }
  TORCH_CHECK(rho0.is_contiguous(), "gas_rk2_stage1: rho0 must be contiguous");
  TORCH_CHECK(mom0.is_contiguous(), "gas_rk2_stage1: mom0 must be contiguous");
  TORCH_CHECK(e0.is_contiguous(), "gas_rk2_stage1: e0 must be contiguous");
  TORCH_CHECK(rho1.is_contiguous(), "gas_rk2_stage1: rho1 must be contiguous");
  TORCH_CHECK(mom1.is_contiguous(), "gas_rk2_stage1: mom1 must be contiguous");
  TORCH_CHECK(e1.is_contiguous(), "gas_rk2_stage1: e1 must be contiguous");
  TORCH_CHECK(k1_rho.is_contiguous(), "gas_rk2_stage1: k1_rho must be contiguous");
  TORCH_CHECK(k1_mom.is_contiguous(), "gas_rk2_stage1: k1_mom must be contiguous");
  TORCH_CHECK(k1_e.is_contiguous(), "gas_rk2_stage1: k1_e must be contiguous");

  const int64_t N = grid_x * grid_y * grid_z;
  TORCH_CHECK(N > 0, "gas_rk2_stage1: grid dims must be positive");
  TORCH_CHECK(rho0.numel() == N, "gas_rk2_stage1: rho0 numel mismatch");
  TORCH_CHECK(e0.numel() == N, "gas_rk2_stage1: e0 numel mismatch");
  TORCH_CHECK(mom0.numel() == N * 3, "gas_rk2_stage1: mom0 numel mismatch");
  TORCH_CHECK(rho1.numel() == N && e1.numel() == N && k1_rho.numel() == N && k1_e.numel() == N,
              "gas_rk2_stage1: scalar field buffer sizes must match grid");
  TORCH_CHECK(mom1.numel() == N * 3 && k1_mom.numel() == N * 3,
              "gas_rk2_stage1: momentum field buffer sizes must match grid");

  ComputeKernel k(&g_pipeline_gas_rk2_stage1, "gas_rk2_stage1");
  k.set_tensor(rho0, 0);
  k.set_tensor(mom0, 1);
  k.set_tensor(e0, 2);
  k.set_tensor(rho1, 3);
  k.set_tensor(mom1, 4);
  k.set_tensor(e1, 5);
  k.set_tensor(k1_rho, 6);
  k.set_tensor(k1_mom, 7);
  k.set_tensor(k1_e, 8);
  k.set_tensor(dbg_head_u32, 10);
  k.set_tensor(dbg_words_u32, 11);

  GasGridParams prm;
  prm.num_cells = (uint32_t)N;
  prm.grid_x = (uint32_t)grid_x;
  prm.grid_y = (uint32_t)grid_y;
  prm.grid_z = (uint32_t)grid_z;
  prm.dx = (float)dx;
  prm.dt = (float)dt;
  prm.gamma = (float)gamma;
  prm.c_v = (float)c_v;
  prm.rho_min = (float)rho_min;
  prm.p_min = (float)p_min;
  prm.mu = (float)mu;
  prm.k_thermal = (float)k_thermal;
  k.set_bytes(prm, 9);

  const uint32_t cap_u32 = (dbg_capacity > 0) ? (uint32_t)dbg_capacity : 0u;
  k.set_bytes(cap_u32, 12);
  k.dispatch_1d(N);
}

void gas_rk2_stage2(
    at::Tensor rho0,     // (X, Y, Z) fp32 MPS (input, may alias output)
    at::Tensor mom0,     // (X, Y, Z, 3) fp32 MPS (input, may alias output)
    at::Tensor e0,       // (X, Y, Z) fp32 MPS (input, may alias output)
    at::Tensor rho1,     // (X, Y, Z) fp32 MPS
    at::Tensor mom1,     // (X, Y, Z, 3) fp32 MPS
    at::Tensor e1,       // (X, Y, Z) fp32 MPS
    at::Tensor k1_rho,   // (X, Y, Z) fp32 MPS
    at::Tensor k1_mom,   // (X, Y, Z, 3) fp32 MPS
    at::Tensor k1_e,     // (X, Y, Z) fp32 MPS
    at::Tensor rho_out,  // (X, Y, Z) fp32 MPS out
    at::Tensor mom_out,  // (X, Y, Z, 3) fp32 MPS out
    at::Tensor e_out,    // (X, Y, Z) fp32 MPS out
    at::Tensor dbg_head_u32,  // (1,) int32/u32 MPS (atomic counter)
    at::Tensor dbg_words_u32, // (cap*6,) int32/u32 MPS (debug words)
    int64_t dbg_capacity,     // number of debug events (0 disables)
    int64_t grid_x,
    int64_t grid_y,
    int64_t grid_z,
    double dx,
    double dt,
    double gamma,
    double c_v,
    double rho_min,
    double p_min,
    double mu,
    double k_thermal) {

  TORCH_CHECK(rho0.device().is_mps(), "gas_rk2_stage2: rho0 must be on MPS");
  TORCH_CHECK(rho0.scalar_type() == at::kFloat, "gas_rk2_stage2: rho0 must be float32");
  TORCH_CHECK(dbg_capacity >= 0, "gas_rk2_stage2: dbg_capacity must be >= 0");
  TORCH_CHECK(dbg_head_u32.device().is_mps(), "gas_rk2_stage2: dbg_head_u32 must be on MPS");
  TORCH_CHECK(dbg_words_u32.device().is_mps(), "gas_rk2_stage2: dbg_words_u32 must be on MPS");
  TORCH_CHECK(dbg_head_u32.is_contiguous(), "gas_rk2_stage2: dbg_head_u32 must be contiguous");
  TORCH_CHECK(dbg_words_u32.is_contiguous(), "gas_rk2_stage2: dbg_words_u32 must be contiguous");
  TORCH_CHECK(dbg_head_u32.numel() >= 1, "gas_rk2_stage2: dbg_head_u32 must have at least 1 element");
  if (dbg_capacity > 0) {
    TORCH_CHECK(dbg_words_u32.numel() >= dbg_capacity * 6, "gas_rk2_stage2: dbg_words_u32 too small for dbg_capacity");
  }
  TORCH_CHECK(rho_out.is_contiguous(), "gas_rk2_stage2: rho_out must be contiguous");
  TORCH_CHECK(mom_out.is_contiguous(), "gas_rk2_stage2: mom_out must be contiguous");
  TORCH_CHECK(e_out.is_contiguous(), "gas_rk2_stage2: e_out must be contiguous");

  const int64_t N = grid_x * grid_y * grid_z;
  TORCH_CHECK(N > 0, "gas_rk2_stage2: grid dims must be positive");

  ComputeKernel k(&g_pipeline_gas_rk2_stage2, "gas_rk2_stage2");
  k.set_tensor(rho0, 0);
  k.set_tensor(mom0, 1);
  k.set_tensor(e0, 2);
  k.set_tensor(rho1, 3);
  k.set_tensor(mom1, 4);
  k.set_tensor(e1, 5);
  k.set_tensor(k1_rho, 6);
  k.set_tensor(k1_mom, 7);
  k.set_tensor(k1_e, 8);
  k.set_tensor(rho_out, 9);
  k.set_tensor(mom_out, 10);
  k.set_tensor(e_out, 11);
  k.set_tensor(dbg_head_u32, 13);
  k.set_tensor(dbg_words_u32, 14);

  GasGridParams prm;
  prm.num_cells = (uint32_t)N;
  prm.grid_x = (uint32_t)grid_x;
  prm.grid_y = (uint32_t)grid_y;
  prm.grid_z = (uint32_t)grid_z;
  prm.dx = (float)dx;
  prm.dt = (float)dt;
  prm.gamma = (float)gamma;
  prm.c_v = (float)c_v;
  prm.rho_min = (float)rho_min;
  prm.p_min = (float)p_min;
  prm.mu = (float)mu;
  prm.k_thermal = (float)k_thermal;
  k.set_bytes(prm, 12);

  const uint32_t cap_u32 = (dbg_capacity > 0) ? (uint32_t)dbg_capacity : 0u;
  k.set_bytes(cap_u32, 15);
  k.dispatch_1d(N);
}

// ============================================================================
// Spatial Hash Grid Acceleration (O(N) collisions)
// ============================================================================

void spatial_hash_assign(
    at::Tensor particle_pos,       // (N, 3) fp32 MPS
    at::Tensor particle_cell_idx,  // (N,) uint32 MPS out
    at::Tensor cell_counts,        // (num_cells,) uint32 MPS out (atomic)
    int64_t grid_x,
    int64_t grid_y,
    int64_t grid_z,
    double cell_size,
    double domain_min_x,
    double domain_min_y,
    double domain_min_z) {
  
  const int64_t N = particle_pos.size(0);
  if (N == 0) return;
  
  TORCH_CHECK(particle_pos.device().is_mps(), "spatial_hash_assign: particle_pos must be on MPS");
  ComputeKernel k(&g_pipeline_spatial_hash_assign, "spatial_hash_assign");
  k.set_tensor(particle_pos, 0);
  k.set_tensor(particle_cell_idx, 1);
  k.set_tensor(cell_counts, 2);
  
  SpatialHashParams prm;
  prm.num_particles = (uint32_t)N;
  prm.grid_x = (uint32_t)grid_x;
  prm.grid_y = (uint32_t)grid_y;
  prm.grid_z = (uint32_t)grid_z;
  prm.cell_size = (float)cell_size;
  prm.inv_cell_size = 1.0f / (float)cell_size;
  prm.domain_min_x = (float)domain_min_x;
  prm.domain_min_y = (float)domain_min_y;
  prm.domain_min_z = (float)domain_min_z;
  k.set_bytes(prm, 3);
  k.dispatch_1d(N);
}

// Forward declarations (used by spatial_hash_prefix_sum implementation below).
void exclusive_scan_u32_pass1(at::Tensor in, at::Tensor out, at::Tensor block_sums, int64_t n);
void exclusive_scan_u32_add_block_offsets(at::Tensor out, at::Tensor block_prefix, int64_t n);
void exclusive_scan_u32_finalize_total(at::Tensor in, at::Tensor out, int64_t n);

void spatial_hash_prefix_sum(
    at::Tensor cell_counts,        // (num_cells,) uint32 MPS
    at::Tensor cell_starts,        // (num_cells + 1,) uint32 MPS out
    int64_t num_cells) {
  
  if (num_cells == 0) return;

  // [CHOICE] GPU hierarchical exclusive scan (no single-thread bottleneck)
  // [FORMULA] cell_starts[i] = Σ_{j<i} cell_counts[j], and cell_starts[num_cells] = total
  // [REASON] spatial hash must scale to large grids/particle counts
  TORCH_CHECK(cell_counts.device().is_mps(), "spatial_hash_prefix_sum: cell_counts must be on MPS");
  TORCH_CHECK(cell_starts.device().is_mps(), "spatial_hash_prefix_sum: cell_starts must be on MPS");
  TORCH_CHECK(cell_counts.dtype() == at::kInt, "spatial_hash_prefix_sum: cell_counts must be int32 (uint32 semantics)");
  TORCH_CHECK(cell_starts.dtype() == at::kInt, "spatial_hash_prefix_sum: cell_starts must be int32 (uint32 semantics)");
  check_contig_1d(cell_counts, "spatial_hash_prefix_sum: cell_counts");
  check_contig_1d(cell_starts, "spatial_hash_prefix_sum: cell_starts");
  TORCH_CHECK(cell_counts.numel() >= num_cells, "spatial_hash_prefix_sum: cell_counts too small");
  TORCH_CHECK(cell_starts.numel() >= (num_cells + 1), "spatial_hash_prefix_sum: cell_starts must be length num_cells+1");

  const int64_t tg = (int64_t)kThreadsPerThreadgroup;
  auto num_blocks = [&](int64_t n) { return (n + tg - 1) / tg; };

  // We allow 2-level or multi-level scans by allocating per-level buffers.
  std::vector<at::Tensor> level_out;
  std::vector<at::Tensor> level_block_sums;

  // Level 0: out writes directly into cell_starts[0:num_cells]
  at::Tensor out0 = cell_starts.narrow(0, 0, num_cells);
  level_out.push_back(out0);

  int64_t cur_n = num_cells;
  at::Tensor cur_in = cell_counts;
  int level = 0;
  while (true) {
    int64_t nb = num_blocks(cur_n);
    at::Tensor bs = at::empty({nb}, cell_counts.options());
    level_block_sums.push_back(bs);

    if (level == 0) {
      exclusive_scan_u32_pass1(cur_in, level_out[0], bs, cur_n);
    } else {
      at::Tensor out = at::empty({cur_n}, cell_counts.options());
      level_out.push_back(out);
      exclusive_scan_u32_pass1(cur_in, out, bs, cur_n);
    }

    if (nb <= 1) break;
    cur_in = bs;
    cur_n = nb;
    level += 1;
  }

  // Backward: add block offsets down the levels.
  // level_out[i+1] is exclusive scan of level_block_sums[i].
  for (int i = (int)level_out.size() - 2; i >= 0; i--) {
    exclusive_scan_u32_add_block_offsets(level_out[i], level_out[i + 1], (int64_t)level_out[i].numel());
  }

  // Finalize total sum into cell_starts[num_cells]
  exclusive_scan_u32_finalize_total(cell_counts, cell_starts, num_cells);
}

void spatial_hash_scatter(
    at::Tensor particle_cell_idx,  // (N,) uint32 MPS
    at::Tensor sorted_particle_idx,// (N,) uint32 MPS out
    at::Tensor cell_offsets,       // (num_cells,) uint32 MPS (working copy of cell_starts)
    int64_t num_particles) {
  
  if (num_particles == 0) return;
  ComputeKernel k(&g_pipeline_spatial_hash_scatter, "spatial_hash_scatter");
  k.set_tensor(particle_cell_idx, 0);
  k.set_tensor(sorted_particle_idx, 1);
  k.set_tensor(cell_offsets, 2);
  const uint32_t np = (uint32_t)num_particles;
  k.set_bytes(np, 3);
  k.dispatch_1d(num_particles);
}

// ============================================================================
// Generic u32 Exclusive Scan (building blocks; no allocations)
// ============================================================================
// Exposes low-level passes so Python can run a hierarchical scan without host sync.
void exclusive_scan_u32_pass1(
    at::Tensor in,         // (n,) int32 MPS (treated as uint32)
    at::Tensor out,        // (n,) int32 MPS out
    at::Tensor block_sums, // (num_blocks,) int32 MPS out
    int64_t n) {

  TORCH_CHECK(n >= 0, "exclusive_scan_u32_pass1: n must be >= 0, got ", n);
  if (n == 0) return;

  TORCH_CHECK(in.device().is_mps(), "exclusive_scan_u32_pass1: in must be on MPS");
  TORCH_CHECK(out.device().is_mps(), "exclusive_scan_u32_pass1: out must be on MPS");
  TORCH_CHECK(block_sums.device().is_mps(), "exclusive_scan_u32_pass1: block_sums must be on MPS");
  TORCH_CHECK(in.dtype() == at::kInt, "exclusive_scan_u32_pass1: in must be int32 (uint32 semantics)");
  TORCH_CHECK(out.dtype() == at::kInt, "exclusive_scan_u32_pass1: out must be int32 (uint32 semantics)");
  TORCH_CHECK(block_sums.dtype() == at::kInt, "exclusive_scan_u32_pass1: block_sums must be int32 (uint32 semantics)");
  check_contig_1d(in, "exclusive_scan_u32_pass1: in");
  check_contig_1d(out, "exclusive_scan_u32_pass1: out");
  check_contig_1d(block_sums, "exclusive_scan_u32_pass1: block_sums");
  TORCH_CHECK(in.numel() >= n, "exclusive_scan_u32_pass1: in.numel() < n");
  TORCH_CHECK(out.numel() >= n, "exclusive_scan_u32_pass1: out.numel() < n");

  const int64_t num_groups = (n + (int64_t)kThreadsPerThreadgroup - 1) / (int64_t)kThreadsPerThreadgroup;
  TORCH_CHECK(block_sums.numel() >= num_groups, "exclusive_scan_u32_pass1: block_sums too small");

  ComputeKernel k(&g_pipeline_exclusive_scan_u32_pass1, "exclusive_scan_u32_pass1");
  k.set_tensor(in, 0);
  k.set_tensor(out, 1);
  k.set_tensor(block_sums, 2);
  const uint32_t n_u = (uint32_t)n;
  k.set_bytes(n_u, 3);
  k.set_threadgroup_memory_length((NSUInteger)(kThreadsPerThreadgroup * sizeof(uint32_t)), 0);
  k.dispatch_groups((NSUInteger)num_groups, (NSUInteger)kThreadsPerThreadgroup);
}

void exclusive_scan_u32_add_block_offsets(
    at::Tensor out,          // (n,) int32 MPS in/out
    at::Tensor block_prefix, // (num_blocks,) int32 MPS (exclusive scan of block_sums)
    int64_t n) {

  TORCH_CHECK(n >= 0, "exclusive_scan_u32_add_block_offsets: n must be >= 0, got ", n);
  if (n == 0) return;

  TORCH_CHECK(out.device().is_mps(), "exclusive_scan_u32_add_block_offsets: out must be on MPS");
  TORCH_CHECK(block_prefix.device().is_mps(), "exclusive_scan_u32_add_block_offsets: block_prefix must be on MPS");
  TORCH_CHECK(out.dtype() == at::kInt, "exclusive_scan_u32_add_block_offsets: out must be int32");
  TORCH_CHECK(block_prefix.dtype() == at::kInt, "exclusive_scan_u32_add_block_offsets: block_prefix must be int32");
  check_contig_1d(out, "exclusive_scan_u32_add_block_offsets: out");
  check_contig_1d(block_prefix, "exclusive_scan_u32_add_block_offsets: block_prefix");
  TORCH_CHECK(out.numel() >= n, "exclusive_scan_u32_add_block_offsets: out.numel() < n");

  const int64_t num_groups = (n + (int64_t)kThreadsPerThreadgroup - 1) / (int64_t)kThreadsPerThreadgroup;
  TORCH_CHECK(block_prefix.numel() >= num_groups, "exclusive_scan_u32_add_block_offsets: block_prefix too small");

  ComputeKernel k(&g_pipeline_exclusive_scan_u32_add_block_offsets, "exclusive_scan_u32_add_block_offsets");
  k.set_tensor(out, 0);
  k.set_tensor(block_prefix, 1);
  const uint32_t n_u = (uint32_t)n;
  k.set_bytes(n_u, 2);
  k.dispatch_groups((NSUInteger)num_groups, (NSUInteger)kThreadsPerThreadgroup);
}

void exclusive_scan_u32_finalize_total(
    at::Tensor in,   // (n,) int32 MPS
    at::Tensor out,  // (n+1,) int32 MPS (first n entries already exclusive-scanned)
    int64_t n) {

  TORCH_CHECK(n >= 0, "exclusive_scan_u32_finalize_total: n must be >= 0, got ", n);
  TORCH_CHECK(out.device().is_mps(), "exclusive_scan_u32_finalize_total: out must be on MPS");
  TORCH_CHECK(in.device().is_mps(), "exclusive_scan_u32_finalize_total: in must be on MPS");
  TORCH_CHECK(in.dtype() == at::kInt, "exclusive_scan_u32_finalize_total: in must be int32");
  TORCH_CHECK(out.dtype() == at::kInt, "exclusive_scan_u32_finalize_total: out must be int32");
  check_contig_1d(in, "exclusive_scan_u32_finalize_total: in");
  check_contig_1d(out, "exclusive_scan_u32_finalize_total: out");
  TORCH_CHECK(in.numel() >= n, "exclusive_scan_u32_finalize_total: in.numel() < n");
  TORCH_CHECK(out.numel() >= (n + 1), "exclusive_scan_u32_finalize_total: out must have length >= n+1");

  ComputeKernel k(&g_pipeline_exclusive_scan_u32_finalize_total, "exclusive_scan_u32_finalize_total");
  k.set_tensor(in, 0);
  k.set_tensor(out, 1);
  const uint32_t n_u = (uint32_t)n;
  k.set_bytes(n_u, 2);
  k.dispatch_groups(1u, 1u);
}

// ============================================================================
// Coherence ω-binning (GPU-only)
// ============================================================================
void coherence_reduce_omega_minmax_keys(
    at::Tensor carrier_omega,         // (maxM,) fp32 MPS
    at::Tensor num_carriers_snapshot, // (1,) int32 MPS
    at::Tensor omega_min_key,         // (1,) int32 MPS (init = -1 / 0xFFFFFFFF)
    at::Tensor omega_max_key) {       // (1,) int32 MPS (init = 0)

  TORCH_CHECK(carrier_omega.device().is_mps(), "coherence_reduce_omega_minmax_keys: carrier_omega must be on MPS");
  TORCH_CHECK(num_carriers_snapshot.device().is_mps(), "coherence_reduce_omega_minmax_keys: num_carriers_snapshot must be on MPS");
  TORCH_CHECK(omega_min_key.device().is_mps(), "coherence_reduce_omega_minmax_keys: omega_min_key must be on MPS");
  TORCH_CHECK(omega_max_key.device().is_mps(), "coherence_reduce_omega_minmax_keys: omega_max_key must be on MPS");
  TORCH_CHECK(carrier_omega.dtype() == at::kFloat, "coherence_reduce_omega_minmax_keys: carrier_omega must be fp32");
  TORCH_CHECK(num_carriers_snapshot.dtype() == at::kInt, "coherence_reduce_omega_minmax_keys: num_carriers_snapshot must be int32");
  TORCH_CHECK(omega_min_key.dtype() == at::kInt, "coherence_reduce_omega_minmax_keys: omega_min_key must be int32");
  TORCH_CHECK(omega_max_key.dtype() == at::kInt, "coherence_reduce_omega_minmax_keys: omega_max_key must be int32");
  check_contig_1d(carrier_omega, "coherence_reduce_omega_minmax_keys: carrier_omega");
  check_contig_1d(num_carriers_snapshot, "coherence_reduce_omega_minmax_keys: num_carriers_snapshot");
  check_contig_1d(omega_min_key, "coherence_reduce_omega_minmax_keys: omega_min_key");
  check_contig_1d(omega_max_key, "coherence_reduce_omega_minmax_keys: omega_max_key");
  TORCH_CHECK(omega_min_key.numel() == 1, "coherence_reduce_omega_minmax_keys: omega_min_key must be (1,)");
  TORCH_CHECK(omega_max_key.numel() == 1, "coherence_reduce_omega_minmax_keys: omega_max_key must be (1,)");

  const int64_t maxM = carrier_omega.numel();
  if (maxM == 0) return;
  ComputeKernel k(&g_pipeline_coherence_reduce_omega_minmax_keys, "coherence_reduce_omega_minmax_keys");
  k.set_tensor(carrier_omega, 0);
  k.set_tensor(num_carriers_snapshot, 1);
  k.set_tensor(omega_min_key, 2);
  k.set_tensor(omega_max_key, 3);
  k.dispatch_1d(maxM);
}

void coherence_compute_bin_params(
    at::Tensor omega_min_key,         // (1,) int32 MPS
    at::Tensor omega_max_key,         // (1,) int32 MPS
    at::Tensor num_carriers_snapshot, // (1,) int32 MPS
    at::Tensor bin_params_out,        // (2,) fp32 MPS [omega_min, inv_bin_width]
    double gate_width_max) {

  TORCH_CHECK(omega_min_key.device().is_mps(), "coherence_compute_bin_params: omega_min_key must be on MPS");
  TORCH_CHECK(omega_max_key.device().is_mps(), "coherence_compute_bin_params: omega_max_key must be on MPS");
  TORCH_CHECK(num_carriers_snapshot.device().is_mps(), "coherence_compute_bin_params: num_carriers_snapshot must be on MPS");
  TORCH_CHECK(bin_params_out.device().is_mps(), "coherence_compute_bin_params: bin_params_out must be on MPS");
  TORCH_CHECK(omega_min_key.dtype() == at::kInt, "coherence_compute_bin_params: omega_min_key must be int32");
  TORCH_CHECK(omega_max_key.dtype() == at::kInt, "coherence_compute_bin_params: omega_max_key must be int32");
  TORCH_CHECK(num_carriers_snapshot.dtype() == at::kInt, "coherence_compute_bin_params: num_carriers_snapshot must be int32");
  TORCH_CHECK(bin_params_out.dtype() == at::kFloat, "coherence_compute_bin_params: bin_params_out must be fp32");
  check_contig_1d(omega_min_key, "coherence_compute_bin_params: omega_min_key");
  check_contig_1d(omega_max_key, "coherence_compute_bin_params: omega_max_key");
  check_contig_1d(num_carriers_snapshot, "coherence_compute_bin_params: num_carriers_snapshot");
  check_contig_1d(bin_params_out, "coherence_compute_bin_params: bin_params_out");
  TORCH_CHECK(omega_min_key.numel() == 1, "coherence_compute_bin_params: omega_min_key must be (1,)");
  TORCH_CHECK(omega_max_key.numel() == 1, "coherence_compute_bin_params: omega_max_key must be (1,)");
  TORCH_CHECK(bin_params_out.numel() >= 2, "coherence_compute_bin_params: bin_params_out must have >=2 floats");
  ComputeKernel k(&g_pipeline_coherence_compute_bin_params, "coherence_compute_bin_params");
  k.set_tensor(omega_min_key, 0);
  k.set_tensor(omega_max_key, 1);
  k.set_tensor(num_carriers_snapshot, 2);
  // bin_params_out is a float buffer; Metal reads it as SpectralBinParams {float,float}
  k.set_tensor(bin_params_out, 3);
  const float gw_max = (float)gate_width_max;
  k.set_bytes(gw_max, 4);
  k.dispatch_groups(1u, 1u);
}

void coherence_bin_count_carriers(
    at::Tensor carrier_omega,         // (maxM,) fp32 MPS
    at::Tensor num_carriers_snapshot, // (1,) int32 MPS
    at::Tensor bin_counts,            // (num_bins,) int32 MPS (zeroed before call)
    at::Tensor bin_params,            // (2,) fp32 MPS
    int64_t num_bins) {

  TORCH_CHECK(num_bins > 0, "coherence_bin_count: num_bins must be > 0");
  TORCH_CHECK(carrier_omega.device().is_mps(), "coherence_bin_count: carrier_omega must be on MPS");
  TORCH_CHECK(num_carriers_snapshot.device().is_mps(), "coherence_bin_count: num_carriers_snapshot must be on MPS");
  TORCH_CHECK(bin_counts.device().is_mps(), "coherence_bin_count: bin_counts must be on MPS");
  TORCH_CHECK(bin_params.device().is_mps(), "coherence_bin_count: bin_params must be on MPS");
  TORCH_CHECK(carrier_omega.dtype() == at::kFloat, "coherence_bin_count: carrier_omega must be fp32");
  TORCH_CHECK(num_carriers_snapshot.dtype() == at::kInt, "coherence_bin_count: num_carriers_snapshot must be int32");
  TORCH_CHECK(bin_counts.dtype() == at::kInt, "coherence_bin_count: bin_counts must be int32");
  TORCH_CHECK(bin_params.dtype() == at::kFloat, "coherence_bin_count: bin_params must be fp32");
  check_contig_1d(carrier_omega, "coherence_bin_count: carrier_omega");
  check_contig_1d(num_carriers_snapshot, "coherence_bin_count: num_carriers_snapshot");
  check_contig_1d(bin_counts, "coherence_bin_count: bin_counts");
  check_contig_1d(bin_params, "coherence_bin_count: bin_params");
  TORCH_CHECK(bin_counts.numel() >= num_bins, "coherence_bin_count: bin_counts too small");

  const int64_t maxM = carrier_omega.numel();
  if (maxM == 0) return;
  ComputeKernel k(&g_pipeline_coherence_bin_count_carriers, "coherence_bin_count_carriers");
  k.set_tensor(carrier_omega, 0);
  k.set_tensor(num_carriers_snapshot, 1);
  k.set_tensor(bin_counts, 2);
  k.set_tensor(bin_params, 3);
  const uint32_t nb = (uint32_t)num_bins;
  k.set_bytes(nb, 4);
  k.dispatch_1d(maxM);
}

void coherence_bin_scatter_carriers(
    at::Tensor carrier_omega,         // (maxM,) fp32 MPS
    at::Tensor num_carriers_snapshot, // (1,) int32 MPS
    at::Tensor bin_offsets,           // (num_bins,) int32 MPS (working copy of bin_starts)
    at::Tensor bin_params,            // (2,) fp32 MPS
    int64_t num_bins,
    at::Tensor carrier_binned_idx) {  // (maxM,) int32 MPS out

  TORCH_CHECK(num_bins > 0, "coherence_bin_scatter: num_bins must be > 0");
  TORCH_CHECK(carrier_omega.device().is_mps(), "coherence_bin_scatter: carrier_omega must be on MPS");
  TORCH_CHECK(num_carriers_snapshot.device().is_mps(), "coherence_bin_scatter: num_carriers_snapshot must be on MPS");
  TORCH_CHECK(bin_offsets.device().is_mps(), "coherence_bin_scatter: bin_offsets must be on MPS");
  TORCH_CHECK(bin_params.device().is_mps(), "coherence_bin_scatter: bin_params must be on MPS");
  TORCH_CHECK(carrier_binned_idx.device().is_mps(), "coherence_bin_scatter: carrier_binned_idx must be on MPS");
  TORCH_CHECK(carrier_omega.dtype() == at::kFloat, "coherence_bin_scatter: carrier_omega must be fp32");
  TORCH_CHECK(num_carriers_snapshot.dtype() == at::kInt, "coherence_bin_scatter: num_carriers_snapshot must be int32");
  TORCH_CHECK(bin_offsets.dtype() == at::kInt, "coherence_bin_scatter: bin_offsets must be int32");
  TORCH_CHECK(bin_params.dtype() == at::kFloat, "coherence_bin_scatter: bin_params must be fp32");
  TORCH_CHECK(carrier_binned_idx.dtype() == at::kInt, "coherence_bin_scatter: carrier_binned_idx must be int32");
  check_contig_1d(carrier_omega, "coherence_bin_scatter: carrier_omega");
  check_contig_1d(num_carriers_snapshot, "coherence_bin_scatter: num_carriers_snapshot");
  check_contig_1d(bin_offsets, "coherence_bin_scatter: bin_offsets");
  check_contig_1d(bin_params, "coherence_bin_scatter: bin_params");
  check_contig_1d(carrier_binned_idx, "coherence_bin_scatter: carrier_binned_idx");
  TORCH_CHECK(bin_offsets.numel() >= num_bins, "coherence_bin_scatter: bin_offsets too small");

  const int64_t maxM = carrier_omega.numel();
  if (maxM == 0) return;
  ComputeKernel k(&g_pipeline_coherence_bin_scatter_carriers, "coherence_bin_scatter_carriers");
  k.set_tensor(carrier_omega, 0);
  k.set_tensor(num_carriers_snapshot, 1);
  k.set_tensor(bin_offsets, 2);
  k.set_tensor(bin_params, 3);
  const uint32_t nb = (uint32_t)num_bins;
  k.set_bytes(nb, 4);
  k.set_tensor(carrier_binned_idx, 5);
  k.dispatch_1d(maxM);
}

void spatial_hash_collisions(
    at::Tensor particle_pos,       // (N, 3) fp32 MPS
    at::Tensor particle_vel,       // (N, 3) fp32 MPS in/out
    at::Tensor particle_excitation,// (N,) fp32 MPS in/out
    at::Tensor particle_mass,      // (N,) fp32 MPS
    at::Tensor particle_heat,      // (N,) fp32 MPS in/out
    at::Tensor sorted_particle_idx,// (N,) uint32 MPS
    at::Tensor cell_starts,        // (num_cells + 1,) uint32 MPS
    at::Tensor particle_cell_idx,  // (N,) uint32 MPS
    at::Tensor particle_vel_in,    // (N, 3) fp32 MPS (snapshot)
    at::Tensor particle_heat_in,   // (N,) fp32 MPS (snapshot)
    int64_t grid_x,
    int64_t grid_y,
    int64_t grid_z,
    double cell_size,
    double domain_min_x,
    double domain_min_y,
    double domain_min_z,
    double dt,
    double particle_radius,
    double young_modulus,
    double thermal_conductivity,
    double specific_heat,
    double restitution) {
  
  const int64_t N = particle_pos.size(0);
  if (N == 0) return;
  
  TORCH_CHECK(particle_pos.device().is_mps(), "spatial_hash_collisions: particle_pos must be on MPS");
  ComputeKernel k(&g_pipeline_spatial_hash_collisions, "spatial_hash_collisions");
  k.set_tensor(particle_pos, 0);
  k.set_tensor(particle_vel, 1);
  k.set_tensor(particle_excitation, 2);
  k.set_tensor(particle_mass, 3);
  k.set_tensor(particle_heat, 4);
  k.set_tensor(sorted_particle_idx, 5);
  k.set_tensor(cell_starts, 6);
  k.set_tensor(particle_cell_idx, 7);
  k.set_tensor(particle_vel_in, 8);
  k.set_tensor(particle_heat_in, 9);
  
  SpatialCollisionParams prm;
  prm.num_particles = (uint32_t)N;
  prm.grid_x = (uint32_t)grid_x;
  prm.grid_y = (uint32_t)grid_y;
  prm.grid_z = (uint32_t)grid_z;
  prm.cell_size = (float)cell_size;
  prm.inv_cell_size = 1.0f / (float)cell_size;
  prm.domain_min_x = (float)domain_min_x;
  prm.domain_min_y = (float)domain_min_y;
  prm.domain_min_z = (float)domain_min_z;
  prm.dt = (float)dt;
  prm.particle_radius = (float)particle_radius;
  prm.young_modulus = (float)young_modulus;
  prm.thermal_conductivity = (float)thermal_conductivity;
  prm.specific_heat = (float)specific_heat;
  prm.restitution = (float)restitution;
  k.set_bytes(prm, 10);
  k.dispatch_1d(N);
}

// -----------------------------------------------------------------------------
// Kernel: Parallel Force Accumulation (Oscillator-Centric)
// -----------------------------------------------------------------------------

void coherence_accumulate_forces_v2(
    at::Tensor osc_phase,
    at::Tensor osc_omega,
    at::Tensor osc_amp,
    at::Tensor particle_pos,        // (N,3) fp32 MPS
    at::Tensor carrier_omega,
    at::Tensor carrier_gate_width,
    at::Tensor carrier_anchor_idx,  // (maxM*anchors,) int32 MPS
    at::Tensor carrier_anchor_weight, // (maxM*anchors,) fp32 MPS
    at::Tensor accums,
    at::Tensor bin_starts,           // (num_bins+1,) int32 MPS
    at::Tensor carrier_binned_idx,   // (maxM,) int32 MPS
    at::Tensor bin_params,           // (2,) fp32 MPS
    int64_t num_bins,
    at::Tensor particle_heat,        // (N,) fp32 MPS in/out
    int64_t num_osc,
    at::Tensor num_carriers_snapshot, // (1,) int32 MPS
    int64_t max_carriers,
    double dt,
    double metabolic_rate,
    double gate_width_min,
    double gate_width_max,
    double offender_weight_floor,
    double domain_x,
    double domain_y,
    double domain_z,
    double spatial_sigma) {

  check_contig_1d(osc_phase, "coherence_accumulate_forces: osc_phase");
  check_contig_1d(osc_omega, "coherence_accumulate_forces: osc_omega");
  check_contig_1d(osc_amp, "coherence_accumulate_forces: osc_amp");
  TORCH_CHECK(particle_pos.device().is_mps(), "coherence_accumulate_forces: particle_pos must be on MPS");
  TORCH_CHECK(particle_pos.dtype() == at::kFloat, "coherence_accumulate_forces: particle_pos must be fp32");
  TORCH_CHECK(particle_pos.is_contiguous(), "coherence_accumulate_forces: particle_pos must be contiguous");
  TORCH_CHECK(particle_pos.dim() == 2 && particle_pos.size(1) == 3, "coherence_accumulate_forces: particle_pos must be (N,3)");
  check_contig_1d(carrier_omega, "coherence_accumulate_forces: carrier_omega");
  check_contig_1d(carrier_gate_width, "coherence_accumulate_forces: carrier_gate_width");
  check_contig_1d(carrier_anchor_idx, "coherence_accumulate_forces: carrier_anchor_idx");
  check_contig_1d(carrier_anchor_weight, "coherence_accumulate_forces: carrier_anchor_weight");
  TORCH_CHECK(carrier_anchor_idx.device().is_mps(), "coherence_accumulate_forces: carrier_anchor_idx must be on MPS");
  TORCH_CHECK(carrier_anchor_weight.device().is_mps(), "coherence_accumulate_forces: carrier_anchor_weight must be on MPS");
  TORCH_CHECK(carrier_anchor_idx.dtype() == at::kInt, "coherence_accumulate_forces: carrier_anchor_idx must be int32");
  TORCH_CHECK(carrier_anchor_weight.dtype() == at::kFloat, "coherence_accumulate_forces: carrier_anchor_weight must be fp32");
  
  TORCH_CHECK(accums.device().is_mps(), "coherence_accumulate_forces: accums must be on MPS");
  TORCH_CHECK(accums.dtype() == at::kInt, "coherence_accumulate_forces: accums must be int32");
  TORCH_CHECK(accums.is_contiguous(), "coherence_accumulate_forces: accums must be contiguous");

  TORCH_CHECK(bin_starts.device().is_mps(), "coherence_accumulate_forces: bin_starts must be on MPS");
  TORCH_CHECK(bin_starts.dtype() == at::kInt, "coherence_accumulate_forces: bin_starts must be int32");
  check_contig_1d(bin_starts, "coherence_accumulate_forces: bin_starts");
  TORCH_CHECK(carrier_binned_idx.device().is_mps(), "coherence_accumulate_forces: carrier_binned_idx must be on MPS");
  TORCH_CHECK(carrier_binned_idx.dtype() == at::kInt, "coherence_accumulate_forces: carrier_binned_idx must be int32");
  check_contig_1d(carrier_binned_idx, "coherence_accumulate_forces: carrier_binned_idx");
  TORCH_CHECK(bin_params.device().is_mps(), "coherence_accumulate_forces: bin_params must be on MPS");
  TORCH_CHECK(bin_params.dtype() == at::kFloat, "coherence_accumulate_forces: bin_params must be fp32");
  check_contig_1d(bin_params, "coherence_accumulate_forces: bin_params");
  TORCH_CHECK(num_bins > 0, "coherence_accumulate_forces: num_bins must be > 0");

  TORCH_CHECK(particle_heat.device().is_mps(), "coherence_accumulate_forces: particle_heat must be on MPS");
  TORCH_CHECK(particle_heat.dtype() == at::kFloat, "coherence_accumulate_forces: particle_heat must be fp32");
  check_contig_1d(particle_heat, "coherence_accumulate_forces: particle_heat");
  TORCH_CHECK(particle_heat.numel() == num_osc, "coherence_accumulate_forces: particle_heat must have length N");

  TORCH_CHECK(num_carriers_snapshot.device().is_mps(), "coherence_accumulate_forces: num_carriers_snapshot must be on MPS");
  TORCH_CHECK(num_carriers_snapshot.dtype() == at::kInt, "coherence_accumulate_forces: num_carriers_snapshot must be int32");
  check_contig_1d(num_carriers_snapshot, "coherence_accumulate_forces: num_carriers_snapshot");

  if (num_osc == 0 || max_carriers == 0) return;
  static id<MTLComputePipelineState> g_pipeline_coherence_accumulate_forces = nil;
  ComputeKernel k(&g_pipeline_coherence_accumulate_forces, "coherence_accumulate_forces");
  k.set_tensor(osc_phase, 0);
  k.set_tensor(osc_omega, 1);
  k.set_tensor(osc_amp, 2);
  k.set_tensor(particle_pos, 3);
  k.set_tensor(carrier_omega, 4);
  k.set_tensor(carrier_gate_width, 5);
  k.set_tensor(carrier_anchor_idx, 6);
  k.set_tensor(carrier_anchor_weight, 7);
  k.set_tensor(accums, 8);
  k.set_tensor(num_carriers_snapshot, 10);
  k.set_tensor(bin_starts, 11);
  k.set_tensor(carrier_binned_idx, 12);
  k.set_tensor(bin_params, 13);
  const uint32_t nb_u = (uint32_t)num_bins;
  k.set_bytes(nb_u, 14);
  k.set_tensor(particle_heat, 15);

  SpectralModeParams prm;
  memset(&prm, 0, sizeof(prm));
  prm.num_osc = (uint32_t)num_osc;
  prm.max_carriers = (uint32_t)max_carriers;
  prm.dt = (float)dt;
  prm.gate_width_min = (float)gate_width_min;
  prm.gate_width_max = (float)gate_width_max;
  prm.offender_weight_floor = (float)offender_weight_floor;
  prm.conflict_threshold = 0.0f;
  prm.domain_x = (float)domain_x;
  prm.domain_y = (float)domain_y;
  prm.domain_z = (float)domain_z;
  prm.spatial_sigma = (float)spatial_sigma;
  prm.metabolic_rate = (float)metabolic_rate;
  k.set_bytes(prm, 9);

  // Allocate threadgroup memory for carrier accumulators.
  // TGCarrierAccum has 8 fields × 4 bytes = 32 bytes per carrier.
  // kMaxCarriersForTG = 128 in Metal → 128 * 32 = 4096 bytes.
  // This reduces atomic contention from N (55M) to N/threadgroup_size (~860K).
  constexpr NSUInteger kMaxCarriersForTG = 128;
  constexpr NSUInteger kTGCarrierAccumSize = 32;  // 6 floats + 2 uints = 8 × 4 bytes
  k.set_threadgroup_memory_length(kMaxCarriersForTG * kTGCarrierAccumSize, 0);
  // Dispatch over oscillators (N)
  k.dispatch_1d(num_osc);
}

// ============================================================================
// Coherence field dynamics (GPE)
// ============================================================================

void coherence_gpe_step(
    at::Tensor osc_phase,            // (N,) fp32 MPS
    at::Tensor osc_omega,            // (N,) fp32 MPS
    at::Tensor osc_amp,              // (N,) fp32 MPS
    at::Tensor particle_pos,         // (N,3) fp32 MPS
    at::Tensor carrier_real,         // (maxM,) fp32 MPS in/out
    at::Tensor carrier_imag,         // (maxM,) fp32 MPS in/out
    at::Tensor carrier_omega,        // (maxM,) fp32 MPS
    at::Tensor carrier_gate_width,   // (maxM,) fp32 MPS
    at::Tensor carrier_anchor_idx,   // (maxM*anchors,) int32 MPS in/out
    at::Tensor carrier_anchor_weight,// (maxM*anchors,) fp32 MPS in/out
    at::Tensor accums,               // (maxM*8,) int32 MPS (CarrierAccumulators backing)
    at::Tensor num_carriers_snapshot,// (1,) int32 MPS snapshot
    int64_t max_carriers,
    double dt,
    double hbar_eff,
    double mass_eff,
    double g_interaction,
    double energy_decay,
    double chemical_potential,
    double inv_domega2,
    uint32_t rng_seed,
    double anchor_eps,
    double gate_width_min,
    double gate_width_max,
    double offender_weight_floor,
    double domain_x,
    double domain_y,
    double domain_z,
    double spatial_sigma,
    double metric_coupling,
    at::Tensor extra_potential) {

  check_contig_1d(osc_phase, "coherence_gpe_step: osc_phase");
// ... (skip lines to match context for packing)
// Wait, I can't skip lines easily with replace_file_content.
// I should use multi_replace for this file since I need to change signature AND packing.

  check_contig_1d(osc_omega, "coherence_gpe_step: osc_omega");
  check_contig_1d(osc_amp, "coherence_gpe_step: osc_amp");
  TORCH_CHECK(particle_pos.device().is_mps(), "coherence_gpe_step: particle_pos must be on MPS");
  TORCH_CHECK(particle_pos.dtype() == at::kFloat, "coherence_gpe_step: particle_pos must be fp32");
  TORCH_CHECK(particle_pos.is_contiguous(), "coherence_gpe_step: particle_pos must be contiguous");
  TORCH_CHECK(particle_pos.dim() == 2 && particle_pos.size(1) == 3, "coherence_gpe_step: particle_pos must be (N,3)");
  check_contig_1d(carrier_real, "coherence_gpe_step: carrier_real");
  check_contig_1d(carrier_imag, "coherence_gpe_step: carrier_imag");
  check_contig_1d(carrier_omega, "coherence_gpe_step: carrier_omega");
  check_contig_1d(carrier_gate_width, "coherence_gpe_step: carrier_gate_width");
  check_contig_1d(carrier_anchor_idx, "coherence_gpe_step: carrier_anchor_idx");
  check_contig_1d(carrier_anchor_weight, "coherence_gpe_step: carrier_anchor_weight");
  check_contig_1d(num_carriers_snapshot, "coherence_gpe_step: num_carriers_snapshot");
  TORCH_CHECK(num_carriers_snapshot.device().is_mps(), "coherence_gpe_step: num_carriers_snapshot must be on MPS");
  TORCH_CHECK(num_carriers_snapshot.dtype() == at::kInt, "coherence_gpe_step: num_carriers_snapshot must be int32");
  TORCH_CHECK(accums.device().is_mps(), "coherence_gpe_step: accums must be on MPS");
  TORCH_CHECK(accums.is_contiguous(), "coherence_gpe_step: accums must be contiguous");

  const int64_t N = osc_phase.size(0);
  if (N == 0 || max_carriers == 0) return;
  ComputeKernel k(&g_pipeline_coherence_gpe_step, "coherence_gpe_step");
  k.set_tensor(osc_phase, 0);
  k.set_tensor(osc_omega, 1);
  k.set_tensor(osc_amp, 2);
  k.set_tensor(carrier_real, 3);
  k.set_tensor(carrier_imag, 4);
  k.set_tensor(carrier_omega, 5);
  k.set_tensor(carrier_gate_width, 6);
  k.set_tensor(carrier_anchor_idx, 7);
  k.set_tensor(carrier_anchor_weight, 8);
  k.set_tensor(accums, 9);
  k.set_tensor(num_carriers_snapshot, 10);
  k.set_tensor(particle_pos, 11);

  SpectralModeParams prm;
  prm.num_osc = (uint32_t)N;
  prm.max_carriers = (uint32_t)max_carriers;
  prm.num_carriers = 0u;
  prm.dt = (float)dt;
  prm.coupling_scale = 0.0f;
  prm.carrier_reg = 0.0f;
  prm.rng_seed = rng_seed;
  prm.conflict_threshold = 0.0f;
  prm.offender_weight_floor = (float)offender_weight_floor;
  prm.gate_width_min = (float)gate_width_min;
  prm.gate_width_max = (float)gate_width_max;
  prm.ema_alpha = 0.0f;
  prm.recenter_alpha = 0.0f;
  prm.mode = 0u;
  prm.anchor_random_eps = (float)anchor_eps;
  prm.stable_amp_threshold = 0.0f;
  prm.crystallize_amp_threshold = 0.0f;
  prm.crystallize_conflict_threshold = 0.0f;
  prm.crystallize_age = 0u;
  prm.crystallized_coupling_boost = 0.0f;
  prm.volatile_decay_mul = 1.0f;
  prm.stable_decay_mul = 1.0f;
  prm.crystallized_decay_mul = 1.0f;
  prm.topdown_phase_scale = 0.0f;
  prm.topdown_energy_scale = 0.0f;
  prm.topdown_random_energy_eps = 0.0f;
  prm.repulsion_scale = 0.0f;
  prm.domain_x = (float)domain_x;
  prm.domain_y = (float)domain_y;
  prm.domain_z = (float)domain_z;
  prm.spatial_sigma = (float)spatial_sigma;
  k.set_bytes(prm, 12);

  GPEParams gp;
  gp.dt = (float)dt;
  gp.hbar_eff = (float)hbar_eff;
  gp.mass_eff = (float)mass_eff;
  gp.g_interaction = (float)g_interaction;
  gp.energy_decay = (float)energy_decay;
  gp.chemical_potential = (float)chemical_potential;
  gp.inv_domega2 = (float)inv_domega2;
  gp.anchors = 8u; // MODE_ANCHORS is 8u in Metal; keep mechanical link at Python level too.
  gp.rng_seed = (uint32_t)rng_seed;
  gp.anchor_eps = (float)anchor_eps;
  gp.metric_coupling = (float)metric_coupling;
  k.set_bytes(gp, 13);
  if (extra_potential.defined() && extra_potential.numel() > 0) {
    k.set_tensor(extra_potential, 14);
  }
  k.dispatch_1d(max_carriers);
}

void coherence_update_oscillator_phases(
    at::Tensor osc_phase,            // (N,) fp32 MPS in/out
    at::Tensor osc_omega,            // (N,) fp32 MPS
    at::Tensor osc_amp,              // (N,) fp32 MPS
    at::Tensor carrier_real,         // (maxM,) fp32 MPS
    at::Tensor carrier_imag,         // (maxM,) fp32 MPS
    at::Tensor carrier_omega,        // (maxM,) fp32 MPS
    at::Tensor carrier_gate_width,   // (maxM,) fp32 MPS
    at::Tensor carrier_anchor_idx,   // (maxM*anchors,) int32 MPS
    at::Tensor carrier_anchor_weight,// (maxM*anchors,) fp32 MPS
    at::Tensor num_carriers_snapshot, // (1,) int32 MPS snapshot
    at::Tensor bin_starts,           // (num_bins+1,) int32 MPS
    at::Tensor carrier_binned_idx,   // (maxM,) int32 MPS
    at::Tensor bin_params,           // (2,) fp32 MPS
    int64_t num_bins,
    int64_t max_carriers,
    double dt,
    double coupling_scale,
    double gate_width_min,
    double gate_width_max,
    at::Tensor particle_pos,         // (N,3) fp32 MPS
    double domain_x,
    double domain_y,
    double domain_z,
    double spatial_sigma) {

  check_contig_1d(osc_phase, "coherence_update_osc: osc_phase");
  check_contig_1d(osc_omega, "coherence_update_osc: osc_omega");
  check_contig_1d(osc_amp, "coherence_update_osc: osc_amp");
  TORCH_CHECK(particle_pos.device().is_mps(), "coherence_update_osc: particle_pos must be on MPS");
  TORCH_CHECK(particle_pos.dtype() == at::kFloat, "coherence_update_osc: particle_pos must be fp32");
  TORCH_CHECK(particle_pos.is_contiguous(), "coherence_update_osc: particle_pos must be contiguous");
  TORCH_CHECK(particle_pos.dim() == 2 && particle_pos.size(1) == 3, "coherence_update_osc: particle_pos must be (N,3)");
  check_contig_1d(carrier_real, "coherence_update_osc: carrier_real");
  check_contig_1d(carrier_imag, "coherence_update_osc: carrier_imag");
  check_contig_1d(carrier_omega, "coherence_update_osc: carrier_omega");
  check_contig_1d(carrier_gate_width, "coherence_update_osc: carrier_gate_width");
  check_contig_1d(carrier_anchor_idx, "coherence_update_osc: carrier_anchor_idx");
  check_contig_1d(carrier_anchor_weight, "coherence_update_osc: carrier_anchor_weight");
  TORCH_CHECK(carrier_anchor_idx.dtype() == at::kInt, "coherence_update_osc: carrier_anchor_idx must be int32");

  check_contig_1d(num_carriers_snapshot, "coherence_update_osc: num_carriers_snapshot");
  TORCH_CHECK(num_carriers_snapshot.device().is_mps(), "coherence_update_osc: num_carriers_snapshot must be on MPS");
  TORCH_CHECK(num_carriers_snapshot.dtype() == at::kInt, "coherence_update_osc: num_carriers_snapshot must be int32");

  TORCH_CHECK(bin_starts.device().is_mps(), "coherence_update_osc: bin_starts must be on MPS");
  TORCH_CHECK(bin_starts.dtype() == at::kInt, "coherence_update_osc: bin_starts must be int32");
  check_contig_1d(bin_starts, "coherence_update_osc: bin_starts");
  TORCH_CHECK(carrier_binned_idx.device().is_mps(), "coherence_update_osc: carrier_binned_idx must be on MPS");
  TORCH_CHECK(carrier_binned_idx.dtype() == at::kInt, "coherence_update_osc: carrier_binned_idx must be int32");
  check_contig_1d(carrier_binned_idx, "coherence_update_osc: carrier_binned_idx");
  TORCH_CHECK(bin_params.device().is_mps(), "coherence_update_osc: bin_params must be on MPS");
  TORCH_CHECK(bin_params.dtype() == at::kFloat, "coherence_update_osc: bin_params must be fp32");
  check_contig_1d(bin_params, "coherence_update_osc: bin_params");
  TORCH_CHECK(num_bins > 0, "coherence_update_osc: num_bins must be > 0");

  const int64_t N = osc_phase.size(0);
  if (N == 0 || max_carriers == 0) return;
  ComputeKernel k(&g_pipeline_coherence_update_osc_phases, "coherence_update_oscillator_phases");
  k.set_tensor(osc_phase, 0);
  k.set_tensor(osc_omega, 1);
  k.set_tensor(osc_amp, 2);
  k.set_tensor(carrier_real, 3);
  k.set_tensor(carrier_imag, 4);
  k.set_tensor(carrier_omega, 5);
  k.set_tensor(carrier_gate_width, 6);
  k.set_tensor(carrier_anchor_idx, 7);
  k.set_tensor(carrier_anchor_weight, 8);
  k.set_tensor(num_carriers_snapshot, 9);

  // Reuse SpectralModeParams for dt/coupling/gate bounds; other fields unused here.
  SpectralModeParams prm;
  prm.num_osc = (uint32_t)N;
  prm.max_carriers = (uint32_t)max_carriers;
  prm.num_carriers = 0u; // kernel uses snapshot tensor
  prm.dt = (float)dt;
  prm.coupling_scale = (float)coupling_scale;
  prm.carrier_reg = 0.0f;
  prm.rng_seed = 0u;
  prm.conflict_threshold = 0.0f;
  prm.offender_weight_floor = 0.0f;
  prm.gate_width_min = (float)gate_width_min;
  prm.gate_width_max = (float)gate_width_max;
  prm.ema_alpha = 0.0f;
  prm.recenter_alpha = 0.0f;
  prm.mode = 0u;
  prm.anchor_random_eps = 0.0f;
  prm.stable_amp_threshold = 0.0f;
  prm.crystallize_amp_threshold = 0.0f;
  prm.crystallize_conflict_threshold = 0.0f;
  prm.crystallize_age = 1u;
  prm.crystallized_coupling_boost = 1.0f;
  prm.volatile_decay_mul = 1.0f;
  prm.stable_decay_mul = 1.0f;
  prm.crystallized_decay_mul = 1.0f;
  prm.topdown_phase_scale = 0.0f;
  prm.topdown_energy_scale = 0.0f;
  prm.topdown_random_energy_eps = 0.0f;
  prm.repulsion_scale = 0.0f;
  prm.domain_x = (float)domain_x;
  prm.domain_y = (float)domain_y;
  prm.domain_z = (float)domain_z;
  prm.spatial_sigma = (float)spatial_sigma;
  k.set_bytes(prm, 10);
  k.set_tensor(bin_starts, 11);
  k.set_tensor(carrier_binned_idx, 12);
  k.set_tensor(bin_params, 13);
  const uint32_t nb_u = (uint32_t)num_bins;
  k.set_bytes(nb_u, 14);
  k.set_tensor(particle_pos, 15);
  k.dispatch_1d(N);
}

// ============================================================================
// Particle Generation Kernels
// ============================================================================

void generate_particles(
    at::Tensor positions,      // (N, 3) fp32 MPS out
    at::Tensor velocities,     // (N, 3) fp32 MPS out
    at::Tensor energies,       // (N,) fp32 MPS out
    at::Tensor heats,          // (N,) fp32 MPS out
    at::Tensor excitations,    // (N,) fp32 MPS out
    at::Tensor masses,         // (N,) fp32 MPS out
    at::Tensor random_pos,     // (N, 3) fp32 MPS (uniform [0,1])
    at::Tensor random_props,   // (N, 4) fp32 MPS (uniform [0,1])
    int64_t pattern,           // 0=cluster, 1=line, 2=sphere, 3=random, 4=grid
    double grid_x,
    double grid_y,
    double grid_z,
    double energy_scale,
    double center_x,
    double center_y,
    double center_z,
    double spread,
    double dir_x,
    double dir_y,
    double dir_z) {
  
  const int64_t N = positions.size(0);
  
  TORCH_CHECK(positions.device().is_mps(), "generate_particles: positions must be on MPS");
  TORCH_CHECK(positions.is_contiguous() && positions.dim() == 2 && positions.size(1) == 3,
              "generate_particles: positions must be contiguous (N, 3)");
  at::mps::MPSStream* stream = at::mps::getCurrentMPSStream();
  
  // Step 1: Generate positions
  {
    ComputeKernel k(&g_pipeline_generate_particle_positions, "generate_particle_positions");
    k.set_tensor(positions, 0);
    k.set_tensor(random_pos, 1);
    ParticleGenParams prm;
    prm.num_particles = (uint32_t)N;
    prm.grid_x = (float)grid_x;
    prm.grid_y = (float)grid_y;
    prm.grid_z = (float)grid_z;
    prm.energy_scale = (float)energy_scale;
    prm.pattern = (uint32_t)pattern;
    prm.center_x = (float)center_x;
    prm.center_y = (float)center_y;
    prm.center_z = (float)center_z;
    prm.spread = (float)spread;
    prm.dir_x = (float)dir_x;
    prm.dir_y = (float)dir_y;
    prm.dir_z = (float)dir_z;
    k.set_bytes(prm, 2);
    k.dispatch_1d(N);
  }
  
  // Need to sync to compute mean position
  stream->synchronize(at::mps::SyncType::COMMIT_AND_WAIT);
  
  // Compute center from positions
  at::Tensor pos_mean = positions.mean(0);
  // NOTE: avoid `.item()` directly on MPS tensors inside extensions.
  at::Tensor pos_mean_cpu = pos_mean.to(at::kCPU);
  float mean_x = pos_mean_cpu[0].item<float>();
  float mean_y = pos_mean_cpu[1].item<float>();
  float mean_z = pos_mean_cpu[2].item<float>();
  
  // Step 2: Initialize properties
  {
    ComputeKernel k(&g_pipeline_initialize_particle_properties, "initialize_particle_properties");
    k.set_tensor(positions, 0);
    k.set_tensor(velocities, 1);
    k.set_tensor(energies, 2);
    k.set_tensor(heats, 3);
    k.set_tensor(excitations, 4);
    k.set_tensor(masses, 5);
    k.set_tensor(random_props, 6);
    ParticleGenParams prm;
    prm.num_particles = (uint32_t)N;
    prm.grid_x = (float)grid_x;
    prm.grid_y = (float)grid_y;
    prm.grid_z = (float)grid_z;
    prm.energy_scale = (float)energy_scale;
    prm.pattern = (uint32_t)pattern;
    prm.center_x = (float)center_x;
    prm.center_y = (float)center_y;
    prm.center_z = (float)center_z;
    prm.spread = (float)spread;
    k.set_bytes(prm, 7);
    k.set_bytes(mean_x, 8);
    k.set_bytes(mean_y, 9);
    k.set_bytes(mean_z, 10);
    k.dispatch_1d(N);
  }
}

} // namespace

PYBIND11_MODULE(TORCH_EXTENSION_NAME, m) {
  // Thermodynamics domain helpers
  m.def("manifold_clear_field", &manifold_clear_field, "Clear a field to zero (Metal/MPS, fp32)");
  // Adaptive thermodynamics (global statistics, GPU-only)
  m.def("thermo_reduce_energy_stats", &thermo_reduce_energy_stats, "Reduce energy stats: [mean_abs, mean, std, count] (Metal/MPS, fp32)");
  m.def("particle_interactions", &particle_interactions, "Particle collision + excitation transfer O(N²) (Metal/MPS, fp32)");
  // Sort-based scatter (deterministic, no hash collisions)
  m.def("scatter_compute_cell_idx", &scatter_compute_cell_idx, "Compute primary cell index for each particle (Metal/MPS)");
  m.def("scatter_count_cells", &scatter_count_cells, "Count particles per cell (Metal/MPS)");
  m.def("scatter_reorder_particles", &scatter_reorder_particles, "Reorder particles to sorted positions (Metal/MPS)");
  m.def("scatter_sorted", &scatter_sorted, "Scatter from sorted particles (Metal/MPS, fp32)");
  m.def("pic_gather_update_particles", &pic_gather_update_particles, "PIC gather primitives and update particles (Metal/MPS, fp32)");
    m.def("project_modes_to_spatial_psi", &project_modes_to_spatial_psi, "Project ω-modes into spatial complex Ψ(x) grid (Metal/MPS, fp32)");
    m.def("pic_gather_update_particles_pilot_wave", &pic_gather_update_particles_pilot_wave, "Advect particles by pilot-wave current from Ψ(x) (Metal/MPS, fp32)");
  // Gas (Eulerian grid update): RK2 for (rho, mom, e_int)
  m.def("gas_rk2_stage1", &gas_rk2_stage1, "Gas grid RK2 stage1 (dual-energy internal e_int) (Metal/MPS, fp32)");
  m.def("gas_rk2_stage2", &gas_rk2_stage2, "Gas grid RK2 stage2 (dual-energy internal e_int) (Metal/MPS, fp32)");
  // Spatial hash grid (O(N) collisions)
  m.def("spatial_hash_assign", &spatial_hash_assign, "Assign particles to spatial hash cells (Metal/MPS)");
  m.def("spatial_hash_prefix_sum", &spatial_hash_prefix_sum, "Compute prefix sum of cell counts (Metal/MPS)");
  m.def("spatial_hash_scatter", &spatial_hash_scatter, "Scatter particle indices to sorted array (Metal/MPS)");
  m.def("spatial_hash_collisions", &spatial_hash_collisions, "Particle collisions using spatial hash O(N) (Metal/MPS, fp32)");
  // Generic u32 scan (building blocks)
  m.def("exclusive_scan_u32_pass1", &exclusive_scan_u32_pass1, "Exclusive scan pass1 (per-block) for u32/int32 (Metal/MPS)");
  m.def("exclusive_scan_u32_add_block_offsets", &exclusive_scan_u32_add_block_offsets, "Exclusive scan add block offsets (Metal/MPS)");
  m.def("exclusive_scan_u32_finalize_total", &exclusive_scan_u32_finalize_total, "Exclusive scan finalize total into out[n] (Metal/MPS)");
  // Coherence field / oscillator coupling kernels
  m.def("coherence_accumulate_forces", &coherence_accumulate_forces_v2, "Parallel force accumulation (Metal/MPS, fp32)");
  m.def("coherence_gpe_step", &coherence_gpe_step, "Coherence field step (dissipative GPE) (Metal/MPS, fp32)");
  m.def("coherence_update_oscillator_phases", &coherence_update_oscillator_phases, "Oscillator phase update from Ψ(ω) (Metal/MPS, fp32)");
  // Coherence ω-binning (GPU-only)
  m.def("coherence_reduce_omega_minmax_keys", &coherence_reduce_omega_minmax_keys, "Reduce ω min/max keys (Metal/MPS)");
  m.def("coherence_compute_bin_params", &coherence_compute_bin_params, "Compute ω-bin params [omega_min, inv_bin_width] (Metal/MPS)");
  m.def("coherence_bin_count", &coherence_bin_count_carriers, "Count carriers per ω-bin (Metal/MPS)");
  m.def("coherence_bin_scatter", &coherence_bin_scatter_carriers, "Scatter carriers into ω-bin order (Metal/MPS)");
  // Particle generation kernels
  m.def("generate_particles", &generate_particles, "Generate particles with pattern (Metal/MPS, fp32)");
}


