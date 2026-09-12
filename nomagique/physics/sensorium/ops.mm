#include "bridge.h"
#import <Foundation/Foundation.h>
#import <Metal/Metal.h>

#include "shared/radix2_math.h"
#include "shared/scan_plan.h"
#include <algorithm>
#include <cstring>
#include <initializer_list>
#include <mutex>
#include <string>
#include <unordered_map>

constexpr NSUInteger kThreadsPerThreadgroup = 256;

struct ManifoldBuffer {
  id<MTLBuffer> mtl_buffer;
  uint64_t size_bytes;
};

struct ManifoldContext {
  id<MTLDevice> device;
  id<MTLCommandQueue> command_queue;
  id<MTLLibrary> library;
  id<MTLCommandBuffer> current_command_buffer;
  id<MTLComputeCommandEncoder> current_encoder;

  std::unordered_map<std::string, id<MTLComputePipelineState>> pipelines;
  std::mutex mtx;
  std::string error;
  id<MTLBuffer> wave_ledger = nil;
  unsigned wave_ledger_modes = 0;
  id<MTLBuffer> phase_ledger = nil;
  unsigned phase_ledger_particles = 0;
  id<MTLBuffer> runtime_scratch = nil;
  NSUInteger runtime_scratch_bytes = 0;
  void fail(const char *message) {
    if (error.empty()) {
      error = message;
      NSLog(@"[Manifold] %s", message);
    }
  }
  id<MTLBuffer> scratch(NSUInteger bytes) {
    if (bytes <= runtime_scratch_bytes)
      return runtime_scratch;
    // Keep all previously encoded consumers valid before replacing storage.
    commit_and_wait();
    if (!error.empty())
      return nil;
    runtime_scratch =
        [device newBufferWithLength:bytes
                            options:MTLResourceStorageModePrivate];
    runtime_scratch_bytes = runtime_scratch ? bytes : 0;
    if (!runtime_scratch)
      fail("failed to allocate runtime GPU scratch");
    return runtime_scratch;
  }

  id<MTLComputePipelineState> get_pipeline(const char *fn_name) {
    std::lock_guard<std::mutex> lock(mtx);
    std::string key(fn_name);
    auto it = pipelines.find(key);
    if (it != pipelines.end()) {
      return it->second;
    }

    NSString *ns_name = [NSString stringWithUTF8String:fn_name];
    id<MTLFunction> fn = [library newFunctionWithName:ns_name];
    if (!fn) {
      NSLog(@"[Manifold] Function '%s' not found in metallib", fn_name);
      fail("Metal kernel missing from metallib");
      return nil;
    }

    NSError *err = nil;
    id<MTLComputePipelineState> pso =
        [device newComputePipelineStateWithFunction:fn error:&err];
    if (!pso) {
      NSLog(@"[Manifold] Failed to build pipeline '%s': %@", fn_name,
            err.localizedDescription);
      fail("Metal pipeline creation failed");
      return nil;
    }
    pipelines[key] = pso;
    return pso;
  }

  void begin_compute() {
    if (!current_command_buffer) {
      current_command_buffer = [command_queue commandBuffer];
      current_encoder = [current_command_buffer computeCommandEncoder];
    }
  }

  void commit_and_wait() {
    @autoreleasepool {
      if (current_encoder) {
        [current_encoder endEncoding];
        current_encoder = nil;
      }
      if (current_command_buffer) {
        [current_command_buffer commit];
        [current_command_buffer waitUntilCompleted];
        if (current_command_buffer.status == MTLCommandBufferStatusError)
          fail(current_command_buffer.error.localizedDescription.UTF8String);
        current_command_buffer = nil;
      }
    }
  }
};

// Dispatch helper struct
struct KernelDispatch {
  ManifoldContext *ctx;
  id<MTLComputePipelineState> pso;

  KernelDispatch(ManifoldContext *c, const char *name) : ctx(c) {
    ctx->begin_compute();
    pso = ctx->get_pipeline(name);
    if (pso && ctx->error.empty())
      [ctx->current_encoder setComputePipelineState:pso];
  }

  inline void set_buffer(ManifoldBuffer *buf, NSUInteger idx) {
    if (buf && buf->mtl_buffer) {
      [ctx->current_encoder setBuffer:buf->mtl_buffer offset:0 atIndex:idx];
    }
  }

  template <class T> inline void set_bytes(const T &val, NSUInteger idx) {
    [ctx->current_encoder setBytes:&val length:sizeof(T) atIndex:idx];
  }

  inline void set_threadgroup_memory(NSUInteger bytes, NSUInteger idx) {
    [ctx->current_encoder setThreadgroupMemoryLength:bytes atIndex:idx];
  }

  inline void dispatch_1d(int64_t n_threads) {
    if (n_threads <= 0 || !pso || !ctx->error.empty())
      return;
    if (kThreadsPerThreadgroup > pso.maxTotalThreadsPerThreadgroup) {
      ctx->fail("threadgroup exceeds pipeline limit");
      return;
    }
    NSUInteger num_groups =
        (n_threads + kThreadsPerThreadgroup - 1) / kThreadsPerThreadgroup;
    [ctx->current_encoder
         dispatchThreadgroups:MTLSizeMake(num_groups, 1, 1)
        threadsPerThreadgroup:MTLSizeMake(kThreadsPerThreadgroup, 1, 1)];
  }

  inline void dispatch_groups(NSUInteger num_groups,
                              NSUInteger tg_threads = kThreadsPerThreadgroup) {
    if (!pso || !ctx->error.empty() || !num_groups)
      return;
    if (tg_threads > pso.maxTotalThreadsPerThreadgroup) {
      ctx->fail("threadgroup exceeds pipeline limit");
      return;
    }
    [ctx->current_encoder dispatchThreadgroups:MTLSizeMake(num_groups, 1, 1)
                         threadsPerThreadgroup:MTLSizeMake(tg_threads, 1, 1)];
  }

  inline void buffer_barrier() {
    [ctx->current_encoder memoryBarrierWithScope:MTLBarrierScopeBuffers];
  }
};

static bool runtime_ready(ManifoldContext *ctx,
                          std::initializer_list<ManifoldBuffer *> buffers) {
  if (!ctx || !ctx->error.empty())
    return false;
  for (auto *b : buffers)
    if (!b || !b->mtl_buffer) {
      ctx->fail("missing Metal buffer");
      return false;
    }
  return true;
}

extern "C" {

ManifoldContext *manifold_create_context(const char *metallib_path) {
  id<MTLDevice> device = MTLCreateSystemDefaultDevice();
  if (!device)
    return nullptr;
  // Metal 4.1 language selection does not remove GPU-family restrictions.
  // This macOS backend uses native ulong device min/max (Apple8+) and float
  // atomics.
  if (![device supportsFamily:MTLGPUFamilyApple8]) {
    NSLog(
        @"[Manifold] Native packed offender min/max requires Apple8+ on macOS");
    return nullptr;
  }

  id<MTLCommandQueue> queue = [device newCommandQueue];
  NSURL *url =
      [NSURL fileURLWithPath:[NSString stringWithUTF8String:metallib_path]];
  NSError *err = nil;
  id<MTLLibrary> lib = [device newLibraryWithURL:url error:&err];
  if (!lib) {
    NSLog(@"[Manifold] Failed to load metallib at %s: %@", metallib_path,
          err.localizedDescription);
    return nullptr;
  }

  ManifoldContext *ctx = new ManifoldContext();
  ctx->device = device;
  ctx->command_queue = queue;
  ctx->library = lib;
  ctx->current_command_buffer = nil;
  ctx->current_encoder = nil;
  return ctx;
}

void manifold_destroy_context(ManifoldContext *ctx) {
  if (!ctx)
    return;
  ctx->commit_and_wait();
  delete ctx;
}

bool manifold_synchronize_checked(ManifoldContext *ctx) {
  if (!ctx)
    return false;
  ctx->commit_and_wait();
  return ctx->error.empty();
}
const char *manifold_last_error(ManifoldContext *ctx) {
  return ctx ? ctx->error.c_str() : "Metal context is null";
}
void manifold_synchronize(ManifoldContext *ctx) {
  if (!ctx)
    return;
  ctx->commit_and_wait();
}

ManifoldBuffer *manifold_create_buffer(ManifoldContext *ctx, uint64_t bytes,
                                       const void *initial_data) {
  if (!ctx || bytes == 0)
    return nullptr;

  id<MTLBuffer> mtl_buf = nil;
  if (initial_data) {
    mtl_buf = [ctx->device newBufferWithBytes:initial_data
                                       length:(NSUInteger)bytes
                                      options:MTLResourceStorageModeShared];
  } else {
    mtl_buf = [ctx->device newBufferWithLength:(NSUInteger)bytes
                                       options:MTLResourceStorageModeShared];
  }

  ManifoldBuffer *buf = new ManifoldBuffer();
  buf->mtl_buffer = mtl_buf;
  buf->size_bytes = bytes;
  return buf;
}

void manifold_destroy_buffer(ManifoldBuffer *buf) {
  if (!buf)
    return;
  @autoreleasepool {
    buf->mtl_buffer = nil;
    delete buf;
  }
}

void *manifold_get_buffer_pointer(ManifoldBuffer *buf) {
  if (!buf || !buf->mtl_buffer)
    return nullptr;
  return [buf->mtl_buffer contents];
}

uint64_t manifold_get_buffer_size(ManifoldBuffer *buf) {
  return buf ? buf->size_bytes : 0;
}

// ----------------------------------------------------------------------------
// 1. Diagnostics & Memory
// ----------------------------------------------------------------------------
void manifold_clear_field(ManifoldContext *ctx, ManifoldBuffer *field) {
  uint32_t n = (uint32_t)(field->size_bytes / sizeof(float));
  KernelDispatch k(ctx, "clear_field");
  k.set_buffer(field, 0);
  k.set_bytes(n, 1);
  k.buffer_barrier();
  k.dispatch_1d(n);
  k.buffer_barrier();
}

void manifold_thermo_reduce_energy_stats(ManifoldContext *ctx,
                                         ManifoldBuffer *x,
                                         ManifoldBuffer *out_stats) {
  int64_t n = x->size_bytes / sizeof(float);
  if (n <= 0)
    return;
  NSUInteger num_groups =
      (n + kThreadsPerThreadgroup - 1) / kThreadsPerThreadgroup;

  // Group scratch allocation
  ManifoldBuffer *group_stats =
      manifold_create_buffer(ctx, num_groups * 4 * sizeof(float), nullptr);

  {
    KernelDispatch k(ctx, "reduce_float_stats_pass1");
    k.set_buffer(x, 0);
    k.set_buffer(group_stats, 1);
    uint32_t nu = (uint32_t)n;
    k.set_bytes(nu, 2);
    k.dispatch_groups(num_groups, kThreadsPerThreadgroup);
  }
  {
    KernelDispatch k(ctx, "reduce_float_stats_finalize");
    k.set_buffer(group_stats, 0);
    k.set_buffer(out_stats, 1);
    uint32_t gu = (uint32_t)num_groups;
    k.set_bytes(gu, 2);
    k.dispatch_groups(1, kThreadsPerThreadgroup);
  }
  manifold_destroy_buffer(group_stats);
}

// ----------------------------------------------------------------------------
// 2. PIC & Sort-Based Scatter
// ----------------------------------------------------------------------------
void manifold_scatter_compute_cell_idx(ManifoldContext *ctx,
                                       ManifoldBuffer *particle_pos,
                                       ManifoldBuffer *particle_cell_idx,
                                       int64_t gx, int64_t gy, int64_t gz,
                                       float grid_spacing) {
  int64_t n = particle_pos->size_bytes / (3 * sizeof(float));
  if (n == 0)
    return;
  SortScatterParams prm = {(uint32_t)n,        (uint32_t)(gx * gy * gz),
                           (uint32_t)gx,       (uint32_t)gy,
                           (uint32_t)gz,       grid_spacing,
                           1.0f / grid_spacing};
  KernelDispatch k(ctx, "scatter_compute_cell_idx");
  k.set_buffer(particle_pos, 0);
  k.set_buffer(particle_cell_idx, 1);
  k.set_bytes(prm, 2);
  k.buffer_barrier();
  k.dispatch_1d(n);
  k.buffer_barrier();
}

void manifold_scatter_count_cells(ManifoldContext *ctx,
                                  ManifoldBuffer *particle_cell_idx,
                                  ManifoldBuffer *cell_counts, int64_t gx,
                                  int64_t gy, int64_t gz, float grid_spacing) {
  int64_t n = particle_cell_idx->size_bytes / sizeof(uint32_t);
  if (n == 0)
    return;
  SortScatterParams prm = {(uint32_t)n,        (uint32_t)(gx * gy * gz),
                           (uint32_t)gx,       (uint32_t)gy,
                           (uint32_t)gz,       grid_spacing,
                           1.0f / grid_spacing};
  KernelDispatch k(ctx, "scatter_count_cells");
  k.set_buffer(particle_cell_idx, 0);
  k.set_buffer(cell_counts, 1);
  k.set_bytes(prm, 2);
  k.buffer_barrier();
  k.dispatch_1d(n);
  k.buffer_barrier();
}

void manifold_scatter_reorder_particles(
    ManifoldContext *ctx, ManifoldBuffer *pos_in, ManifoldBuffer *vel_in,
    ManifoldBuffer *mass_in, ManifoldBuffer *heat_in, ManifoldBuffer *energy_in,
    ManifoldBuffer *particle_cell_idx, ManifoldBuffer *cell_starts,
    ManifoldBuffer *cell_offsets, ManifoldBuffer *pos_out,
    ManifoldBuffer *vel_out, ManifoldBuffer *mass_out, ManifoldBuffer *heat_out,
    ManifoldBuffer *energy_out, ManifoldBuffer *sorted_original_idx, int64_t gx,
    int64_t gy, int64_t gz, float grid_spacing) {
  int64_t n = pos_in->size_bytes / (3 * sizeof(float));
  if (n == 0)
    return;
  SortScatterParams prm = {(uint32_t)n,        (uint32_t)(gx * gy * gz),
                           (uint32_t)gx,       (uint32_t)gy,
                           (uint32_t)gz,       grid_spacing,
                           1.0f / grid_spacing};
  KernelDispatch k(ctx, "scatter_reorder_particles");
  k.set_buffer(pos_in, 0);
  k.set_buffer(vel_in, 1);
  k.set_buffer(mass_in, 2);
  k.set_buffer(heat_in, 3);
  k.set_buffer(energy_in, 4);
  k.set_buffer(particle_cell_idx, 5);
  k.set_buffer(cell_starts, 6);
  k.set_buffer(cell_offsets, 7);
  k.set_buffer(pos_out, 8);
  k.set_buffer(vel_out, 9);
  k.set_buffer(mass_out, 10);
  k.set_buffer(heat_out, 11);
  k.set_buffer(energy_out, 12);
  k.set_buffer(sorted_original_idx, 13);
  k.set_bytes(prm, 14);
  k.buffer_barrier();
  k.dispatch_1d(n);
  k.buffer_barrier();
}

void manifold_scatter_sorted(ManifoldContext *ctx, ManifoldBuffer *pos,
                             ManifoldBuffer *vel, ManifoldBuffer *mass,
                             ManifoldBuffer *heat, ManifoldBuffer *energy,
                             ManifoldBuffer *rho_field,
                             ManifoldBuffer *mom_field, ManifoldBuffer *E_field,
                             int64_t gx, int64_t gy, int64_t gz,
                             float grid_spacing) {
  int64_t n = pos->size_bytes / (3 * sizeof(float));
  if (n == 0)
    return;
  SortScatterParams prm = {(uint32_t)n,        (uint32_t)(gx * gy * gz),
                           (uint32_t)gx,       (uint32_t)gy,
                           (uint32_t)gz,       grid_spacing,
                           1.0f / grid_spacing};
  KernelDispatch k(ctx, "scatter_sorted");
  k.set_buffer(pos, 0);
  k.set_buffer(vel, 1);
  k.set_buffer(mass, 2);
  k.set_buffer(heat, 3);
  k.set_buffer(energy, 4);
  k.set_buffer(rho_field, 5);
  k.set_buffer(mom_field, 6);
  k.set_buffer(E_field, 7);
  k.set_bytes(prm, 8);
  k.buffer_barrier();
  k.dispatch_1d(n);
  k.buffer_barrier();
}

void manifold_pic_gather_update_particles(
    ManifoldContext *ctx, ManifoldBuffer *pos_in, ManifoldBuffer *mass,
    ManifoldBuffer *pos_out, ManifoldBuffer *vel_out, ManifoldBuffer *heat_out,
    ManifoldBuffer *rho_field, ManifoldBuffer *mom_field,
    ManifoldBuffer *E_field, ManifoldBuffer *gravity_potential,
    ManifoldBuffer *dbg_head, ManifoldBuffer *dbg_words, int64_t dbg_capacity,
    int64_t gx, int64_t gy, int64_t gz, float grid_spacing, float dt,
    float domain_x, float domain_y, float domain_z, float gamma,
    float R_specific, float c_v, float rho_min, float p_min,
    float gravity_enabled) {
  int64_t n = pos_in->size_bytes / (3 * sizeof(float));
  if (n == 0)
    return;
  PicGatherParams prm = {(uint32_t)n,  (uint32_t)gx,
                         (uint32_t)gy, (uint32_t)gz,
                         grid_spacing, 1.0f / grid_spacing,
                         dt,           domain_x,
                         domain_y,     domain_z,
                         gamma,        R_specific,
                         c_v,          rho_min,
                         p_min,        gravity_enabled};
  KernelDispatch k(ctx, "pic_gather_update_particles");
  k.set_buffer(pos_in, 0);
  k.set_buffer(mass, 1);
  k.set_buffer(pos_out, 2);
  k.set_buffer(vel_out, 3);
  k.set_buffer(heat_out, 4);
  k.set_buffer(rho_field, 5);
  k.set_buffer(mom_field, 6);
  k.set_buffer(E_field, 7);
  k.set_buffer(gravity_potential, 8);
  k.set_bytes(prm, 9);
  k.set_buffer(dbg_head, 10);
  k.set_buffer(dbg_words, 11);
  uint32_t cap_u32 = (uint32_t)dbg_capacity;
  k.set_bytes(cap_u32, 12);
  k.dispatch_1d(n);
}

// ----------------------------------------------------------------------------
// 3. Quantum Flow & Waves
// ----------------------------------------------------------------------------
void manifold_project_modes_to_spatial_psi(
    ManifoldContext *ctx, ManifoldBuffer *mode_psi_real,
    ManifoldBuffer *mode_psi_imag, ManifoldBuffer *mode_anchor_idx,
    ManifoldBuffer *mode_anchor_weight, ManifoldBuffer *particle_pos,
    ManifoldBuffer *psi_re_field, ManifoldBuffer *psi_im_field,
    int64_t anchors_per_mode, int64_t gx, int64_t gy, int64_t gz,
    float grid_spacing) {
  int64_t num_modes = mode_psi_real->size_bytes / sizeof(float);
  int64_t num_particles = particle_pos->size_bytes / (3 * sizeof(float));
  int64_t total = num_modes * anchors_per_mode;
  if (total <= 0)
    return;

  ModeProjectParams p = {(uint32_t)num_modes,
                         (uint32_t)num_particles,
                         (uint32_t)anchors_per_mode,
                         (uint32_t)gx,
                         (uint32_t)gy,
                         (uint32_t)gz,
                         grid_spacing,
                         1.0f / grid_spacing};
  KernelDispatch k(ctx, "project_modes_to_spatial_psi");
  k.set_buffer(mode_psi_real, 0);
  k.set_buffer(mode_psi_imag, 1);
  k.set_buffer(mode_anchor_idx, 2);
  k.set_buffer(mode_anchor_weight, 3);
  k.set_buffer(particle_pos, 4);
  k.set_buffer(psi_re_field, 5);
  k.set_buffer(psi_im_field, 6);
  k.set_bytes(p, 7);
  k.dispatch_1d(total);
}

void manifold_pic_gather_pilot_wave(
    ManifoldContext *ctx, ManifoldBuffer *pos_in, ManifoldBuffer *mass,
    ManifoldBuffer *pos_out, ManifoldBuffer *vel_out, ManifoldBuffer *psi_re,
    ManifoldBuffer *psi_im, int64_t num_particles, int64_t gx, int64_t gy,
    int64_t gz, float grid_spacing, float dt, float domain_x, float domain_y,
    float domain_z, float hbar_eff, float eps_denom, float mass_min) {
  if (!runtime_ready(ctx, {pos_in, mass, pos_out, vel_out, psi_re, psi_im}))
    return;
  if (num_particles < 0 || uint64_t(num_particles) > UINT32_MAX / 3u ||
      gx <= 0 || gy <= 0 || gz <= 0 || uint64_t(gx) > UINT32_MAX ||
      uint64_t(gy) > UINT32_MAX || uint64_t(gz) > UINT32_MAX ||
      uint64_t(gx) * uint64_t(gy) > UINT32_MAX / uint64_t(gz) ||
      !std::isfinite(grid_spacing) || grid_spacing <= 0 || !std::isfinite(dt) ||
      dt < 0 || !std::isfinite(hbar_eff) || hbar_eff <= 0 ||
      !std::isfinite(eps_denom) || eps_denom < 0 || !std::isfinite(mass_min) ||
      mass_min <= 0 || !std::isfinite(domain_x) || domain_x <= 0 ||
      !std::isfinite(domain_y) || domain_y <= 0 || !std::isfinite(domain_z) ||
      domain_z <= 0) {
    ctx->fail("invalid pilot dimensions/parameters");
    return;
  }
  const double dx = grid_spacing;
  if (std::abs(double(domain_x) - double(gx) * dx) > 8e-7 * domain_x ||
      std::abs(double(domain_y) - double(gy) * dx) > 8e-7 * domain_y ||
      std::abs(double(domain_z) - double(gz) * dx) > 8e-7 * domain_z) {
    ctx->fail("pilot domain differs from periodic field extent");
    return;
  }
  uint64_t np = uint64_t(num_particles),
           cells = uint64_t(gx) * uint64_t(gy) * uint64_t(gz);
  if (pos_in->size_bytes < np * 12 || mass->size_bytes < np * 4 ||
      pos_out->size_bytes < np * 12 || vel_out->size_bytes < np * 12 ||
      psi_re->size_bytes < cells * 4 || psi_im->size_bytes < cells * 4) {
    ctx->fail("undersized pilot buffer");
    return;
  }
  ManifoldBuffer *inputs[] = {pos_in, mass, psi_re, psi_im};
  for (auto b : inputs)
    if (b->mtl_buffer == pos_out->mtl_buffer ||
        b->mtl_buffer == vel_out->mtl_buffer) {
      ctx->fail("pilot writable/input alias");
      return;
    }
  if (pos_out->mtl_buffer == vel_out->mtl_buffer) {
    ctx->fail("pilot writable alias");
    return;
  }
  if (num_particles == 0)
    return;
  PilotWaveParams p = {(uint32_t)num_particles,
                       (uint32_t)gx,
                       (uint32_t)gy,
                       (uint32_t)gz,
                       grid_spacing,
                       1.0f / grid_spacing,
                       dt,
                       domain_x,
                       domain_y,
                       domain_z,
                       hbar_eff,
                       eps_denom,
                       mass_min};
  KernelDispatch k(ctx, "pic_gather_update_particles_pilot_wave");
  k.set_buffer(pos_in, 0);
  k.set_buffer(mass, 1);
  k.set_buffer(pos_out, 2);
  k.set_buffer(vel_out, 3);
  k.set_buffer(psi_re, 4);
  k.set_buffer(psi_im, 5);
  k.set_bytes(p, 6);
  k.dispatch_1d(num_particles);
}

// ----------------------------------------------------------------------------
// 4. Gas Dynamics (Eulerian RK2)
// ----------------------------------------------------------------------------
void manifold_gas_rk2_stage1(ManifoldContext *ctx, ManifoldBuffer *rho0,
                             ManifoldBuffer *mom0, ManifoldBuffer *e0,
                             ManifoldBuffer *rho1, ManifoldBuffer *mom1,
                             ManifoldBuffer *e1, ManifoldBuffer *k1_rho,
                             ManifoldBuffer *k1_mom, ManifoldBuffer *k1_e,
                             ManifoldBuffer *dbg_head,
                             ManifoldBuffer *dbg_words, int64_t dbg_capacity,
                             int64_t gx, int64_t gy, int64_t gz, float dx,
                             float dt, float gamma, float c_v, float rho_min,
                             float p_min, float mu, float k_thermal) {
  int64_t n = gx * gy * gz;
  GasGridParams prm = {(uint32_t)n, (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
                       dx,          dt,           gamma,        c_v,
                       rho_min,     p_min,        mu,           k_thermal};
  KernelDispatch k(ctx, "gas_rk2_stage1");
  k.set_buffer(rho0, 0);
  k.set_buffer(mom0, 1);
  k.set_buffer(e0, 2);
  k.set_buffer(rho1, 3);
  k.set_buffer(mom1, 4);
  k.set_buffer(e1, 5);
  k.set_buffer(k1_rho, 6);
  k.set_buffer(k1_mom, 7);
  k.set_buffer(k1_e, 8);
  k.set_bytes(prm, 9);
  k.set_buffer(dbg_head, 10);
  k.set_buffer(dbg_words, 11);
  uint32_t cap = (uint32_t)dbg_capacity;
  k.set_bytes(cap, 12);
  k.dispatch_1d(n);
}

void manifold_gas_rk2_stage2(ManifoldContext *ctx, ManifoldBuffer *rho0,
                             ManifoldBuffer *mom0, ManifoldBuffer *e0,
                             ManifoldBuffer *rho1, ManifoldBuffer *mom1,
                             ManifoldBuffer *e1, ManifoldBuffer *k1_rho,
                             ManifoldBuffer *k1_mom, ManifoldBuffer *k1_e,
                             ManifoldBuffer *rho_out, ManifoldBuffer *mom_out,
                             ManifoldBuffer *e_out, ManifoldBuffer *dbg_head,
                             ManifoldBuffer *dbg_words, int64_t dbg_capacity,
                             int64_t gx, int64_t gy, int64_t gz, float dx,
                             float dt, float gamma, float c_v, float rho_min,
                             float p_min, float mu, float k_thermal) {
  int64_t n = gx * gy * gz;
  GasGridParams prm = {(uint32_t)n, (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
                       dx,          dt,           gamma,        c_v,
                       rho_min,     p_min,        mu,           k_thermal};
  KernelDispatch k(ctx, "gas_rk2_stage2");
  k.set_buffer(rho0, 0);
  k.set_buffer(mom0, 1);
  k.set_buffer(e0, 2);
  k.set_buffer(rho1, 3);
  k.set_buffer(mom1, 4);
  k.set_buffer(e1, 5);
  k.set_buffer(k1_rho, 6);
  k.set_buffer(k1_mom, 7);
  k.set_buffer(k1_e, 8);
  k.set_buffer(rho_out, 9);
  k.set_buffer(mom_out, 10);
  k.set_buffer(e_out, 11);
  k.set_bytes(prm, 12);
  k.set_buffer(dbg_head, 13);
  k.set_buffer(dbg_words, 14);
  uint32_t cap = (uint32_t)dbg_capacity;
  k.set_bytes(cap, 15);
  k.dispatch_1d(n);
}

// ----------------------------------------------------------------------------
// 5. Spatial Hash Grid Collisions
// ----------------------------------------------------------------------------
void manifold_spatial_hash_assign(ManifoldContext *ctx, ManifoldBuffer *pos,
                                  ManifoldBuffer *cell_idx,
                                  ManifoldBuffer *cell_counts, int64_t gx,
                                  int64_t gy, int64_t gz, float cell_size,
                                  float min_x, float min_y, float min_z) {
  int64_t n = pos->size_bytes / (3 * sizeof(float));
  if (n == 0)
    return;
  SpatialHashParams prm = {(uint32_t)n,  (uint32_t)gx, (uint32_t)gy,
                           (uint32_t)gz, cell_size,    1.0f / cell_size,
                           min_x,        min_y,        min_z};
  KernelDispatch k(ctx, "spatial_hash_assign");
  k.set_buffer(pos, 0);
  k.set_buffer(cell_idx, 1);
  k.set_buffer(cell_counts, 2);
  k.set_bytes(prm, 3);
  k.dispatch_1d(n);
}

void manifold_spatial_hash_scatter(ManifoldContext *ctx,
                                   ManifoldBuffer *cell_idx,
                                   ManifoldBuffer *sorted_idx,
                                   ManifoldBuffer *cell_offsets,
                                   int64_t num_particles) {
  if (num_particles == 0)
    return;
  KernelDispatch k(ctx, "spatial_hash_scatter");
  k.set_buffer(cell_idx, 0);
  k.set_buffer(sorted_idx, 1);
  k.set_buffer(cell_offsets, 2);
  uint32_t np = (uint32_t)num_particles;
  k.set_bytes(np, 3);
  k.dispatch_1d(num_particles);
}

void manifold_spatial_hash_collisions(
    ManifoldContext *ctx, ManifoldBuffer *pos, ManifoldBuffer *vel,
    ManifoldBuffer *excitation, ManifoldBuffer *mass, ManifoldBuffer *heat,
    ManifoldBuffer *sorted_idx, ManifoldBuffer *cell_starts,
    ManifoldBuffer *cell_idx, ManifoldBuffer *vel_in, ManifoldBuffer *heat_in,
    int64_t gx, int64_t gy, int64_t gz, float cell_size, float min_x,
    float min_y, float min_z, float dt, float radius, float young_modulus,
    float thermal_conductivity, float specific_heat, float restitution) {
  int64_t n = pos->size_bytes / (3 * sizeof(float));
  if (n == 0)
    return;
  SpatialCollisionParams prm = {(uint32_t)n,
                                (uint32_t)gx,
                                (uint32_t)gy,
                                (uint32_t)gz,
                                cell_size,
                                1.0f / cell_size,
                                min_x,
                                min_y,
                                min_z,
                                dt,
                                radius,
                                young_modulus,
                                thermal_conductivity,
                                specific_heat,
                                restitution};
  KernelDispatch k(ctx, "spatial_hash_collisions");
  k.set_buffer(pos, 0);
  k.set_buffer(vel, 1);
  k.set_buffer(excitation, 2);
  k.set_buffer(mass, 3);
  k.set_buffer(heat, 4);
  k.set_buffer(sorted_idx, 5);
  k.set_buffer(cell_starts, 6);
  k.set_buffer(cell_idx, 7);
  k.set_buffer(vel_in, 8);
  k.set_buffer(heat_in, 9);
  k.set_bytes(prm, 10);
  k.dispatch_1d(n);
}

void manifold_particle_interactions(
    ManifoldContext *ctx, ManifoldBuffer *pos, ManifoldBuffer *vel,
    ManifoldBuffer *excitation, ManifoldBuffer *mass, ManifoldBuffer *heat,
    ManifoldBuffer *vel_in, ManifoldBuffer *heat_in, float dt, float radius,
    float young_modulus, float thermal_conductivity, float specific_heat,
    float restitution, float domain_x, float domain_y, float domain_z) {
  int64_t n = pos->size_bytes / (3 * sizeof(float));
  if (n == 0)
    return;
  ParticleInteractionParams prm = {
      (uint32_t)n,   dt,          radius,   young_modulus, thermal_conductivity,
      specific_heat, restitution, domain_x, domain_y,      domain_z};
  KernelDispatch k(ctx, "particle_interactions");
  k.set_buffer(pos, 0);
  k.set_buffer(vel, 1);
  k.set_buffer(excitation, 2);
  k.set_buffer(mass, 3);
  k.set_buffer(heat, 4);
  k.set_buffer(vel_in, 5);
  k.set_buffer(heat_in, 6);
  k.set_bytes(prm, 7);
  k.dispatch_1d(n);
}

// ----------------------------------------------------------------------------
// 6. Generic Parallel Exclusive Scan (u32)
// ----------------------------------------------------------------------------
bool manifold_exclusive_scan_u32(ManifoldContext *ctx, ManifoldBuffer *in,
                                 ManifoldBuffer *out, int64_t n) {
  if (!runtime_ready(ctx, {in, out}))
    return false;
  if (n < 0 || uint64_t(n) > UINT32_MAX || in->mtl_buffer == out->mtl_buffer ||
      in->size_bytes < uint64_t(n) * 4 ||
      out->size_bytes < (uint64_t(n) + 1) * 4) {
    ctx->fail(
        "exclusive scan requires disjoint n-input and n+1-output buffers");
    return false;
  }
  uint32_t len = uint32_t(n);
  ManifoldScanPlan plan;
  if (!plan.make(len)) {
    ctx->fail("scan plan overflow");
    return false;
  }
  id<MTLBuffer> work = plan.words ? ctx->scratch(plan.words * 4) : nil;
  if (plan.words && !work)
    return false;
  for (unsigned l = 0; l < plan.count; l++) {
    const auto &a = plan.level[l];
    KernelDispatch k(ctx, "exclusive_scan_u32_pass1");
    k.buffer_barrier();
    [ctx->current_encoder setBuffer:(l ? work : in->mtl_buffer)
                             offset:(l ? plan.level[l - 1].sums * 4 : 0)
                            atIndex:0];
    [ctx->current_encoder setBuffer:(l ? work : out->mtl_buffer)
                             offset:(l ? plan.level[l - 1].prefix * 4 : 0)
                            atIndex:1];
    [ctx->current_encoder setBuffer:work offset:a.sums * 4 atIndex:2];
    k.set_bytes(a.n, 3);
    k.set_threadgroup_memory(256 * sizeof(uint32_t), 0);
    k.dispatch_groups(a.groups, 256);
    k.buffer_barrier();
  }
  for (int l = int(plan.count) - 2; l >= 0; --l) {
    const auto &a = plan.level[l];
    KernelDispatch k(ctx, "exclusive_scan_u32_add_block_offsets");
    [ctx->current_encoder setBuffer:(l ? work : out->mtl_buffer)
                             offset:(l ? plan.level[l - 1].prefix * 4 : 0)
                            atIndex:0];
    [ctx->current_encoder setBuffer:work offset:a.prefix * 4 atIndex:1];
    k.set_bytes(a.n, 2);
    k.dispatch_groups(a.groups, 256);
    k.buffer_barrier();
  }
  KernelDispatch k(ctx, "exclusive_scan_u32_finalize_total");
  k.set_buffer(in, 0);
  k.set_buffer(out, 1);
  k.set_bytes(len, 2);
  k.dispatch_groups(1, 1);
  k.buffer_barrier();
  return ctx->error.empty();
}

void manifold_exclusive_scan_u32_pass1(ManifoldContext *ctx, ManifoldBuffer *in,
                                       ManifoldBuffer *out,
                                       ManifoldBuffer *block_sums, int64_t n) {
  if (n <= 0)
    return;
  NSUInteger num_groups =
      (n + kThreadsPerThreadgroup - 1) / kThreadsPerThreadgroup;
  uint32_t nu = (uint32_t)n;
  KernelDispatch k(ctx, "exclusive_scan_u32_pass1");
  k.set_buffer(in, 0);
  k.set_buffer(out, 1);
  k.set_buffer(block_sums, 2);
  k.set_bytes(nu, 3);
  k.set_threadgroup_memory(kThreadsPerThreadgroup * sizeof(uint32_t), 0);
  k.dispatch_groups(num_groups, kThreadsPerThreadgroup);
}

void manifold_exclusive_scan_u32_add_block_offsets(ManifoldContext *ctx,
                                                   ManifoldBuffer *out,
                                                   ManifoldBuffer *block_prefix,
                                                   int64_t n) {
  if (n <= 0)
    return;
  NSUInteger num_groups =
      (n + kThreadsPerThreadgroup - 1) / kThreadsPerThreadgroup;
  uint32_t nu = (uint32_t)n;
  KernelDispatch k(ctx, "exclusive_scan_u32_add_block_offsets");
  k.set_buffer(out, 0);
  k.set_buffer(block_prefix, 1);
  k.set_bytes(nu, 2);
  k.dispatch_groups(num_groups, kThreadsPerThreadgroup);
}

void manifold_exclusive_scan_u32_finalize_total(ManifoldContext *ctx,
                                                ManifoldBuffer *in,
                                                ManifoldBuffer *out,
                                                int64_t n) {
  uint32_t nu = (uint32_t)n;
  KernelDispatch k(ctx, "exclusive_scan_u32_finalize_total");
  k.set_buffer(in, 0);
  k.set_buffer(out, 1);
  k.set_bytes(nu, 2);
  k.dispatch_groups(1, 1);
}

// ----------------------------------------------------------------------------
// 7. Coherence ω-Binning & Lattice Dynamics (GPE)
// ----------------------------------------------------------------------------
void manifold_coherence_reduce_omega_minmax_keys(
    ManifoldContext *ctx, ManifoldBuffer *carrier_omega,
    ManifoldBuffer *num_carriers_snapshot, ManifoldBuffer *omega_min_key,
    ManifoldBuffer *omega_max_key) {
  int64_t maxM = carrier_omega->size_bytes / sizeof(float);
  if (maxM == 0)
    return;
  KernelDispatch k(ctx, "coherence_reduce_omega_minmax_keys");
  k.set_buffer(carrier_omega, 0);
  k.set_buffer(num_carriers_snapshot, 1);
  k.set_buffer(omega_min_key, 2);
  k.set_buffer(omega_max_key, 3);
  k.dispatch_1d(maxM);
}

void manifold_coherence_compute_bin_params(
    ManifoldContext *ctx, ManifoldBuffer *omega_min_key,
    ManifoldBuffer *omega_max_key, ManifoldBuffer *num_carriers_snapshot,
    ManifoldBuffer *bin_params_out, float gate_width_max) {
  KernelDispatch k(ctx, "coherence_compute_bin_params");
  k.set_buffer(omega_min_key, 0);
  k.set_buffer(omega_max_key, 1);
  k.set_buffer(num_carriers_snapshot, 2);
  k.set_buffer(bin_params_out, 3);
  k.set_bytes(gate_width_max, 4);
  k.dispatch_groups(1, 1);
}

void manifold_coherence_bin_count(ManifoldContext *ctx,
                                  ManifoldBuffer *carrier_omega,
                                  ManifoldBuffer *num_carriers_snapshot,
                                  ManifoldBuffer *bin_counts,
                                  ManifoldBuffer *bin_params,
                                  int64_t num_bins) {
  int64_t maxM = carrier_omega->size_bytes / sizeof(float);
  if (maxM == 0)
    return;
  uint32_t nb = (uint32_t)num_bins;
  KernelDispatch k(ctx, "coherence_bin_count_carriers");
  k.set_buffer(carrier_omega, 0);
  k.set_buffer(num_carriers_snapshot, 1);
  k.set_buffer(bin_counts, 2);
  k.set_buffer(bin_params, 3);
  k.set_bytes(nb, 4);
  k.dispatch_1d(maxM);
}

void manifold_coherence_bin_scatter(ManifoldContext *ctx,
                                    ManifoldBuffer *carrier_omega,
                                    ManifoldBuffer *num_carriers_snapshot,
                                    ManifoldBuffer *bin_offsets,
                                    ManifoldBuffer *bin_params,
                                    int64_t num_bins,
                                    ManifoldBuffer *carrier_binned_idx) {
  int64_t maxM = carrier_omega->size_bytes / sizeof(float);
  if (maxM == 0)
    return;
  uint32_t nb = (uint32_t)num_bins;
  KernelDispatch k(ctx, "coherence_bin_scatter_carriers");
  k.set_buffer(carrier_omega, 0);
  k.set_buffer(num_carriers_snapshot, 1);
  k.set_buffer(bin_offsets, 2);
  k.set_buffer(bin_params, 3);
  k.set_bytes(nb, 4);
  k.set_buffer(carrier_binned_idx, 5);
  k.dispatch_1d(maxM);
}

void manifold_coherence_accumulate_forces(
    ManifoldContext *ctx, ManifoldBuffer *osc_phase, ManifoldBuffer *osc_omega,
    ManifoldBuffer *osc_amp, ManifoldBuffer *particle_pos,
    ManifoldBuffer *carrier_omega, ManifoldBuffer *carrier_gate_width,
    ManifoldBuffer *carrier_anchor_idx, ManifoldBuffer *carrier_anchor_weight,
    ManifoldBuffer *accums, ManifoldBuffer *bin_starts,
    ManifoldBuffer *carrier_binned_idx, ManifoldBuffer *bin_params,
    int64_t num_bins, ManifoldBuffer *particle_heat, int64_t num_osc,
    ManifoldBuffer *num_carriers_snapshot, int64_t max_carriers, float dt,
    float metabolic_rate, float gate_width_min, float gate_width_max,
    float offender_weight_floor, float domain_x, float domain_y, float domain_z,
    float spatial_sigma) {
  if (!runtime_ready(ctx, {osc_phase, osc_omega, osc_amp, particle_pos,
                           carrier_omega, carrier_gate_width,
                           carrier_anchor_idx, carrier_anchor_weight, accums,
                           bin_starts, carrier_binned_idx, bin_params,
                           particle_heat, num_carriers_snapshot}))
    return;
  // Bound every uint-indexed stride BEFORE dispatch; capacity is not inferred
  // from the requested count and a malformed snapshot cannot enlarge it.
  if (num_osc < 0 || uint64_t(num_osc) > UINT32_MAX / 6u || max_carriers < 0 ||
      uint64_t(max_carriers) > UINT32_MAX / 8u || num_bins < 0 ||
      uint64_t(num_bins) > UINT32_MAX || !std::isfinite(dt) || dt < 0 ||
      !std::isfinite(metabolic_rate) || metabolic_rate < 0 ||
      !std::isfinite(gate_width_min) || gate_width_min <= 0 ||
      !std::isfinite(gate_width_max) || gate_width_max < gate_width_min ||
      !std::isfinite(offender_weight_floor) || offender_weight_floor < 0 ||
      !std::isfinite(spatial_sigma) || spatial_sigma < 0 ||
      !std::isfinite(domain_x) || domain_x <= 0 || !std::isfinite(domain_y) ||
      domain_y <= 0 || !std::isfinite(domain_z) || domain_z <= 0) {
    ctx->fail("invalid coherence dimensions/parameters");
    return;
  }
  uint64_t n = uint64_t(num_osc), m = uint64_t(max_carriers);
  if (osc_phase->size_bytes < n * 4 || osc_omega->size_bytes < n * 4 ||
      osc_amp->size_bytes < n * 4 || particle_pos->size_bytes < n * 12 ||
      particle_heat->size_bytes < n * 4 || carrier_omega->size_bytes < m * 4 ||
      carrier_gate_width->size_bytes < m * 4 ||
      carrier_anchor_idx->size_bytes < m * 32 ||
      carrier_anchor_weight->size_bytes < m * 32 ||
      accums->size_bytes < m * 32 || num_carriers_snapshot->size_bytes < 4) {
    ctx->fail("undersized coherence buffer");
    return;
  }
  ManifoldBuffer *inputs[] = {osc_phase,
                              osc_omega,
                              osc_amp,
                              particle_pos,
                              carrier_omega,
                              carrier_gate_width,
                              carrier_anchor_idx,
                              carrier_anchor_weight,
                              num_carriers_snapshot};
  for (auto b : inputs)
    if (b->mtl_buffer == accums->mtl_buffer ||
        b->mtl_buffer == particle_heat->mtl_buffer) {
      ctx->fail("coherence writable/input alias");
      return;
    }
  if (accums->mtl_buffer == particle_heat->mtl_buffer) {
    ctx->fail("coherence writable alias");
    return;
  }
  if (num_osc == 0 || max_carriers == 0)
    return;
  SpectralModeParams prm = {};
  prm.num_osc = (uint32_t)num_osc;
  prm.max_carriers = (uint32_t)max_carriers;
  prm.dt = dt;
  prm.gate_width_min = gate_width_min;
  prm.gate_width_max = gate_width_max;
  prm.offender_weight_floor = offender_weight_floor;
  prm.domain_x = domain_x;
  prm.domain_y = domain_y;
  prm.domain_z = domain_z;
  prm.spatial_sigma = spatial_sigma;
  prm.metabolic_rate = metabolic_rate;

  KernelDispatch k(ctx, "coherence_accumulate_forces");
  k.set_buffer(osc_phase, 0);
  k.set_buffer(osc_omega, 1);
  k.set_buffer(osc_amp, 2);
  k.set_buffer(particle_pos, 3);
  k.set_buffer(carrier_omega, 4);
  k.set_buffer(carrier_gate_width, 5);
  k.set_buffer(carrier_anchor_idx, 6);
  k.set_buffer(carrier_anchor_weight, 7);
  k.set_buffer(accums, 8);
  k.set_bytes(prm, 9);
  k.set_buffer(num_carriers_snapshot, 10);
  k.set_buffer(bin_starts, 11);
  k.set_buffer(carrier_binned_idx, 12);
  k.set_buffer(bin_params, 13);
  uint32_t nb = (uint32_t)num_bins;
  k.set_bytes(nb, 14);
  k.set_buffer(particle_heat, 15);
  const NSUInteger shared_bytes =
      std::min<NSUInteger>(max_carriers, MANIFOLD_LOCAL_CARRIER_CAPACITY) *
      sizeof(ManifoldLocalCarrierAccumulator);
  if (!k.pso || shared_bytes + k.pso.staticThreadgroupMemoryLength >
                    ctx->device.maxThreadgroupMemoryLength) {
    ctx->fail("coherence local memory exceeds device limit");
    return;
  }
  k.set_threadgroup_memory((shared_bytes + 15) & ~NSUInteger(15), 0);
  k.buffer_barrier();
  k.dispatch_1d(num_osc);
  k.buffer_barrier();
}

void manifold_coherence_gpe_step_geometry(
    ManifoldContext *ctx, ManifoldBuffer *osc_phase, ManifoldBuffer *osc_omega,
    ManifoldBuffer *osc_amp, ManifoldBuffer *carrier_real,
    ManifoldBuffer *carrier_imag, ManifoldBuffer *carrier_omega,
    ManifoldBuffer *carrier_gate_width, ManifoldBuffer *kinetic_real,
    ManifoldBuffer *kinetic_imag, ManifoldBuffer *carrier_anchor_idx,
    ManifoldBuffer *carrier_anchor_weight, ManifoldBuffer *accums,
    ManifoldBuffer *num_carriers_snapshot, ManifoldBuffer *particle_pos,
    SpectralModeParams prm, GPEParams gp, ManifoldBuffer *extra_potential,
    ManifoldBuffer *metric_volume) {
  (void)osc_phase;
  (void)osc_omega;
  (void)osc_amp;
  (void)carrier_omega;
  (void)carrier_gate_width;
  (void)carrier_anchor_idx;
  (void)carrier_anchor_weight;
  (void)particle_pos;
  if (!runtime_ready(ctx, {carrier_real, carrier_imag, kinetic_real,
                           kinetic_imag, accums, num_carriers_snapshot}))
    return;
  if (gp.metric_coupling != 0) {
    ctx->fail("legacy scalar metric_coupling cannot specify a metric; supply "
              "positive metric_volume");
    return;
  }
  if (!prm.max_carriers)
    return;
  uint32_t count = mr_workspace_bound(prm.max_carriers);
  NSUInteger bytes = NSUInteger(count) * sizeof(MRComplex);
  uint64_t state_bytes = uint64_t(prm.max_carriers) * sizeof(float);
  if (!count || carrier_real->size_bytes < state_bytes ||
      carrier_imag->size_bytes < state_bytes ||
      kinetic_real->size_bytes < state_bytes ||
      kinetic_imag->size_bytes < state_bytes ||
      accums->size_bytes <
          uint64_t(prm.max_carriers) * sizeof(ManifoldCarrierAccumulator) ||
      num_carriers_snapshot->size_bytes < 4) {
    ctx->fail("invalid FFT capacity or undersized state/work buffers");
    return;
  }
  uint32_t geometry_flags =
      (extra_potential ? 1u : 0u) | (metric_volume ? 2u : 0u);
  ManifoldBuffer *optional[] = {extra_potential, metric_volume};
  for (unsigned k = 0; k < 2; ++k)
    if (optional[k]) {
      if (!runtime_ready(ctx, {optional[k]}) ||
          optional[k]->size_bytes < state_bytes) {
        ctx->fail("undersized spectral geometry");
        return;
      }
      ManifoldBuffer *writable[] = {carrier_real, carrier_imag,
                                    kinetic_real, kinetic_imag,
                                    accums,       num_carriers_snapshot};
      for (auto b : writable)
        if (b->mtl_buffer == optional[k]->mtl_buffer) {
          ctx->fail("spectral geometry aliases state/work");
          return;
        }
    }
  if (geometry_flags) {
    if (!manifold_synchronize_checked(ctx))
      return;
    for (unsigned k = 0; k < 2; ++k)
      if (optional[k]) {
        const float *v = static_cast<const float *>(
            manifold_get_buffer_pointer(optional[k]));
        if (!v) {
          ctx->fail("spectral geometry readback unavailable");
          return;
        }
        for (unsigned j = 0; j < prm.max_carriers; ++j)
          if (!std::isfinite(v[j]) || (k == 1 && !(v[j] > 0))) {
            ctx->fail("invalid scalar potential or nonpositive metric volume");
            return;
          }
      }
  }
  ManifoldBuffer *all[] = {carrier_real, carrier_imag, kinetic_real,
                           kinetic_imag, accums,       num_carriers_snapshot};
  for (unsigned i = 0; i < 6; i++)
    for (unsigned j = i + 1; j < 6; j++)
      if (all[i]->mtl_buffer == all[j]->mtl_buffer) {
        ctx->fail("FFT buffers must not alias");
        return;
      }
  if (!ctx->wave_ledger || ctx->wave_ledger.length < state_bytes * 6) {
    ctx->commit_and_wait();
    if (!ctx->error.empty())
      return;
    ctx->wave_ledger =
        [ctx->device newBufferWithLength:state_bytes * 6
                                 options:MTLResourceStorageModeShared];
    if (!ctx->wave_ledger) {
      ctx->fail("allocate wave ledger");
      return;
    }
  }
  ctx->wave_ledger_modes = prm.max_carriers;
  NSUInteger shared_bytes = (bytes + 15) & ~NSUInteger(15);
  id<MTLComputePipelineState> fast =
      ctx->get_pipeline("coherence_gpe_fft_fused");
  if (!fast)
    return;
  bool local = shared_bytes + fast.staticThreadgroupMemoryLength <=
               ctx->device.maxThreadgroupMemoryLength;
  id<MTLBuffer> work = local ? nil : ctx->scratch(bytes);
  if (!local && !work)
    return;
  KernelDispatch k(ctx, local ? "coherence_gpe_fft_fused"
                              : "coherence_gpe_fft_fused_global");
  if (!k.pso)
    return;
  k.buffer_barrier();
  k.set_buffer(carrier_real, 0);
  k.set_buffer(carrier_imag, 1);
  k.set_buffer(accums, 2);
  k.set_buffer(num_carriers_snapshot, 3);
  k.set_bytes(prm, 4);
  k.set_bytes(gp, 5);
  k.set_buffer(kinetic_real, 6);
  k.set_buffer(kinetic_imag, 7);
  k.set_bytes(count, 8);
  [ctx->current_encoder setBuffer:ctx->wave_ledger offset:0 atIndex:10];
  // Always bind legal buffers. Flags prevent reads of these unused aliases.
  k.set_buffer(extra_potential ? extra_potential : carrier_real, 11);
  k.set_buffer(metric_volume ? metric_volume : carrier_real, 12);
  k.set_bytes(geometry_flags, 13);
  if (local)
    k.set_threadgroup_memory(shared_bytes, 0);
  else
    [ctx->current_encoder setBuffer:work offset:0 atIndex:9];
  NSUInteger threads =
      std::min<NSUInteger>(256, k.pso.maxTotalThreadsPerThreadgroup);
  k.dispatch_groups(1, threads);
  k.buffer_barrier();
}

void manifold_coherence_gpe_step(
    ManifoldContext *ctx, ManifoldBuffer *osc_phase, ManifoldBuffer *osc_omega,
    ManifoldBuffer *osc_amp, ManifoldBuffer *carrier_real,
    ManifoldBuffer *carrier_imag, ManifoldBuffer *carrier_omega,
    ManifoldBuffer *carrier_gate_width, ManifoldBuffer *kinetic_real,
    ManifoldBuffer *kinetic_imag, ManifoldBuffer *carrier_anchor_idx,
    ManifoldBuffer *carrier_anchor_weight, ManifoldBuffer *accums,
    ManifoldBuffer *num_carriers_snapshot, ManifoldBuffer *particle_pos,
    SpectralModeParams prm, GPEParams gp, ManifoldBuffer *extra_potential) {
  manifold_coherence_gpe_step_geometry(
      ctx, osc_phase, osc_omega, osc_amp, carrier_real, carrier_imag,
      carrier_omega, carrier_gate_width, kinetic_real, kinetic_imag,
      carrier_anchor_idx, carrier_anchor_weight, accums, num_carriers_snapshot,
      particle_pos, prm, gp, extra_potential, nullptr);
}

void manifold_coherence_update_oscillator_phases(
    ManifoldContext *ctx, ManifoldBuffer *osc_phase, ManifoldBuffer *osc_omega,
    ManifoldBuffer *osc_amp, ManifoldBuffer *carrier_real,
    ManifoldBuffer *carrier_imag, ManifoldBuffer *carrier_omega,
    ManifoldBuffer *carrier_gate_width, ManifoldBuffer *carrier_anchor_idx,
    ManifoldBuffer *carrier_anchor_weight,
    ManifoldBuffer *num_carriers_snapshot, SpectralModeParams prm,
    ManifoldBuffer *bin_starts, ManifoldBuffer *carrier_binned_idx,
    ManifoldBuffer *bin_params, int64_t num_bins,
    ManifoldBuffer *particle_pos) {
  if (!runtime_ready(ctx,
                     {osc_phase, osc_omega, osc_amp, carrier_real, carrier_imag,
                      carrier_omega, carrier_gate_width, carrier_anchor_idx,
                      carrier_anchor_weight, num_carriers_snapshot, bin_starts,
                      carrier_binned_idx, bin_params, particle_pos}))
    return;
  if (prm.num_osc > UINT32_MAX / 6u || prm.max_carriers > UINT32_MAX / 8u ||
      num_bins < 0 || uint64_t(num_bins) > UINT32_MAX ||
      !std::isfinite(prm.dt) || prm.dt < 0 ||
      !std::isfinite(prm.coupling_scale) || prm.coupling_scale < 0 ||
      !std::isfinite(prm.spatial_sigma) || prm.spatial_sigma < 0 ||
      !std::isfinite(prm.domain_x) || prm.domain_x <= 0 ||
      !std::isfinite(prm.domain_y) || prm.domain_y <= 0 ||
      !std::isfinite(prm.domain_z) || prm.domain_z <= 0) {
    ctx->fail("invalid phase dimensions/parameters");
    return;
  }
  uint64_t n = prm.num_osc, m = prm.max_carriers;
  if (osc_phase->size_bytes < n * 4 || osc_omega->size_bytes < n * 4 ||
      osc_amp->size_bytes < n * 4 || particle_pos->size_bytes < n * 12 ||
      carrier_real->size_bytes < m * 4 || carrier_imag->size_bytes < m * 4 ||
      carrier_omega->size_bytes < m * 4 ||
      carrier_gate_width->size_bytes < m * 4 ||
      carrier_anchor_idx->size_bytes < m * 32 ||
      carrier_anchor_weight->size_bytes < m * 32 ||
      num_carriers_snapshot->size_bytes < 4) {
    ctx->fail("undersized phase buffer");
    return;
  }
  ManifoldBuffer *inputs[] = {osc_omega,
                              osc_amp,
                              particle_pos,
                              carrier_real,
                              carrier_imag,
                              carrier_omega,
                              carrier_gate_width,
                              carrier_anchor_idx,
                              carrier_anchor_weight,
                              num_carriers_snapshot};
  for (auto b : inputs)
    if (b->mtl_buffer == osc_phase->mtl_buffer) {
      ctx->fail("phase writable/input alias");
      return;
    }
  if (prm.num_osc == 0)
    return;
  uint64_t ledger_bytes = uint64_t(prm.num_osc) * 24;
  if (!ctx->phase_ledger || ctx->phase_ledger.length < ledger_bytes) {
    ctx->commit_and_wait();
    if (!ctx->error.empty())
      return;
    ctx->phase_ledger =
        [ctx->device newBufferWithLength:ledger_bytes
                                 options:MTLResourceStorageModeShared];
    if (!ctx->phase_ledger) {
      ctx->fail("allocate phase ledger");
      return;
    }
  }
  ctx->phase_ledger_particles = prm.num_osc;
  KernelDispatch k(ctx, "coherence_update_oscillator_phases");
  k.set_buffer(osc_phase, 0);
  k.set_buffer(osc_omega, 1);
  k.set_buffer(osc_amp, 2);
  k.set_buffer(carrier_real, 3);
  k.set_buffer(carrier_imag, 4);
  k.set_buffer(carrier_omega, 5);
  k.set_buffer(carrier_gate_width, 6);
  k.set_buffer(carrier_anchor_idx, 7);
  k.set_buffer(carrier_anchor_weight, 8);
  k.set_buffer(num_carriers_snapshot, 9);
  k.set_bytes(prm, 10);
  k.set_buffer(bin_starts, 11);
  k.set_buffer(carrier_binned_idx, 12);
  k.set_buffer(bin_params, 13);
  uint32_t nb = (uint32_t)num_bins;
  k.set_bytes(nb, 14);
  k.set_buffer(particle_pos, 15);
  [ctx->current_encoder setBuffer:ctx->phase_ledger offset:0 atIndex:16];
  k.dispatch_1d(prm.num_osc);
}

// ----------------------------------------------------------------------------
// 8. Particle Generation
// ----------------------------------------------------------------------------
void manifold_generate_particles(
    ManifoldContext *ctx, ManifoldBuffer *positions, ManifoldBuffer *velocities,
    ManifoldBuffer *energies, ManifoldBuffer *heats,
    ManifoldBuffer *excitations, ManifoldBuffer *masses,
    ManifoldBuffer *random_pos, ManifoldBuffer *random_props,
    ParticleGenParams prm) {
  int64_t n = prm.num_particles;
  if (n == 0)
    return;

  {
    KernelDispatch k(ctx, "generate_particle_positions");
    k.set_buffer(positions, 0);
    k.set_buffer(random_pos, 1);
    k.set_bytes(prm, 2);
    k.dispatch_1d(n);
  }

  // CPU Barrier to calculate center position
  ctx->commit_and_wait();

  float *pos_ptr = (float *)positions->mtl_buffer.contents;
  float sum_x = 0, sum_y = 0, sum_z = 0;
  for (int64_t i = 0; i < n; i++) {
    sum_x += pos_ptr[i * 3 + 0];
    sum_y += pos_ptr[i * 3 + 1];
    sum_z += pos_ptr[i * 3 + 2];
  }
  float mean_x = sum_x / (float)n;
  float mean_y = sum_y / (float)n;
  float mean_z = sum_z / (float)n;

  {
    KernelDispatch k(ctx, "initialize_particle_properties");
    k.set_buffer(positions, 0);
    k.set_buffer(velocities, 1);
    k.set_buffer(energies, 2);
    k.set_buffer(heats, 3);
    k.set_buffer(excitations, 4);
    k.set_buffer(masses, 5);
    k.set_buffer(random_props, 6);
    k.set_bytes(prm, 7);
    k.set_bytes(mean_x, 8);
    k.set_bytes(mean_y, 9);
    k.set_bytes(mean_z, 10);
    k.dispatch_1d(n);
  }
}

} // extern "C"

extern "C" bool manifold_coherence_copy_ledger(ManifoldContext *ctx,
                                               ManifoldBuffer *out,
                                               unsigned n) {
  if (!runtime_ready(ctx, {out}))
    return false;
  ctx->commit_and_wait();
  if (!ctx->error.empty())
    return false;
  const size_t bytes = size_t(n) * 6 * sizeof(float);
  if (n > ctx->wave_ledger_modes || out->size_bytes < bytes ||
      (bytes && (!ctx->wave_ledger || ctx->wave_ledger.length < bytes))) {
    ctx->fail("wave ledger read exceeds completed storage");
    return false;
  }
  if (bytes)
    memcpy(out->mtl_buffer.contents, ctx->wave_ledger.contents, bytes);
  return true;
}

extern "C" bool manifold_phase_copy_ledger(ManifoldContext *ctx,
                                           ManifoldBuffer *out, unsigned n) {
  if (!runtime_ready(ctx, {out}))
    return false;
  ctx->commit_and_wait();
  if (!ctx->error.empty())
    return false;
  size_t bytes = size_t(n) * 24;
  if (n > ctx->phase_ledger_particles || out->size_bytes < bytes ||
      (bytes && (!ctx->phase_ledger || ctx->phase_ledger.length < bytes))) {
    ctx->fail("phase ledger bounds");
    return false;
  }
  if (bytes)
    memcpy(out->mtl_buffer.contents, ctx->phase_ledger.contents, bytes);
  return true;
}
