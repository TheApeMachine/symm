#include "kernels.cuh"
#include "coupled_kernels.cuh"
#include <atomic>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <initializer_list>
#include <limits>
#include <new>
#include "../shared/radix2_math.h"
#include "../shared/scan_plan.h"
#include <algorithm>

namespace kernels = sensorium::kernels;
using uint = unsigned;
using kernels::CarrierAccumulators;
using kernels::CoherenceBinParams;
static constexpr unsigned kBlockSize = 256;

// An Engine owns one device and one non-default stream. There is no process-
// global current Engine. Each boundary call restores the device because a Go
// goroutine need not return on the OS thread that created the context.
struct ManifoldContext {
    int device = 0;
    cudaStream_t stream = nullptr;
    std::atomic<unsigned> references{1};
    bool closed = false;
    bool pending = false;
    char error[512]{};
    float* scratch = nullptr;
    size_t scratch_bytes = 0;
    float* wave_ledger = nullptr;
    size_t wave_ledger_bytes = 0;
    unsigned wave_ledger_modes = 0;
    float* phase_ledger=nullptr;size_t phase_ledger_bytes=0;unsigned phase_ledger_particles=0;

    void fail(const char* message) {
        if (!error[0]) std::snprintf(error, sizeof(error), "%s", message);
    }
    bool record(cudaError_t status, const char* operation) {
        if (status != cudaSuccess && !error[0]) {
            std::snprintf(error, sizeof(error), "%s: %s", operation, cudaGetErrorString(status));
        }
        return status == cudaSuccess;
    }
    bool select() { return record(cudaSetDevice(device), "cudaSetDevice"); }
    void launch_status(cudaError_t status, const char* name) {
        if (record(status, name)) pending = true;
    }
    bool synchronize() {
        if (!select()) return false;
        if (pending) {
            record(cudaStreamSynchronize(stream), "cudaStreamSynchronize");
            pending = false;
        }
        return !error[0];
    }
    float* reduction_scratch(size_t bytes) {
        if (bytes <= scratch_bytes) return scratch;
        if (!synchronize()) return nullptr;
        if (scratch && !record(cudaFree(scratch), "free reduction scratch")) return nullptr;
        scratch = nullptr;
        scratch_bytes = 0;
        if (!record(cudaMalloc(reinterpret_cast<void**>(&scratch), bytes), "allocate reduction scratch")) return nullptr;
        scratch_bytes = bytes;
        return scratch;
    }
    float* ensure_wave_ledger(size_t bytes) {
        if (bytes<=wave_ledger_bytes) return wave_ledger;
        if (!synchronize()) return nullptr;
        if (wave_ledger && !record(cudaFree(wave_ledger),"free wave ledger")) return nullptr;
        wave_ledger=nullptr;wave_ledger_bytes=0;wave_ledger_modes=0;
        if (!record(cudaMalloc(reinterpret_cast<void**>(&wave_ledger),bytes),"allocate wave ledger")) return nullptr;
        wave_ledger_bytes=bytes; return wave_ledger;
    }
    float* ensure_phase_ledger(size_t bytes){
        if(bytes<=phase_ledger_bytes)return phase_ledger;
        if(!synchronize())return nullptr;
        if(phase_ledger&&!record(cudaFree(phase_ledger),"free phase ledger"))return nullptr;
        phase_ledger=nullptr;phase_ledger_bytes=0;phase_ledger_particles=0;
        if(!record(cudaMalloc(reinterpret_cast<void**>(&phase_ledger),bytes),"allocate phase ledger"))return nullptr;
        phase_ledger_bytes=bytes;return phase_ledger;
    }
    ~ManifoldContext() {
        cudaSetDevice(device);
        if (stream) cudaStreamSynchronize(stream);
        if (scratch) cudaFree(scratch);
        if (wave_ledger) cudaFree(wave_ledger);
        if (phase_ledger) cudaFree(phase_ledger);
        if (stream) cudaStreamDestroy(stream);
    }
};

struct ManifoldBuffer {
    ManifoldContext* owner;
    void* data;
    uint64_t size_bytes;
};

static void release(ManifoldContext* ctx) {
    if (ctx && ctx->references.fetch_sub(1, std::memory_order_acq_rel) == 1) delete ctx;
}
static bool ready(ManifoldContext* ctx, std::initializer_list<ManifoldBuffer*> buffers) {
    if (!ctx) return false;
    if (ctx->closed) { ctx->fail("CUDA context is closed"); return false; }
    if (ctx->error[0] || !ctx->select()) return false;
    for (const auto* buffer : buffers) {
        if (!buffer || !buffer->data || buffer->owner != ctx) {
            ctx->fail("missing buffer or buffer belongs to another CUDA context");
            return false;
        }
        if (buffer->size_bytes / 4 > UINT32_MAX) {
            ctx->fail("buffer exceeds the source kernel's uint32 element indexing");
            return false;
        }
    }
    return true;
}
static bool valid_grid(ManifoldContext* ctx, int64_t x, int64_t y, int64_t z, float spacing) {
    // Momentum indexing uses three uint32 scalar indices per cell.
    if (x <= 0 || y <= 0 || z <= 0 || !std::isfinite(spacing) || !(spacing > 0) ||
        uint64_t(x) > uint64_t(UINT32_MAX)/3/uint64_t(y)/uint64_t(z)) {
        ctx->fail("grid dimensions/spacing invalid or exceed uint32 indexing");
        return false;
    }
    return true;
}
static unsigned blocks(uint64_t n) { return unsigned((n + kBlockSize - 1) / kBlockSize); }
template<class T> static T* ptr(ManifoldBuffer* buffer) {
    return buffer ? static_cast<T*>(buffer->data) : nullptr;
}

extern "C" {

ManifoldContext* manifold_create_cuda_context(int device, char* error, uint64_t capacity) {
    auto message = [&](const char* op, cudaError_t status) {
        if (error && capacity) std::snprintf(error, size_t(capacity), "%s: %s", op, cudaGetErrorString(status));
    };
    if (error && capacity) error[0] = '\0';
    int count = 0;
    cudaError_t status = cudaGetDeviceCount(&count);
    if (status != cudaSuccess) { message("cudaGetDeviceCount", status); return nullptr; }
    if (device < 0 || device >= count) { message("CUDA device ordinal", cudaErrorInvalidDevice); return nullptr; }
    status = cudaSetDevice(device);
    if (status != cudaSuccess) { message("cudaSetDevice", status); return nullptr; }
    int managed = 0;
    status = cudaDeviceGetAttribute(&managed, cudaDevAttrManagedMemory, device);
    if (status != cudaSuccess) { message("managed memory capability", status); return nullptr; }
    if (!managed) { message("CUDA managed memory required for the Go host views", cudaErrorNotSupported); return nullptr; }
    auto* ctx = new (std::nothrow) ManifoldContext;
    if (!ctx) { message("allocate context", cudaErrorMemoryAllocation); return nullptr; }
    ctx->device = device;
    status = cudaStreamCreateWithFlags(&ctx->stream, cudaStreamNonBlocking);
    if (status != cudaSuccess) { message("cudaStreamCreateWithFlags", status); delete ctx; return nullptr; }
    return ctx;
}

ManifoldContext* manifold_create_context(const char* /* unused_metallib_path */) {
    // Existing C entry point selects device 0. CUDA code is compiled, not loaded
    // from a Metal library. Use the explicit device constructor for another GPU.
    return manifold_create_cuda_context(0, nullptr, 0);
}
void manifold_destroy_context(ManifoldContext* ctx) {
    if (!ctx || ctx->closed) return;
    ctx->synchronize();
    ctx->closed = true;
    // Buffers retain the context until they are destroyed; closing in either
    // order cannot leave a buffer with a dangling owner/stream.
    release(ctx);
}
void manifold_synchronize(ManifoldContext* ctx) { if (ctx) ctx->synchronize(); }
bool manifold_synchronize_checked(ManifoldContext* ctx) {
    return ctx && !ctx->closed && ctx->synchronize();
}
const char* manifold_last_error(ManifoldContext* ctx) {
    return ctx ? ctx->error : "CUDA context is null";
}
int manifold_cuda_device(ManifoldContext* ctx) { return ctx ? ctx->device : -1; }

ManifoldBuffer* manifold_create_buffer(ManifoldContext* ctx, uint64_t bytes, const void* initial_data) {
    if (!ready(ctx, {})) return nullptr;
    if (!bytes || bytes > std::numeric_limits<size_t>::max()) {
        ctx->fail("buffer size must be positive and representable by size_t"); return nullptr;
    }
    auto* buffer = new (std::nothrow) ManifoldBuffer{ctx, nullptr, bytes};
    if (!buffer) { ctx->fail("failed to allocate buffer descriptor"); return nullptr; }
    // AttachHost prevents unrelated streams from making a fresh allocation
    // inaccessible to the CPU during initialization. Then bind it to this
    // context's single stream, including on limited-managed-memory devices.
    if (!ctx->record(cudaMallocManaged(&buffer->data, size_t(bytes), cudaMemAttachHost), "cudaMallocManaged")) {
        delete buffer; return nullptr;
    }
    if (initial_data) std::memcpy(buffer->data, initial_data, size_t(bytes));
    else std::memset(buffer->data, 0, size_t(bytes));
    if (!ctx->record(cudaStreamAttachMemAsync(ctx->stream, buffer->data, 0, cudaMemAttachSingle), "cudaStreamAttachMemAsync")) {
        cudaFree(buffer->data); delete buffer; return nullptr;
    }
    ctx->pending = true;
    ctx->references.fetch_add(1, std::memory_order_relaxed);
    return buffer;
}
void manifold_destroy_buffer(ManifoldBuffer* buffer) {
    if (!buffer) return;
    auto* ctx = buffer->owner;
    ctx->synchronize();
    ctx->record(cudaFree(buffer->data), "cudaFree");
    delete buffer;
    release(ctx);
}
void* manifold_get_buffer_pointer(ManifoldBuffer* buffer) {
    if (!buffer || buffer->owner->closed) return nullptr;
    // CPU reads/writes must not race an already-enqueued CUDA kernel. Merely
    // using managed memory does NOT supply that synchronization.
    if (!buffer->owner->synchronize()) return nullptr;
    return buffer->data;
}
uint64_t manifold_get_buffer_size(ManifoldBuffer* buffer) { return buffer ? buffer->size_bytes : 0; }

// ----------------------------------------------------------------------------
// 1. Diagnostics & Memory
// ----------------------------------------------------------------------------
void manifold_clear_field(ManifoldContext* ctx, ManifoldBuffer* field) {
    if (!ready(ctx, {field})) return;
    uint32_t n = (uint32_t)(field->size_bytes / sizeof(float));
    if (n == 0) return;
    kernels::clear_field<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
        ptr<float>(field),
        n
    );
    ctx->launch_status(cudaGetLastError(), "clear_field");
}

void manifold_thermo_reduce_energy_stats(ManifoldContext* ctx, ManifoldBuffer* x, ManifoldBuffer* out_stats) {
    if (!ready(ctx, {x, out_stats})) return;
    int64_t n = x->size_bytes / sizeof(float);
    if (n <= 0) return;
    size_t num_groups = (n + kBlockSize - 1) / kBlockSize;

    // Group scratch allocation
    float* group_stats = ctx->reduction_scratch(num_groups * 4 * sizeof(float));
    if (!group_stats) return;

    {
        uint32_t nu = (uint32_t)n;
        kernels::reduce_float_stats_pass1<<<num_groups, kBlockSize, 0, ctx->stream>>>(
            ptr<const float>(x),
            group_stats,
            nu
        );
        ctx->launch_status(cudaGetLastError(), "reduce_float_stats_pass1");
    }
    {
        uint32_t gu = (uint32_t)num_groups;
        kernels::reduce_float_stats_finalize<<<1, kBlockSize, 0, ctx->stream>>>(
            group_stats,
            ptr<float>(out_stats),
            gu
        );
        ctx->launch_status(cudaGetLastError(), "reduce_float_stats_finalize");
    }
    // Reused only on this stream; retained until context destruction.
}

// ----------------------------------------------------------------------------
// 2. PIC & Sort-Based Scatter
// ----------------------------------------------------------------------------
void manifold_scatter_compute_cell_idx(
    ManifoldContext* ctx,
    ManifoldBuffer* particle_pos,
    ManifoldBuffer* particle_cell_idx,
    int64_t gx, int64_t gy, int64_t gz,
    float grid_spacing
) {
    if (!ready(ctx, {particle_pos, particle_cell_idx})) return;
    if (!valid_grid(ctx, gx, gy, gz, grid_spacing)) return;
    int64_t n = particle_pos->size_bytes / (3 * sizeof(float));
    if (n == 0) return;
    SortScatterParams prm = {
        (uint32_t)n, (uint32_t)(gx * gy * gz),
        (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
        grid_spacing, 1.0f / grid_spacing
    };
    kernels::scatter_compute_cell_idx<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(particle_pos),
        ptr<uint>(particle_cell_idx),
        prm
    );
    ctx->launch_status(cudaGetLastError(), "scatter_compute_cell_idx");
}

void manifold_scatter_count_cells(
    ManifoldContext* ctx,
    ManifoldBuffer* particle_cell_idx,
    ManifoldBuffer* cell_counts,
    int64_t gx, int64_t gy, int64_t gz,
    float grid_spacing
) {
    if (!ready(ctx, {particle_cell_idx, cell_counts})) return;
    if (!valid_grid(ctx, gx, gy, gz, grid_spacing)) return;
    int64_t n = particle_cell_idx->size_bytes / sizeof(uint32_t);
    if (n == 0) return;
    SortScatterParams prm = {
        (uint32_t)n, (uint32_t)(gx * gy * gz),
        (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
        grid_spacing, 1.0f / grid_spacing
    };
    kernels::scatter_count_cells<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
        ptr<const uint>(particle_cell_idx),
        ptr<unsigned>(cell_counts),
        prm
    );
    ctx->launch_status(cudaGetLastError(), "scatter_count_cells");
}

void manifold_scatter_reorder_particles(
    ManifoldContext* ctx,
    ManifoldBuffer* pos_in,
    ManifoldBuffer* vel_in,
    ManifoldBuffer* mass_in,
    ManifoldBuffer* heat_in,
    ManifoldBuffer* energy_in,
    ManifoldBuffer* particle_cell_idx,
    ManifoldBuffer* cell_starts,
    ManifoldBuffer* cell_offsets,
    ManifoldBuffer* pos_out,
    ManifoldBuffer* vel_out,
    ManifoldBuffer* mass_out,
    ManifoldBuffer* heat_out,
    ManifoldBuffer* energy_out,
    ManifoldBuffer* sorted_original_idx,
    int64_t gx, int64_t gy, int64_t gz,
    float grid_spacing
) {
    if (!ready(ctx, {pos_in, vel_in, mass_in, heat_in, energy_in, particle_cell_idx, cell_starts, cell_offsets, pos_out, vel_out, mass_out, heat_out, energy_out, sorted_original_idx})) return;
    if (!valid_grid(ctx, gx, gy, gz, grid_spacing)) return;
    int64_t n = pos_in->size_bytes / (3 * sizeof(float));
    if (n == 0) return;
    SortScatterParams prm = {
        (uint32_t)n, (uint32_t)(gx * gy * gz),
        (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
        grid_spacing, 1.0f / grid_spacing
    };
    kernels::scatter_reorder_particles<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(pos_in),
        ptr<const float>(vel_in),
        ptr<const float>(mass_in),
        ptr<const float>(heat_in),
        ptr<const float>(energy_in),
        ptr<const uint>(particle_cell_idx),
        ptr<const uint>(cell_starts),
        ptr<unsigned>(cell_offsets),
        ptr<float>(pos_out),
        ptr<float>(vel_out),
        ptr<float>(mass_out),
        ptr<float>(heat_out),
        ptr<float>(energy_out),
        ptr<uint>(sorted_original_idx),
        prm
    );
    ctx->launch_status(cudaGetLastError(), "scatter_reorder_particles");
}

void manifold_scatter_sorted(
    ManifoldContext* ctx,
    ManifoldBuffer* pos,
    ManifoldBuffer* vel,
    ManifoldBuffer* mass,
    ManifoldBuffer* heat,
    ManifoldBuffer* energy,
    ManifoldBuffer* rho_field,
    ManifoldBuffer* mom_field,
    ManifoldBuffer* E_field,
    int64_t gx, int64_t gy, int64_t gz,
    float grid_spacing
) {
    if (!ready(ctx, {pos, vel, mass, heat, energy, rho_field, mom_field, E_field})) return;
    if (!valid_grid(ctx, gx, gy, gz, grid_spacing)) return;
    int64_t n = pos->size_bytes / (3 * sizeof(float));
    if (n == 0) return;
    SortScatterParams prm = {
        (uint32_t)n, (uint32_t)(gx * gy * gz),
        (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
        grid_spacing, 1.0f / grid_spacing
    };
    kernels::scatter_sorted<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(pos),
        ptr<const float>(vel),
        ptr<const float>(mass),
        ptr<const float>(heat),
        ptr<const float>(energy),
        ptr<float>(rho_field),
        ptr<float>(mom_field),
        ptr<float>(E_field),
        prm
    );
    ctx->launch_status(cudaGetLastError(), "scatter_sorted");
}

void manifold_pic_gather_update_particles(
    ManifoldContext* ctx,
    ManifoldBuffer* pos_in,
    ManifoldBuffer* mass,
    ManifoldBuffer* pos_out,
    ManifoldBuffer* vel_out,
    ManifoldBuffer* heat_out,
    ManifoldBuffer* rho_field,
    ManifoldBuffer* mom_field,
    ManifoldBuffer* E_field,
    ManifoldBuffer* gravity_potential,
    ManifoldBuffer* dbg_head,
    ManifoldBuffer* dbg_words,
    int64_t dbg_capacity,
    int64_t gx, int64_t gy, int64_t gz,
    float grid_spacing,
    float dt,
    float domain_x, float domain_y, float domain_z,
    float gamma, float R_specific, float c_v,
    float rho_min, float p_min,
    float gravity_enabled
) {
    if (!ready(ctx, {pos_in, mass, pos_out, vel_out, heat_out, rho_field, mom_field, E_field})) return;
    if (!valid_grid(ctx, gx, gy, gz, grid_spacing)) return;
    if (dbg_capacity < 0 || dbg_capacity > UINT32_MAX) { ctx->fail("invalid debug capacity"); return; }
    if (dbg_capacity && !ready(ctx, {dbg_head, dbg_words})) return;
    if (gravity_enabled > 0.5f && !ready(ctx, {gravity_potential})) return;
    int64_t n = pos_in->size_bytes / (3 * sizeof(float));
    if (n == 0) return;
    PicGatherParams prm = {
        (uint32_t)n, (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
        grid_spacing, 1.0f / grid_spacing, dt,
        domain_x, domain_y, domain_z,
        gamma, R_specific, c_v,
        rho_min, p_min, gravity_enabled
    };
    uint32_t cap_u32 = (uint32_t)dbg_capacity;
    kernels::pic_gather_update_particles<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(pos_in),
        ptr<const float>(mass),
        ptr<float>(pos_out),
        ptr<float>(vel_out),
        ptr<float>(heat_out),
        ptr<const float>(rho_field),
        ptr<const float>(mom_field),
        ptr<const float>(E_field),
        ptr<const float>(gravity_potential),
        prm,
        ptr<unsigned>(dbg_head),
        ptr<uint>(dbg_words),
        cap_u32
    );
    ctx->launch_status(cudaGetLastError(), "pic_gather_update_particles");
}

// ----------------------------------------------------------------------------
// 3. Quantum Flow & Waves
// ----------------------------------------------------------------------------
void manifold_project_modes_to_spatial_psi(
    ManifoldContext* ctx,
    ManifoldBuffer* mode_psi_real,
    ManifoldBuffer* mode_psi_imag,
    ManifoldBuffer* mode_anchor_idx,
    ManifoldBuffer* mode_anchor_weight,
    ManifoldBuffer* particle_pos,
    ManifoldBuffer* psi_re_field,
    ManifoldBuffer* psi_im_field,
    int64_t anchors_per_mode,
    int64_t gx, int64_t gy, int64_t gz,
    float grid_spacing
) {
    if (ctx && (anchors_per_mode < 0 || uint64_t(anchors_per_mode) > UINT32_MAX)) { ctx->fail("count outside source uint32 range"); return; }
    if (!ready(ctx, {mode_psi_real, mode_psi_imag, mode_anchor_idx, mode_anchor_weight, particle_pos, psi_re_field, psi_im_field})) return;
    if (!valid_grid(ctx, gx, gy, gz, grid_spacing)) return;
    int64_t num_modes = mode_psi_real->size_bytes / sizeof(float);
    int64_t num_particles = particle_pos->size_bytes / (3 * sizeof(float));
    int64_t total = num_modes * anchors_per_mode;
    if (total <= 0) return;

    ModeProjectParams p = {
        (uint32_t)num_modes, (uint32_t)num_particles, (uint32_t)anchors_per_mode,
        (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
        grid_spacing, 1.0f / grid_spacing
    };
    kernels::project_modes_to_spatial_psi<<<blocks(total), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(mode_psi_real),
        ptr<const float>(mode_psi_imag),
        ptr<const uint>(mode_anchor_idx),
        ptr<const float>(mode_anchor_weight),
        ptr<const float>(particle_pos),
        ptr<float>(psi_re_field),
        ptr<float>(psi_im_field),
        p
    );
    ctx->launch_status(cudaGetLastError(), "project_modes_to_spatial_psi");
}

void manifold_pic_gather_pilot_wave(
    ManifoldContext* ctx,
    ManifoldBuffer* pos_in,
    ManifoldBuffer* mass,
    ManifoldBuffer* pos_out,
    ManifoldBuffer* vel_out,
    ManifoldBuffer* psi_re,
    ManifoldBuffer* psi_im,
    int64_t num_particles,
    int64_t gx, int64_t gy, int64_t gz,
    float grid_spacing,
    float dt,
    float domain_x, float domain_y, float domain_z,
    float hbar_eff, float eps_denom, float mass_min
) {
    if(!ready(ctx,{pos_in,mass,pos_out,vel_out,psi_re,psi_im}))return;
    if(num_particles<0||uint64_t(num_particles)>UINT32_MAX/3u||gx<=0||gy<=0||gz<=0||uint64_t(gx)>UINT32_MAX||uint64_t(gy)>UINT32_MAX||uint64_t(gz)>UINT32_MAX||
       uint64_t(gx)*uint64_t(gy)>UINT32_MAX/uint64_t(gz)||!std::isfinite(grid_spacing)||grid_spacing<=0||
       !std::isfinite(dt)||dt<0||!std::isfinite(hbar_eff)||hbar_eff<=0||!std::isfinite(eps_denom)||eps_denom<0||!std::isfinite(mass_min)||mass_min<=0||
       !std::isfinite(domain_x)||domain_x<=0||!std::isfinite(domain_y)||domain_y<=0||!std::isfinite(domain_z)||domain_z<=0) {ctx->fail("invalid pilot dimensions/parameters");return;}
    const double dx=grid_spacing;
    if(std::abs(double(domain_x)-double(gx)*dx)>8e-7*domain_x || std::abs(double(domain_y)-double(gy)*dx)>8e-7*domain_y ||
       std::abs(double(domain_z)-double(gz)*dx)>8e-7*domain_z) {ctx->fail("pilot domain differs from periodic field extent");return;}
    uint64_t np=uint64_t(num_particles),cells=uint64_t(gx)*uint64_t(gy)*uint64_t(gz);
    if(pos_in->size_bytes<np*12||mass->size_bytes<np*4||pos_out->size_bytes<np*12||vel_out->size_bytes<np*12||psi_re->size_bytes<cells*4||psi_im->size_bytes<cells*4) {ctx->fail("undersized pilot buffer");return;}
    ManifoldBuffer* inputs[]={pos_in,mass,psi_re,psi_im};
    for(auto b:inputs)if(b->data==pos_out->data||b->data==vel_out->data){ctx->fail("pilot writable/input alias");return;}
    if(pos_out->data==vel_out->data){ctx->fail("pilot writable alias");return;}
    if(num_particles==0)return;
    PilotWaveParams p = {
        (uint32_t)num_particles, (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
        grid_spacing, 1.0f / grid_spacing, dt,
        domain_x, domain_y, domain_z,
        hbar_eff, eps_denom, mass_min
    };
    kernels::pic_gather_update_particles_pilot_wave<<<blocks(num_particles), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(pos_in),
        ptr<const float>(mass),
        ptr<float>(pos_out),
        ptr<float>(vel_out),
        ptr<const float>(psi_re),
        ptr<const float>(psi_im),
        p
    );
    ctx->launch_status(cudaGetLastError(), "pic_gather_update_particles_pilot_wave");
}

// ----------------------------------------------------------------------------
// 4. Gas Dynamics (Eulerian RK2)
// ----------------------------------------------------------------------------
void manifold_gas_rk2_stage1(
    ManifoldContext* ctx,
    ManifoldBuffer* rho0, ManifoldBuffer* mom0, ManifoldBuffer* e0,
    ManifoldBuffer* rho1, ManifoldBuffer* mom1, ManifoldBuffer* e1,
    ManifoldBuffer* k1_rho, ManifoldBuffer* k1_mom, ManifoldBuffer* k1_e,
    ManifoldBuffer* dbg_head, ManifoldBuffer* dbg_words,
    int64_t dbg_capacity,
    int64_t gx, int64_t gy, int64_t gz,
    float dx, float dt, float gamma, float c_v,
    float rho_min, float p_min, float mu, float k_thermal
) {
    if (!ready(ctx, {rho0, mom0, e0, rho1, mom1, e1, k1_rho, k1_mom, k1_e})) return;
    if (!valid_grid(ctx, gx, gy, gz, dx)) return;
    if (dbg_capacity < 0 || dbg_capacity > UINT32_MAX) { ctx->fail("invalid debug capacity"); return; }
    if (dbg_capacity && !ready(ctx, {dbg_head, dbg_words})) return;
    int64_t n = gx * gy * gz;
    GasGridParams prm = {
        (uint32_t)n, (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
        dx, dt, gamma, c_v, rho_min, p_min, mu, k_thermal
    };
    uint32_t cap = (uint32_t)dbg_capacity;
    kernels::gas_rk2_stage1<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(rho0),
        ptr<const float>(mom0),
        ptr<const float>(e0),
        ptr<float>(rho1),
        ptr<float>(mom1),
        ptr<float>(e1),
        ptr<float>(k1_rho),
        ptr<float>(k1_mom),
        ptr<float>(k1_e),
        prm,
        ptr<unsigned>(dbg_head),
        ptr<uint>(dbg_words),
        cap
    );
    ctx->launch_status(cudaGetLastError(), "gas_rk2_stage1");
}

void manifold_gas_rk2_stage2(
    ManifoldContext* ctx,
    ManifoldBuffer* rho0, ManifoldBuffer* mom0, ManifoldBuffer* e0,
    ManifoldBuffer* rho1, ManifoldBuffer* mom1, ManifoldBuffer* e1,
    ManifoldBuffer* k1_rho, ManifoldBuffer* k1_mom, ManifoldBuffer* k1_e,
    ManifoldBuffer* rho_out, ManifoldBuffer* mom_out, ManifoldBuffer* e_out,
    ManifoldBuffer* dbg_head, ManifoldBuffer* dbg_words,
    int64_t dbg_capacity,
    int64_t gx, int64_t gy, int64_t gz,
    float dx, float dt, float gamma, float c_v,
    float rho_min, float p_min, float mu, float k_thermal
) {
    if (!ready(ctx, {rho0, mom0, e0, rho1, mom1, e1, k1_rho, k1_mom, k1_e, rho_out, mom_out, e_out})) return;
    if (!valid_grid(ctx, gx, gy, gz, dx)) return;
    if (dbg_capacity < 0 || dbg_capacity > UINT32_MAX) { ctx->fail("invalid debug capacity"); return; }
    if (dbg_capacity && !ready(ctx, {dbg_head, dbg_words})) return;
    int64_t n = gx * gy * gz;
    GasGridParams prm = {
        (uint32_t)n, (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
        dx, dt, gamma, c_v, rho_min, p_min, mu, k_thermal
    };
    uint32_t cap = (uint32_t)dbg_capacity;
    kernels::gas_rk2_stage2<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(rho0),
        ptr<const float>(mom0),
        ptr<const float>(e0),
        ptr<const float>(rho1),
        ptr<const float>(mom1),
        ptr<const float>(e1),
        ptr<const float>(k1_rho),
        ptr<const float>(k1_mom),
        ptr<const float>(k1_e),
        ptr<float>(rho_out),
        ptr<float>(mom_out),
        ptr<float>(e_out),
        prm,
        ptr<unsigned>(dbg_head),
        ptr<uint>(dbg_words),
        cap
    );
    ctx->launch_status(cudaGetLastError(), "gas_rk2_stage2");
}

// ----------------------------------------------------------------------------
// 5. Spatial Hash Grid Collisions
// ----------------------------------------------------------------------------
void manifold_spatial_hash_assign(
    ManifoldContext* ctx,
    ManifoldBuffer* pos,
    ManifoldBuffer* cell_idx,
    ManifoldBuffer* cell_counts,
    int64_t gx, int64_t gy, int64_t gz,
    float cell_size,
    float min_x, float min_y, float min_z
) {
    if (!ready(ctx, {pos, cell_idx, cell_counts})) return;
    if (!valid_grid(ctx, gx, gy, gz, cell_size)) return;
    int64_t n = pos->size_bytes / (3 * sizeof(float));
    if (n == 0) return;
    SpatialHashParams prm = {
        (uint32_t)n, (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
        cell_size, 1.0f / cell_size, min_x, min_y, min_z
    };
    kernels::spatial_hash_assign<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(pos),
        ptr<uint>(cell_idx),
        ptr<unsigned>(cell_counts),
        prm
    );
    ctx->launch_status(cudaGetLastError(), "spatial_hash_assign");
}

void manifold_spatial_hash_scatter(
    ManifoldContext* ctx,
    ManifoldBuffer* cell_idx,
    ManifoldBuffer* sorted_idx,
    ManifoldBuffer* cell_offsets,
    int64_t num_particles
) {
    if (ctx && (num_particles < 0 || uint64_t(num_particles) > UINT32_MAX)) { ctx->fail("count outside source uint32 range"); return; }
    if (!ready(ctx, {cell_idx, sorted_idx, cell_offsets})) return;
    if (num_particles == 0) return;
    uint32_t np = (uint32_t)num_particles;
    kernels::spatial_hash_scatter<<<blocks(num_particles), kBlockSize, 0, ctx->stream>>>(
        ptr<const uint>(cell_idx),
        ptr<uint>(sorted_idx),
        ptr<unsigned>(cell_offsets),
        np
    );
    ctx->launch_status(cudaGetLastError(), "spatial_hash_scatter");
}

void manifold_spatial_hash_collisions(
    ManifoldContext* ctx,
    ManifoldBuffer* pos,
    ManifoldBuffer* vel,
    ManifoldBuffer* excitation,
    ManifoldBuffer* mass,
    ManifoldBuffer* heat,
    ManifoldBuffer* sorted_idx,
    ManifoldBuffer* cell_starts,
    ManifoldBuffer* cell_idx,
    ManifoldBuffer* vel_in,
    ManifoldBuffer* heat_in,
    int64_t gx, int64_t gy, int64_t gz,
    float cell_size,
    float min_x, float min_y, float min_z,
    float dt, float radius, float young_modulus,
    float thermal_conductivity, float specific_heat, float restitution
) {
    if (!ready(ctx, {pos, vel, excitation, mass, heat, sorted_idx, cell_starts, cell_idx, vel_in, heat_in})) return;
    if (!valid_grid(ctx, gx, gy, gz, cell_size)) return;
    int64_t n = pos->size_bytes / (3 * sizeof(float));
    if (n == 0) return;
    SpatialCollisionParams prm = {
        (uint32_t)n, (uint32_t)gx, (uint32_t)gy, (uint32_t)gz,
        cell_size, 1.0f / cell_size, min_x, min_y, min_z,
        dt, radius, young_modulus, thermal_conductivity, specific_heat, restitution
    };
    kernels::spatial_hash_collisions<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(pos),
        ptr<float>(vel),
        ptr<float>(excitation),
        ptr<const float>(mass),
        ptr<float>(heat),
        ptr<const uint>(sorted_idx),
        ptr<const uint>(cell_starts),
        ptr<const uint>(cell_idx),
        ptr<const float>(vel_in),
        ptr<const float>(heat_in),
        prm
    );
    ctx->launch_status(cudaGetLastError(), "spatial_hash_collisions");
}

void manifold_particle_interactions(
    ManifoldContext* ctx,
    ManifoldBuffer* pos,
    ManifoldBuffer* vel,
    ManifoldBuffer* excitation,
    ManifoldBuffer* mass,
    ManifoldBuffer* heat,
    ManifoldBuffer* vel_in,
    ManifoldBuffer* heat_in,
    float dt, float radius, float young_modulus,
    float thermal_conductivity, float specific_heat, float restitution,
    float domain_x, float domain_y, float domain_z
) {
    if (!ready(ctx, {pos, vel, excitation, mass, heat, vel_in, heat_in})) return;
    int64_t n = pos->size_bytes / (3 * sizeof(float));
    if (n == 0) return;
    ParticleInteractionParams prm = {
        (uint32_t)n, dt, radius, young_modulus,
        thermal_conductivity, specific_heat, restitution,
        domain_x, domain_y, domain_z
    };
    kernels::particle_interactions<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
        ptr<float>(pos),
        ptr<float>(vel),
        ptr<float>(excitation),
        ptr<const float>(mass),
        ptr<float>(heat),
        ptr<const float>(vel_in),
        ptr<const float>(heat_in),
        prm
    );
    ctx->launch_status(cudaGetLastError(), "particle_interactions");
}

// ----------------------------------------------------------------------------
// 6. Generic Parallel Exclusive Scan (u32)
// ----------------------------------------------------------------------------
bool manifold_exclusive_scan_u32(ManifoldContext* ctx,ManifoldBuffer* in,ManifoldBuffer* out,int64_t n) {
    if(!ready(ctx,{in,out})) return false;
    if(n<0 || uint64_t(n)>UINT32_MAX || in->data==out->data ||
       in->size_bytes<uint64_t(n)*4 || out->size_bytes<(uint64_t(n)+1)*4) {
        ctx->fail("exclusive scan requires disjoint n-input and n+1-output buffers");return false;
    }
    uint32_t len=uint32_t(n);ManifoldScanPlan plan;
    if(!plan.make(len)) {ctx->fail("scan plan overflow");return false;}
    auto* work=plan.words ? reinterpret_cast<unsigned*>(ctx->reduction_scratch(plan.words*4)) : nullptr;
    if(plan.words && !work) return false;
    for(unsigned l=0;l<plan.count;l++) {
        const auto& a=plan.level[l];
        auto* src=l ? work+plan.level[l-1].sums : ptr<const unsigned>(in);
        auto* dst=l ? work+plan.level[l-1].prefix : ptr<unsigned>(out);
        kernels::exclusive_scan_u32_pass1<<<a.groups,256,256*sizeof(unsigned),ctx->stream>>>(src,dst,work+a.sums,a.n);
        ctx->launch_status(cudaGetLastError(),"exclusive_scan_u32_pass1");if(ctx->error[0])return false;
    }
    for(int l=int(plan.count)-2;l>=0;--l) {
        const auto& a=plan.level[l];auto* dst=l ? work+plan.level[l-1].prefix : ptr<unsigned>(out);
        kernels::exclusive_scan_u32_add_block_offsets<<<a.groups,256,0,ctx->stream>>>(dst,work+a.prefix,a.n);
        ctx->launch_status(cudaGetLastError(),"exclusive_scan_u32_add_block_offsets");if(ctx->error[0])return false;
    }
    kernels::exclusive_scan_u32_finalize_total<<<1,1,0,ctx->stream>>>(ptr<const unsigned>(in),ptr<unsigned>(out),len);
    ctx->launch_status(cudaGetLastError(),"exclusive_scan_u32_finalize_total");return !ctx->error[0];
}

void manifold_exclusive_scan_u32_pass1(
    ManifoldContext* ctx,
    ManifoldBuffer* in,
    ManifoldBuffer* out,
    ManifoldBuffer* block_sums,
    int64_t n
) {
    if (ctx && (n < 0 || uint64_t(n) > UINT32_MAX)) { ctx->fail("count outside source uint32 range"); return; }
    if (!ready(ctx, {in, out, block_sums})) return;
    if (n <= 0) return;
    size_t num_groups = (n + kBlockSize - 1) / kBlockSize;
    uint32_t nu = (uint32_t)n;
    kernels::exclusive_scan_u32_pass1<<<num_groups, kBlockSize, kBlockSize * sizeof(uint32_t), ctx->stream>>>(
        ptr<const uint>(in),
        ptr<uint>(out),
        ptr<uint>(block_sums),
        nu
    );
    ctx->launch_status(cudaGetLastError(), "exclusive_scan_u32_pass1");
}

void manifold_exclusive_scan_u32_add_block_offsets(
    ManifoldContext* ctx,
    ManifoldBuffer* out,
    ManifoldBuffer* block_prefix,
    int64_t n
) {
    if (ctx && (n < 0 || uint64_t(n) > UINT32_MAX)) { ctx->fail("count outside source uint32 range"); return; }
    if (!ready(ctx, {out, block_prefix})) return;
    if (n <= 0) return;
    size_t num_groups = (n + kBlockSize - 1) / kBlockSize;
    uint32_t nu = (uint32_t)n;
    kernels::exclusive_scan_u32_add_block_offsets<<<num_groups, kBlockSize, 0, ctx->stream>>>(
        ptr<uint>(out),
        ptr<const uint>(block_prefix),
        nu
    );
    ctx->launch_status(cudaGetLastError(), "exclusive_scan_u32_add_block_offsets");
}

void manifold_exclusive_scan_u32_finalize_total(
    ManifoldContext* ctx,
    ManifoldBuffer* in,
    ManifoldBuffer* out,
    int64_t n
) {
    if (ctx && (n < 0 || uint64_t(n) > UINT32_MAX)) { ctx->fail("count outside source uint32 range"); return; }
    if (!ready(ctx, {in, out})) return;
    uint32_t nu = (uint32_t)n;
    kernels::exclusive_scan_u32_finalize_total<<<1, 1, 0, ctx->stream>>>(
        ptr<const uint>(in),
        ptr<uint>(out),
        nu
    );
    ctx->launch_status(cudaGetLastError(), "exclusive_scan_u32_finalize_total");
}

// ----------------------------------------------------------------------------
// 7. Coherence ω-Binning & Lattice Dynamics (GPE)
// ----------------------------------------------------------------------------
void manifold_coherence_reduce_omega_minmax_keys(
    ManifoldContext* ctx,
    ManifoldBuffer* carrier_omega,
    ManifoldBuffer* num_carriers_snapshot,
    ManifoldBuffer* omega_min_key,
    ManifoldBuffer* omega_max_key
) {
    if (!ready(ctx, {carrier_omega, num_carriers_snapshot, omega_min_key, omega_max_key})) return;
    int64_t maxM = carrier_omega->size_bytes / sizeof(float);
    if (maxM == 0) return;
    kernels::coherence_reduce_omega_minmax_keys<<<blocks(maxM), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(carrier_omega),
        ptr<const uint>(num_carriers_snapshot),
        ptr<unsigned>(omega_min_key),
        ptr<unsigned>(omega_max_key)
    );
    ctx->launch_status(cudaGetLastError(), "coherence_reduce_omega_minmax_keys");
}

void manifold_coherence_compute_bin_params(
    ManifoldContext* ctx,
    ManifoldBuffer* omega_min_key,
    ManifoldBuffer* omega_max_key,
    ManifoldBuffer* num_carriers_snapshot,
    ManifoldBuffer* bin_params_out,
    float gate_width_max
) {
    if (!ready(ctx, {omega_min_key, omega_max_key, num_carriers_snapshot, bin_params_out})) return;
    kernels::coherence_compute_bin_params<<<1, 1, 0, ctx->stream>>>(
        ptr<const unsigned>(omega_min_key),
        ptr<const unsigned>(omega_max_key),
        ptr<const uint>(num_carriers_snapshot),
        ptr<CoherenceBinParams>(bin_params_out),
        gate_width_max
    );
    ctx->launch_status(cudaGetLastError(), "coherence_compute_bin_params");
}

void manifold_coherence_bin_count(
    ManifoldContext* ctx,
    ManifoldBuffer* carrier_omega,
    ManifoldBuffer* num_carriers_snapshot,
    ManifoldBuffer* bin_counts,
    ManifoldBuffer* bin_params,
    int64_t num_bins
) {
    if (ctx && (num_bins < 0 || uint64_t(num_bins) > UINT32_MAX)) { ctx->fail("count outside source uint32 range"); return; }
    if (!ready(ctx, {carrier_omega, num_carriers_snapshot, bin_counts, bin_params})) return;
    int64_t maxM = carrier_omega->size_bytes / sizeof(float);
    if (maxM == 0) return;
    uint32_t nb = (uint32_t)num_bins;
    kernels::coherence_bin_count_carriers<<<blocks(maxM), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(carrier_omega),
        ptr<const uint>(num_carriers_snapshot),
        ptr<unsigned>(bin_counts),
        ptr<const CoherenceBinParams>(bin_params),
        nb
    );
    ctx->launch_status(cudaGetLastError(), "coherence_bin_count_carriers");
}

void manifold_coherence_bin_scatter(
    ManifoldContext* ctx,
    ManifoldBuffer* carrier_omega,
    ManifoldBuffer* num_carriers_snapshot,
    ManifoldBuffer* bin_offsets,
    ManifoldBuffer* bin_params,
    int64_t num_bins,
    ManifoldBuffer* carrier_binned_idx
) {
    if (ctx && (num_bins < 0 || uint64_t(num_bins) > UINT32_MAX)) { ctx->fail("count outside source uint32 range"); return; }
    if (!ready(ctx, {carrier_omega, num_carriers_snapshot, bin_offsets, bin_params, carrier_binned_idx})) return;
    int64_t maxM = carrier_omega->size_bytes / sizeof(float);
    if (maxM == 0) return;
    uint32_t nb = (uint32_t)num_bins;
    kernels::coherence_bin_scatter_carriers<<<blocks(maxM), kBlockSize, 0, ctx->stream>>>(
        ptr<const float>(carrier_omega),
        ptr<const uint>(num_carriers_snapshot),
        ptr<unsigned>(bin_offsets),
        ptr<const CoherenceBinParams>(bin_params),
        nb,
        ptr<uint>(carrier_binned_idx)
    );
    ctx->launch_status(cudaGetLastError(), "coherence_bin_scatter_carriers");
}

void manifold_coherence_accumulate_forces(
    ManifoldContext* ctx,
    ManifoldBuffer* osc_phase,
    ManifoldBuffer* osc_omega,
    ManifoldBuffer* osc_amp,
    ManifoldBuffer* particle_pos,
    ManifoldBuffer* carrier_omega,
    ManifoldBuffer* carrier_gate_width,
    ManifoldBuffer* carrier_anchor_idx,
    ManifoldBuffer* carrier_anchor_weight,
    ManifoldBuffer* accums,
    ManifoldBuffer* bin_starts,
    ManifoldBuffer* carrier_binned_idx,
    ManifoldBuffer* bin_params,
    int64_t num_bins,
    ManifoldBuffer* particle_heat,
    int64_t num_osc,
    ManifoldBuffer* num_carriers_snapshot,
    int64_t max_carriers,
    float dt,
    float metabolic_rate,
    float gate_width_min,
    float gate_width_max,
    float offender_weight_floor,
    float domain_x, float domain_y, float domain_z,
    float spatial_sigma
) {
    if(!ready(ctx,{osc_phase,osc_omega,osc_amp,particle_pos,carrier_omega,carrier_gate_width,carrier_anchor_idx,carrier_anchor_weight,accums,bin_starts,carrier_binned_idx,bin_params,particle_heat,num_carriers_snapshot}))return;
    // Bound every uint-indexed stride BEFORE dispatch; capacity is not inferred
    // from the requested count and a malformed snapshot cannot enlarge it.
    if(num_osc<0 || uint64_t(num_osc)>UINT32_MAX/6u || max_carriers<0 || uint64_t(max_carriers)>UINT32_MAX/8u || num_bins<0 || uint64_t(num_bins)>UINT32_MAX ||
       !std::isfinite(dt)||dt<0||!std::isfinite(metabolic_rate)||metabolic_rate<0||
       !std::isfinite(gate_width_min)||gate_width_min<=0||!std::isfinite(gate_width_max)||gate_width_max<gate_width_min||
       !std::isfinite(offender_weight_floor)||offender_weight_floor<0||!std::isfinite(spatial_sigma)||spatial_sigma<0||
       !std::isfinite(domain_x)||domain_x<=0||!std::isfinite(domain_y)||domain_y<=0||!std::isfinite(domain_z)||domain_z<=0) {ctx->fail("invalid coherence dimensions/parameters");return;}
    uint64_t n=uint64_t(num_osc),m=uint64_t(max_carriers);
    if(osc_phase->size_bytes<n*4 || osc_omega->size_bytes<n*4 || osc_amp->size_bytes<n*4 || particle_pos->size_bytes<n*12 || particle_heat->size_bytes<n*4 ||
       carrier_omega->size_bytes<m*4 || carrier_gate_width->size_bytes<m*4 || carrier_anchor_idx->size_bytes<m*32 || carrier_anchor_weight->size_bytes<m*32 ||
       accums->size_bytes<m*32 || num_carriers_snapshot->size_bytes<4) {ctx->fail("undersized coherence buffer");return;}
    ManifoldBuffer* inputs[]={osc_phase,osc_omega,osc_amp,particle_pos,carrier_omega,carrier_gate_width,carrier_anchor_idx,carrier_anchor_weight,num_carriers_snapshot};
    for(auto b:inputs)if(b->data==accums->data||b->data==particle_heat->data){ctx->fail("coherence writable/input alias");return;}
    if(accums->data==particle_heat->data){ctx->fail("coherence writable alias");return;}
    if(num_osc==0 || max_carriers==0)return;
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

    uint32_t nb = (uint32_t)num_bins;
    const size_t shared_bytes = std::min<size_t>(max_carriers, MANIFOLD_LOCAL_CARRIER_CAPACITY) * sizeof(kernels::TGCarrierAccum);
    if (accums->size_bytes < uint64_t(max_carriers)*sizeof(CarrierAccumulators)) {ctx->fail("undersized carrier accumulator buffer");return;}
    kernels::coherence_accumulate_forces<<<blocks(num_osc), kBlockSize, shared_bytes, ctx->stream>>>(
        ptr<const float>(osc_phase),
        ptr<const float>(osc_omega),
        ptr<const float>(osc_amp),
        ptr<const float>(particle_pos),
        ptr<const float>(carrier_omega),
        ptr<const float>(carrier_gate_width),
        ptr<const uint>(carrier_anchor_idx),
        ptr<const float>(carrier_anchor_weight),
        ptr<CarrierAccumulators>(accums),
        prm,
        ptr<const uint>(num_carriers_snapshot),
        ptr<const uint>(bin_starts),
        ptr<const uint>(carrier_binned_idx),
        ptr<const CoherenceBinParams>(bin_params),
        nb,
        ptr<float>(particle_heat)
    );
    ctx->launch_status(cudaGetLastError(), "coherence_accumulate_forces");
}

void manifold_coherence_gpe_step_geometry(
    ManifoldContext* ctx,
    ManifoldBuffer* osc_phase,
    ManifoldBuffer* osc_omega,
    ManifoldBuffer* osc_amp,
    ManifoldBuffer* carrier_real,
    ManifoldBuffer* carrier_imag,
    ManifoldBuffer* carrier_omega,
    ManifoldBuffer* carrier_gate_width,
    ManifoldBuffer* kinetic_real,
    ManifoldBuffer* kinetic_imag,
    ManifoldBuffer* carrier_anchor_idx,
    ManifoldBuffer* carrier_anchor_weight,
    ManifoldBuffer* accums,
    ManifoldBuffer* num_carriers_snapshot,
    ManifoldBuffer* particle_pos,
    SpectralModeParams prm,
    GPEParams gp,
    ManifoldBuffer* extra_potential,
    ManifoldBuffer* metric_volume
) {
    // Preserve the public API but eliminate the four separate device passes.
    (void)osc_phase;(void)osc_omega;(void)osc_amp;(void)carrier_omega;
    (void)carrier_gate_width;(void)carrier_anchor_idx;(void)carrier_anchor_weight;(void)particle_pos;
    if (!ready(ctx,{carrier_real,carrier_imag,kinetic_real,kinetic_imag,accums,num_carriers_snapshot})) return;
    if(gp.metric_coupling!=0) {ctx->fail("legacy scalar metric_coupling cannot specify a metric; supply positive metric_volume");return;}
    if (!prm.max_carriers) return;
    const uint32_t count=mr_workspace_bound(prm.max_carriers);
    const size_t bytes=size_t(count)*sizeof(MRComplex);
    const uint64_t state_bytes=uint64_t(prm.max_carriers)*sizeof(float);
    if (!count || carrier_real->size_bytes<state_bytes || carrier_imag->size_bytes<state_bytes ||
        kinetic_real->size_bytes<state_bytes || kinetic_imag->size_bytes<state_bytes ||
        accums->size_bytes<uint64_t(prm.max_carriers)*sizeof(CarrierAccumulators) || num_carriers_snapshot->size_bytes<4) {
        ctx->fail("invalid FFT capacity or undersized state/work buffers");return;
    }
    uint32_t geometry_flags=(extra_potential?1u:0u)|(metric_volume?2u:0u);
    ManifoldBuffer* optional[]={extra_potential,metric_volume};
    for(unsigned k=0;k<2;++k)if(optional[k]) {
        if(!ready(ctx,{optional[k]}) || optional[k]->size_bytes<state_bytes) {ctx->fail("undersized spectral geometry");return;}
        ManifoldBuffer* writable[]={carrier_real,carrier_imag,kinetic_real,kinetic_imag,accums,num_carriers_snapshot};
        for(auto b:writable)if(b->data==optional[k]->data) {ctx->fail("spectral geometry aliases state/work");return;}
    }
    if(geometry_flags) {
        if(!manifold_synchronize_checked(ctx))return;
        for(unsigned k=0;k<2;++k)if(optional[k]) {
            const float* v=static_cast<const float*>(manifold_get_buffer_pointer(optional[k]));
            if(!v){ctx->fail("spectral geometry readback unavailable");return;}
            for(unsigned j=0;j<prm.max_carriers;++j)if(!std::isfinite(v[j])||(k==1&&!(v[j]>0))) {ctx->fail("invalid scalar potential or nonpositive metric volume");return;}
        }
    }
    ManifoldBuffer* all[]={carrier_real,carrier_imag,kinetic_real,kinetic_imag,accums,num_carriers_snapshot};
    for(unsigned i=0;i<6;i++) for(unsigned j=i+1;j<6;j++) if(all[i]->data==all[j]->data) {
        ctx->fail("FFT state, accumulator and snapshot buffers must not alias");return;
    }
    float* ledger=ctx->ensure_wave_ledger(state_bytes*6);if(!ledger)return;
    ctx->wave_ledger_modes=prm.max_carriers;
    int shared_limit=0;
    if (!ctx->record(cudaDeviceGetAttribute(&shared_limit,cudaDevAttrMaxSharedMemoryPerBlock,ctx->device),"query shared memory")) return;
    cudaFuncAttributes attr{};
    if (bytes<=size_t(shared_limit)) {
        if (!ctx->record(cudaFuncGetAttributes(&attr,kernels::coherence_gpe_fft_fused),"query FFT kernel")) return;
        if(bytes+attr.sharedSizeBytes>size_t(shared_limit)) {ctx->fail("FFT static plus dynamic shared memory exceeds device limit");return;}
        unsigned threads=std::min(256,attr.maxThreadsPerBlock);
        if(!threads) {ctx->fail("FFT kernel supports no threads");return;}
        kernels::coherence_gpe_fft_fused<<<1,threads,bytes,ctx->stream>>>(
            ptr<float>(carrier_real),ptr<float>(carrier_imag),ptr<CarrierAccumulators>(accums),
            ptr<const uint>(num_carriers_snapshot),prm,gp,ptr<float>(kinetic_real),ptr<float>(kinetic_imag),count,ledger,ptr<const float>(extra_potential),ptr<const float>(metric_volume),geometry_flags);
    } else {
        if (!ctx->record(cudaFuncGetAttributes(&attr,kernels::coherence_gpe_fft_fused_global),"query global FFT kernel")) return;
        void* work=ctx->reduction_scratch(bytes);if(!work)return;
        unsigned threads=std::min(256,attr.maxThreadsPerBlock);
        if(!threads) {ctx->fail("FFT kernel supports no threads");return;}
        kernels::coherence_gpe_fft_fused_global<<<1,threads,0,ctx->stream>>>(
            ptr<float>(carrier_real),ptr<float>(carrier_imag),ptr<CarrierAccumulators>(accums),
            ptr<const uint>(num_carriers_snapshot),prm,gp,ptr<float>(kinetic_real),ptr<float>(kinetic_imag),count,work,ledger,ptr<const float>(extra_potential),ptr<const float>(metric_volume),geometry_flags);
    }
    ctx->launch_status(cudaGetLastError(),"coherence_gpe_fft_fused");
}

void manifold_coherence_gpe_step(
    ManifoldContext* ctx,
    ManifoldBuffer* osc_phase,
    ManifoldBuffer* osc_omega,
    ManifoldBuffer* osc_amp,
    ManifoldBuffer* carrier_real,
    ManifoldBuffer* carrier_imag,
    ManifoldBuffer* carrier_omega,
    ManifoldBuffer* carrier_gate_width,
    ManifoldBuffer* kinetic_real,
    ManifoldBuffer* kinetic_imag,
    ManifoldBuffer* carrier_anchor_idx,
    ManifoldBuffer* carrier_anchor_weight,
    ManifoldBuffer* accums,
    ManifoldBuffer* num_carriers_snapshot,
    ManifoldBuffer* particle_pos,
    SpectralModeParams prm,
    GPEParams gp,
    ManifoldBuffer* extra_potential
) {
    manifold_coherence_gpe_step_geometry(ctx,osc_phase,osc_omega,osc_amp,carrier_real,carrier_imag,carrier_omega,carrier_gate_width,kinetic_real,kinetic_imag,carrier_anchor_idx,carrier_anchor_weight,accums,num_carriers_snapshot,particle_pos,prm,gp,extra_potential,nullptr);
}

void manifold_coherence_update_oscillator_phases(
    ManifoldContext* ctx,
    ManifoldBuffer* osc_phase,
    ManifoldBuffer* osc_omega,
    ManifoldBuffer* osc_amp,
    ManifoldBuffer* carrier_real,
    ManifoldBuffer* carrier_imag,
    ManifoldBuffer* carrier_omega,
    ManifoldBuffer* carrier_gate_width,
    ManifoldBuffer* carrier_anchor_idx,
    ManifoldBuffer* carrier_anchor_weight,
    ManifoldBuffer* num_carriers_snapshot,
    SpectralModeParams prm,
    ManifoldBuffer* bin_starts,
    ManifoldBuffer* carrier_binned_idx,
    ManifoldBuffer* bin_params,
    int64_t num_bins,
    ManifoldBuffer* particle_pos
) {
    if(!ready(ctx,{osc_phase,osc_omega,osc_amp,carrier_real,carrier_imag,carrier_omega,carrier_gate_width,carrier_anchor_idx,carrier_anchor_weight,num_carriers_snapshot,bin_starts,carrier_binned_idx,bin_params,particle_pos}))return;
    if(prm.num_osc>UINT32_MAX/6u || prm.max_carriers>UINT32_MAX/8u || num_bins<0 || uint64_t(num_bins)>UINT32_MAX ||
       !std::isfinite(prm.dt)||prm.dt<0||!std::isfinite(prm.coupling_scale)||prm.coupling_scale<0||
       !std::isfinite(prm.spatial_sigma)||prm.spatial_sigma<0||!std::isfinite(prm.domain_x)||prm.domain_x<=0||
       !std::isfinite(prm.domain_y)||prm.domain_y<=0||!std::isfinite(prm.domain_z)||prm.domain_z<=0) {ctx->fail("invalid phase dimensions/parameters");return;}
    uint64_t n=prm.num_osc,m=prm.max_carriers;
    if(osc_phase->size_bytes<n*4||osc_omega->size_bytes<n*4||osc_amp->size_bytes<n*4||particle_pos->size_bytes<n*12||
       carrier_real->size_bytes<m*4||carrier_imag->size_bytes<m*4||carrier_omega->size_bytes<m*4||carrier_gate_width->size_bytes<m*4||
       carrier_anchor_idx->size_bytes<m*32||carrier_anchor_weight->size_bytes<m*32||num_carriers_snapshot->size_bytes<4) {ctx->fail("undersized phase buffer");return;}
    ManifoldBuffer* inputs[]={osc_omega,osc_amp,particle_pos,carrier_real,carrier_imag,carrier_omega,carrier_gate_width,carrier_anchor_idx,carrier_anchor_weight,num_carriers_snapshot};
    for(auto b:inputs)if(b->data==osc_phase->data){ctx->fail("phase writable/input alias");return;}
    if(prm.num_osc==0)return;
    uint32_t nb = (uint32_t)num_bins;
    float* ledger=ctx->ensure_phase_ledger(uint64_t(prm.num_osc)*24);if(!ledger)return;
    ctx->phase_ledger_particles=prm.num_osc;
    kernels::coherence_update_oscillator_phases<<<blocks(prm.num_osc), kBlockSize, 0, ctx->stream>>>(
        ptr<float>(osc_phase),
        ptr<const float>(osc_omega),
        ptr<const float>(osc_amp),
        ptr<const float>(carrier_real),
        ptr<const float>(carrier_imag),
        ptr<const float>(carrier_omega),
        ptr<const float>(carrier_gate_width),
        ptr<const uint>(carrier_anchor_idx),
        ptr<const float>(carrier_anchor_weight),
        ptr<const uint>(num_carriers_snapshot),
        prm,
        ptr<const uint>(bin_starts),
        ptr<const uint>(carrier_binned_idx),
        ptr<const CoherenceBinParams>(bin_params),
        nb,
        ptr<const float>(particle_pos),ledger
    );
    ctx->launch_status(cudaGetLastError(), "coherence_update_oscillator_phases");
}

// ----------------------------------------------------------------------------
// 8. Particle Generation
// ----------------------------------------------------------------------------
void manifold_generate_particles(
    ManifoldContext* ctx,
    ManifoldBuffer* positions,
    ManifoldBuffer* velocities,
    ManifoldBuffer* energies,
    ManifoldBuffer* heats,
    ManifoldBuffer* excitations,
    ManifoldBuffer* masses,
    ManifoldBuffer* random_pos,
    ManifoldBuffer* random_props,
    ParticleGenParams prm
) {
    if (!ready(ctx, {positions, velocities, energies, heats, excitations, masses, random_pos, random_props})) return;
    int64_t n = prm.num_particles;
    if (n == 0) return;

    {
        kernels::generate_particle_positions<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
            ptr<float>(positions),
            ptr<const float>(random_pos),
            prm
        );
        ctx->launch_status(cudaGetLastError(), "generate_particle_positions");
    }

    // CPU Barrier to calculate center position
    if (!ctx->synchronize()) return;

    float* pos_ptr = ptr<float>(positions);
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
        kernels::initialize_particle_properties<<<blocks(n), kBlockSize, 0, ctx->stream>>>(
            ptr<const float>(positions),
            ptr<float>(velocities),
            ptr<float>(energies),
            ptr<float>(heats),
            ptr<float>(excitations),
            ptr<float>(masses),
            ptr<const float>(random_props),
            prm,
            mean_x,
            mean_y,
            mean_z
        );
        ctx->launch_status(cudaGetLastError(), "initialize_particle_properties");
    }
}


} // extern "C"

#include "physics_v2_cuda_host.inc"

#include "coupled_cuda_host.inc"

extern "C" bool manifold_coherence_copy_ledger(ManifoldContext* ctx,ManifoldBuffer* out,unsigned n) {
    if(!ready(ctx,{out}) || !ctx->synchronize())return false;
    const size_t bytes=size_t(n)*6*sizeof(float);
    if(n>ctx->wave_ledger_modes || out->size_bytes<bytes || bytes>ctx->wave_ledger_bytes) {ctx->fail("wave ledger read exceeds completed storage");return false;}
    if(!bytes)return true;
    ctx->launch_status(cudaMemcpyAsync(out->data,ctx->wave_ledger,bytes,cudaMemcpyDeviceToDevice,ctx->stream),"copy wave ledger");
    return ctx->synchronize();
}

extern "C" bool manifold_phase_copy_ledger(ManifoldContext* ctx,ManifoldBuffer* out,unsigned n){
    if(!ready(ctx,{out})||!ctx->synchronize())return false;size_t bytes=size_t(n)*24;
    if(n>ctx->phase_ledger_particles||out->size_bytes<bytes||bytes>ctx->phase_ledger_bytes){ctx->fail("phase ledger bounds");return false;}
    if(!bytes)return true;
    ctx->launch_status(cudaMemcpyAsync(out->data,ctx->phase_ledger,bytes,cudaMemcpyDeviceToDevice,ctx->stream),"copy phase ledger");return ctx->synchronize();
}
