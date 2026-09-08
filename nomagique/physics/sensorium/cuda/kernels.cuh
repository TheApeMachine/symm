#pragma once
#include <cuda_runtime.h>
#include <cstdint>
#include "../bridge.h"

// Only scalar POD structs cross the C/Go boundary. CUDA float3 is private to
// device arithmetic; no Metal/CUDA float3 ABI equivalence is assumed.
namespace sensorium::kernels {
using uint = unsigned;
using CoherenceModeParams = SpectralModeParams;

// Host/kernel POD size guards (all fields are scalar 32-bit words).
static_assert(sizeof(float) == 4 && sizeof(unsigned) == 4);
static_assert(sizeof(::ParticleGenParams) == 52);
static_assert(sizeof(::ParticleInteractionParams) == 40);
static_assert(sizeof(::SpectralModeParams) == 128);
static_assert(sizeof(::GPEParams) == 44);
static_assert(sizeof(::SpatialHashParams) == 36);
static_assert(sizeof(::SpatialCollisionParams) == 60);
static_assert(sizeof(::SortScatterParams) == 28);
static_assert(sizeof(::PicGatherParams) == 64);
static_assert(sizeof(::ModeProjectParams) == 32);
static_assert(sizeof(::PilotWaveParams) == 52);
static_assert(sizeof(::GasGridParams) == 48);

struct SpectralBinParams {
    float omega_min;
    float inv_bin_width;
};
struct CarrierAccumulators {
    float force_r;
    float force_i;
    float w_sum;
    float w_omega_sum;
    float w_omega2_sum;
    float w_amp_sum;
    unsigned offender_score; 
    unsigned offender_idx;   
};
struct TGCarrierAccum {
    unsigned force_r;      
    unsigned force_i;      
    unsigned w_sum;        
    unsigned w_omega_sum;  
    unsigned w_omega2_sum; 
    unsigned w_amp_sum;    
    unsigned offender_score;
    unsigned offender_idx;
};
using CoherenceBinParams = SpectralBinParams;
static_assert(sizeof(CarrierAccumulators) == 32);
static_assert(sizeof(TGCarrierAccum) == 32);
static_assert(sizeof(SpectralBinParams) == 8);

__global__ void reduce_float_stats_pass1(
    const float* x,
    float* group_stats,
    uint  N
);

__global__ void reduce_float_stats_finalize(
    const float* group_stats,
    float* out_stats,
    uint  num_groups
);

__global__ void clear_field(
    float* field,
    uint  num_elements
);

__global__ void particle_interactions(
    float* particle_pos,
    float* particle_vel,
    float* particle_excitation,
    const float* particle_mass,
    float* particle_heat,
    const float* particle_vel_in,
    const float* particle_heat_in,
    ParticleInteractionParams  p
);

__global__ void spatial_hash_assign(
    const float* particle_pos,
    uint* particle_cell_idx,
    unsigned* cell_counts,
    SpatialHashParams  p
);

__global__ void spatial_hash_prefix_sum(
    const uint* cell_counts,
    uint* cell_starts,
    uint  num_cells
);

__global__ void spatial_hash_prefix_sum_parallel(
    uint* cell_counts,
    uint* block_sums,
    uint  num_cells
);

__global__ void exclusive_scan_u32_pass1(
    const uint* in,
    uint* out,
    uint* block_sums,
    uint  n
);

__global__ void exclusive_scan_u32_add_block_offsets(
    uint* out,
    const uint* block_prefix,
    uint  n
);

__global__ void exclusive_scan_u32_finalize_total(
    const uint* in,
    uint* out,
    uint  n
);

__global__ void spatial_hash_scatter(
    const uint* particle_cell_idx,
    uint* sorted_particle_idx,
    unsigned* cell_offsets,
    uint  num_particles
);

__global__ void coherence_reduce_omega_minmax_keys(
    const float* carrier_omega,
    const uint* num_carriers_in,
    unsigned* omega_min_key,
    unsigned* omega_max_key
);

__global__ void coherence_compute_bin_params(
    const unsigned* omega_min_key,
    const unsigned* omega_max_key,
    const uint* num_carriers_in,
    CoherenceBinParams* out_params,
    float  gate_width_max
);

__global__ void coherence_bin_count_carriers(
    const float* carrier_omega,
    const uint* num_carriers_in,
    unsigned* bin_counts,
    const CoherenceBinParams* bin_p,
    uint  num_bins
);

__global__ void coherence_bin_scatter_carriers(
    const float* carrier_omega,
    const uint* num_carriers_in,
    unsigned* bin_offsets,
    const CoherenceBinParams* bin_p,
    uint  num_bins,
    uint* carrier_binned_idx
);

__global__ void spatial_hash_collisions(
    const float* particle_pos,
    float* particle_vel,
    float* particle_excitation,
    const float* particle_mass,
    float* particle_heat,
    const uint* sorted_particle_idx,
    const uint* cell_starts,
    const uint* particle_cell_idx,
    const float* particle_vel_in,
    const float* particle_heat_in,
    SpatialCollisionParams  p
);

__global__ void scatter_compute_cell_idx(
    const float* particle_pos,
    uint* particle_cell_idx,
    SortScatterParams  p
);

__global__ void scatter_count_cells(
    const uint* particle_cell_idx,
    unsigned* cell_counts,
    SortScatterParams  p
);

__global__ void scatter_prefix_sum_upsweep(
    uint* data,
    uint  stride,
    uint  n
);

__global__ void scatter_prefix_sum_downsweep(
    uint* data,
    uint  stride,
    uint  n
);

__global__ void scatter_reorder_particles(
    const float* particle_pos_in,
    const float* particle_vel_in,
    const float* particle_mass_in,
    const float* particle_heat_in,
    const float* particle_energy_in,
    const uint* particle_cell_idx,
    const uint* cell_starts,
    unsigned* cell_offsets,
    float* particle_pos_out,
    float* particle_vel_out,
    float* particle_mass_out,
    float* particle_heat_out,
    float* particle_energy_out,
    uint* sorted_original_idx,
    SortScatterParams  p
);

__global__ void scatter_sorted(
    const float* particle_pos,
    const float* particle_vel,
    const float* particle_mass,
    const float* particle_heat,
    const float* particle_energy,
    unsigned* rho_field,
    unsigned* mom_field,
    unsigned* E_field,
    SortScatterParams  p
);

__global__ void gas_rk2_stage1(
    const float* rho0,
    const float* mom0,
    const float* e0,
    float* rho1,
    float* mom1,
    float* e1,
    float* k1_rho,
    float* k1_mom,
    float* k1_e,
    GasGridParams  p,
    unsigned* dbg_head,
    uint* dbg_words,
    uint  dbg_cap
);

__global__ void gas_rk2_stage2(
    const float* rho0,
    const float* mom0,
    const float* e0,
    const float* rho1,
    const float* mom1,
    const float* e1,
    const float* k1_rho,
    const float* k1_mom,
    const float* k1_e,
    float* rho_out,
    float* mom_out,
    float* e_out,
    GasGridParams  p,
    unsigned* dbg_head,
    uint* dbg_words,
    uint  dbg_cap
);

__global__ void pic_gather_update_particles(
    const float* particle_pos_in,
    const float* particle_mass,
    float* particle_pos_out,
    float* particle_vel_out,
    float* particle_heat_out,
    const float* rho_field,
    const float* mom_field,
    const float* E_field,
    const float* gravity_potential,
    PicGatherParams  p,
    unsigned* dbg_head,
    uint* dbg_words,
    uint  dbg_cap
);

__global__ void project_modes_to_spatial_psi(
    const float* mode_psi_real,
    const float* mode_psi_imag,
    const uint*  mode_anchor_idx,
    const float* mode_anchor_weight,
    const float* particle_pos,
    unsigned* psi_re_field,
    unsigned* psi_im_field,
    ModeProjectParams  p
);

__global__ void pic_gather_update_particles_pilot_wave(
    const float* particle_pos_in,
    const float* particle_mass,
    float*       particle_pos_out,
    float*       particle_vel_out,
    const float* psi_re_field,
    const float* psi_im_field,
    PilotWaveParams  p
);

__global__ void coherence_accumulate_forces(
    const float* osc_phase,
    const float* osc_omega,
    const float* osc_amp,
    const float* particle_pos,
    const float* carrier_omega,
    const float* carrier_gate_width,
    const uint* carrier_anchor_idx,
    const float* carrier_anchor_w,
    CarrierAccumulators* accums,
    CoherenceModeParams  p,
    const uint* num_carriers_in,
    const uint* bin_starts,
    const uint* carrier_binned_idx,
    const CoherenceBinParams* bin_p,
    uint  num_bins,
    float* particle_heat
);

__global__ void coherence_gpe_step(
    const float* osc_phase,
    const float* osc_omega,
    const float* osc_amp,
    float* mode_real,
    float* mode_imag,
    const float* mode_omega,
    const float* mode_gate_width,
    uint* mode_anchor_idx,
    float* mode_anchor_weight,
    CarrierAccumulators* accums,
    const uint* num_modes_in,
    const float* particle_pos,
    CoherenceModeParams  p,
    GPEParams  gp
);

__global__ void coherence_gpe_kinetic_dft(
    const float* mode_real,
    const float* mode_imag,
    float* transformed_real,
    float* transformed_imag,
    const uint* num_modes_in,
    uint  max_modes,
    GPEParams  gp
);

__global__ void coherence_gpe_kinetic_idft(
    const float* transformed_real,
    const float* transformed_imag,
    float* mode_real,
    float* mode_imag,
    const uint* num_modes_in,
    uint  max_modes
);

__global__ void coherence_gpe_finish(
    float* mode_real,
    float* mode_imag,
    CarrierAccumulators* accums,
    const uint* num_modes_in,
    CoherenceModeParams  p,
    GPEParams  gp
);

__global__ void coherence_update_oscillator_phases(
    float* particle_phase,
    const float* particle_omega,
    const float* particle_amp,
    const float* mode_real,
    const float* mode_imag,
    const float* mode_omega,
    const float* mode_gate_width,
    const uint* mode_anchor_idx,
    const float* mode_anchor_weight,
    const uint* num_carriers_in,
    CoherenceModeParams  p,
    const uint* bin_starts,
    const uint* carrier_binned_idx,
    const CoherenceBinParams* bin_p,
    uint  num_bins,
    const float* particle_pos
);

__global__ void generate_particle_positions(
    float* positions,
    const float* random_vals,
    ParticleGenParams  p
);

__global__ void initialize_particle_properties(
    const float* positions,
    float* velocities,
    float* energies,
    float* heats,
    float* excitations,
    float* masses,
    const float* random_vals,
    ParticleGenParams  p,
    float  center_x,
    float  center_y,
    float  center_z
);

} // namespace sensorium::kernels
