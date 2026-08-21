#ifndef MANIFOLD_BRIDGE_H
#define MANIFOLD_BRIDGE_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C"
{
#endif

    // ----------------------------------------------------------------------------
    // Opaque Handles
    // ----------------------------------------------------------------------------
    typedef struct ManifoldContext ManifoldContext;
    typedef struct ManifoldBuffer ManifoldBuffer;

    // ----------------------------------------------------------------------------
    // Parameter Structs (Exact binary layout matching Metal shaders)
    // ----------------------------------------------------------------------------
    typedef struct
    {
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
    } ParticleGenParams;

    typedef struct
    {
        uint32_t num_particles;
        float dt;
        float particle_radius;
        float young_modulus;
        float thermal_conductivity;
        float specific_heat;
        float restitution;
    } ParticleInteractionParams;

    typedef struct
    {
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
    } SpectralModeParams;

    typedef struct
    {
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
    } GPEParams;

    typedef struct
    {
        uint32_t num_particles;
        uint32_t grid_x;
        uint32_t grid_y;
        uint32_t grid_z;
        float cell_size;
        float inv_cell_size;
        float domain_min_x;
        float domain_min_y;
        float domain_min_z;
    } SpatialHashParams;

    typedef struct
    {
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
    } SpatialCollisionParams;

    typedef struct
    {
        uint32_t num_particles;
        uint32_t num_cells;
        uint32_t grid_x;
        uint32_t grid_y;
        uint32_t grid_z;
        float grid_spacing;
        float inv_grid_spacing;
    } SortScatterParams;

    typedef struct
    {
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
        float gravity_enabled;
    } PicGatherParams;

    typedef struct
    {
        uint32_t num_modes;
        uint32_t num_particles;
        uint32_t anchors_per_mode;
        uint32_t grid_x;
        uint32_t grid_y;
        uint32_t grid_z;
        float grid_spacing;
        float inv_grid_spacing;
    } ModeProjectParams;

    typedef struct
    {
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
    } PilotWaveParams;

    typedef struct
    {
        uint32_t num_cells;
        uint32_t grid_x;
        uint32_t grid_y;
        uint32_t grid_z;
        float dx;
        float dt;
        float gamma;
        float c_v;
        float rho_min;
        float p_min;
        float mu;
        float k_thermal;
    } GasGridParams;

    // ----------------------------------------------------------------------------
    // Lifecycle & Context Management
    // ----------------------------------------------------------------------------
    ManifoldContext *manifold_create_context(const char *metallib_path);
    void manifold_destroy_context(ManifoldContext *ctx);
    void manifold_synchronize(ManifoldContext *ctx);

    // ----------------------------------------------------------------------------
    // Unified Memory Buffer Management
    // ----------------------------------------------------------------------------
    ManifoldBuffer *manifold_create_buffer(ManifoldContext *ctx, uint64_t bytes, const void *initial_data);
    void manifold_destroy_buffer(ManifoldBuffer *buf);
    void *manifold_get_buffer_pointer(ManifoldBuffer *buf);
    uint64_t manifold_get_buffer_size(ManifoldBuffer *buf);

    // ----------------------------------------------------------------------------
    // 1. Diagnostics & Memory Kernels
    // ----------------------------------------------------------------------------
    void manifold_clear_field(ManifoldContext *ctx, ManifoldBuffer *field);
    void manifold_thermo_reduce_energy_stats(ManifoldContext *ctx, ManifoldBuffer *x, ManifoldBuffer *out_stats);

    // ----------------------------------------------------------------------------
    // 2. PIC & Sort-Based Scatter Kernels
    // ----------------------------------------------------------------------------
    void manifold_scatter_compute_cell_idx(
        ManifoldContext *ctx,
        ManifoldBuffer *particle_pos,
        ManifoldBuffer *particle_cell_idx,
        int64_t gx, int64_t gy, int64_t gz,
        float grid_spacing);

    void manifold_scatter_count_cells(
        ManifoldContext *ctx,
        ManifoldBuffer *particle_cell_idx,
        ManifoldBuffer *cell_counts,
        int64_t gx, int64_t gy, int64_t gz,
        float grid_spacing);

    void manifold_scatter_reorder_particles(
        ManifoldContext *ctx,
        ManifoldBuffer *pos_in,
        ManifoldBuffer *vel_in,
        ManifoldBuffer *mass_in,
        ManifoldBuffer *heat_in,
        ManifoldBuffer *energy_in,
        ManifoldBuffer *particle_cell_idx,
        ManifoldBuffer *cell_starts,
        ManifoldBuffer *cell_offsets,
        ManifoldBuffer *pos_out,
        ManifoldBuffer *vel_out,
        ManifoldBuffer *mass_out,
        ManifoldBuffer *heat_out,
        ManifoldBuffer *energy_out,
        ManifoldBuffer *sorted_original_idx,
        int64_t gx, int64_t gy, int64_t gz,
        float grid_spacing);

    void manifold_scatter_sorted(
        ManifoldContext *ctx,
        ManifoldBuffer *pos,
        ManifoldBuffer *vel,
        ManifoldBuffer *mass,
        ManifoldBuffer *heat,
        ManifoldBuffer *energy,
        ManifoldBuffer *rho_field,
        ManifoldBuffer *mom_field,
        ManifoldBuffer *E_field,
        int64_t gx, int64_t gy, int64_t gz,
        float grid_spacing);

    void manifold_pic_gather_update_particles(
        ManifoldContext *ctx,
        ManifoldBuffer *pos_in,
        ManifoldBuffer *mass,
        ManifoldBuffer *pos_out,
        ManifoldBuffer *vel_out,
        ManifoldBuffer *heat_out,
        ManifoldBuffer *rho_field,
        ManifoldBuffer *mom_field,
        ManifoldBuffer *E_field,
        ManifoldBuffer *gravity_potential,
        ManifoldBuffer *dbg_head,
        ManifoldBuffer *dbg_words,
        int64_t dbg_capacity,
        int64_t gx, int64_t gy, int64_t gz,
        float grid_spacing,
        float dt,
        float domain_x, float domain_y, float domain_z,
        float gamma, float R_specific, float c_v,
        float rho_min, float p_min,
        float gravity_enabled);

    // ----------------------------------------------------------------------------
    // 3. Quantum Flow & Waves
    // ----------------------------------------------------------------------------
    void manifold_project_modes_to_spatial_psi(
        ManifoldContext *ctx,
        ManifoldBuffer *mode_psi_real,
        ManifoldBuffer *mode_psi_imag,
        ManifoldBuffer *mode_anchor_idx,
        ManifoldBuffer *mode_anchor_weight,
        ManifoldBuffer *particle_pos,
        ManifoldBuffer *psi_re_field,
        ManifoldBuffer *psi_im_field,
        int64_t anchors_per_mode,
        int64_t gx, int64_t gy, int64_t gz,
        float grid_spacing);

    void manifold_pic_gather_pilot_wave(
        ManifoldContext *ctx,
        ManifoldBuffer *pos_in,
        ManifoldBuffer *mass,
        ManifoldBuffer *pos_out,
        ManifoldBuffer *vel_out,
        ManifoldBuffer *psi_re,
        ManifoldBuffer *psi_im,
        int64_t num_particles,
        int64_t gx, int64_t gy, int64_t gz,
        float grid_spacing,
        float dt,
        float domain_x, float domain_y, float domain_z,
        float hbar_eff, float eps_denom, float mass_min);

    // ----------------------------------------------------------------------------
    // 4. Gas Dynamics (Eulerian RK2)
    // ----------------------------------------------------------------------------
    void manifold_gas_rk2_stage1(
        ManifoldContext *ctx,
        ManifoldBuffer *rho0, ManifoldBuffer *mom0, ManifoldBuffer *e0,
        ManifoldBuffer *rho1, ManifoldBuffer *mom1, ManifoldBuffer *e1,
        ManifoldBuffer *k1_rho, ManifoldBuffer *k1_mom, ManifoldBuffer *k1_e,
        ManifoldBuffer *dbg_head, ManifoldBuffer *dbg_words,
        int64_t dbg_capacity,
        int64_t gx, int64_t gy, int64_t gz,
        float dx, float dt, float gamma, float c_v,
        float rho_min, float p_min, float mu, float k_thermal);

    void manifold_gas_rk2_stage2(
        ManifoldContext *ctx,
        ManifoldBuffer *rho0, ManifoldBuffer *mom0, ManifoldBuffer *e0,
        ManifoldBuffer *rho1, ManifoldBuffer *mom1, ManifoldBuffer *e1,
        ManifoldBuffer *k1_rho, ManifoldBuffer *k1_mom, ManifoldBuffer *k1_e,
        ManifoldBuffer *rho_out, ManifoldBuffer *mom_out, ManifoldBuffer *e_out,
        ManifoldBuffer *dbg_head, ManifoldBuffer *dbg_words,
        int64_t dbg_capacity,
        int64_t gx, int64_t gy, int64_t gz,
        float dx, float dt, float gamma, float c_v,
        float rho_min, float p_min, float mu, float k_thermal);

    // ----------------------------------------------------------------------------
    // 5. Spatial Hash Grid Collisions
    // ----------------------------------------------------------------------------
    void manifold_spatial_hash_assign(
        ManifoldContext *ctx,
        ManifoldBuffer *pos,
        ManifoldBuffer *cell_idx,
        ManifoldBuffer *cell_counts,
        int64_t gx, int64_t gy, int64_t gz,
        float cell_size,
        float min_x, float min_y, float min_z);

    void manifold_spatial_hash_scatter(
        ManifoldContext *ctx,
        ManifoldBuffer *cell_idx,
        ManifoldBuffer *sorted_idx,
        ManifoldBuffer *cell_offsets,
        int64_t num_particles);

    void manifold_spatial_hash_collisions(
        ManifoldContext *ctx,
        ManifoldBuffer *pos,
        ManifoldBuffer *vel,
        ManifoldBuffer *excitation,
        ManifoldBuffer *mass,
        ManifoldBuffer *heat,
        ManifoldBuffer *sorted_idx,
        ManifoldBuffer *cell_starts,
        ManifoldBuffer *cell_idx,
        ManifoldBuffer *vel_in,
        ManifoldBuffer *heat_in,
        int64_t gx, int64_t gy, int64_t gz,
        float cell_size,
        float min_x, float min_y, float min_z,
        float dt, float radius, float young_modulus,
        float thermal_conductivity, float specific_heat, float restitution);

    void manifold_particle_interactions(
        ManifoldContext *ctx,
        ManifoldBuffer *pos,
        ManifoldBuffer *vel,
        ManifoldBuffer *excitation,
        ManifoldBuffer *mass,
        ManifoldBuffer *heat,
        ManifoldBuffer *vel_in,
        ManifoldBuffer *heat_in,
        float dt, float radius, float young_modulus,
        float thermal_conductivity, float specific_heat, float restitution);

    // ----------------------------------------------------------------------------
    // 6. Generic Parallel Exclusive Scan (u32)
    // ----------------------------------------------------------------------------
    void manifold_exclusive_scan_u32_pass1(
        ManifoldContext *ctx,
        ManifoldBuffer *in,
        ManifoldBuffer *out,
        ManifoldBuffer *block_sums,
        int64_t n);

    void manifold_exclusive_scan_u32_add_block_offsets(
        ManifoldContext *ctx,
        ManifoldBuffer *out,
        ManifoldBuffer *block_prefix,
        int64_t n);

    void manifold_exclusive_scan_u32_finalize_total(
        ManifoldContext *ctx,
        ManifoldBuffer *in,
        ManifoldBuffer *out,
        int64_t n);

    // ----------------------------------------------------------------------------
    // 7. Coherence ω-Binning & Lattice Dynamics (GPE)
    // ----------------------------------------------------------------------------
    void manifold_coherence_reduce_omega_minmax_keys(
        ManifoldContext *ctx,
        ManifoldBuffer *carrier_omega,
        ManifoldBuffer *num_carriers_snapshot,
        ManifoldBuffer *omega_min_key,
        ManifoldBuffer *omega_max_key);

    void manifold_coherence_compute_bin_params(
        ManifoldContext *ctx,
        ManifoldBuffer *omega_min_key,
        ManifoldBuffer *omega_max_key,
        ManifoldBuffer *num_carriers_snapshot,
        ManifoldBuffer *bin_params_out,
        float gate_width_max);

    void manifold_coherence_bin_count(
        ManifoldContext *ctx,
        ManifoldBuffer *carrier_omega,
        ManifoldBuffer *num_carriers_snapshot,
        ManifoldBuffer *bin_counts,
        ManifoldBuffer *bin_params,
        int64_t num_bins);

    void manifold_coherence_bin_scatter(
        ManifoldContext *ctx,
        ManifoldBuffer *carrier_omega,
        ManifoldBuffer *num_carriers_snapshot,
        ManifoldBuffer *bin_offsets,
        ManifoldBuffer *bin_params,
        int64_t num_bins,
        ManifoldBuffer *carrier_binned_idx);

    void manifold_coherence_accumulate_forces(
        ManifoldContext *ctx,
        ManifoldBuffer *osc_phase,
        ManifoldBuffer *osc_omega,
        ManifoldBuffer *osc_amp,
        ManifoldBuffer *particle_pos,
        ManifoldBuffer *carrier_omega,
        ManifoldBuffer *carrier_gate_width,
        ManifoldBuffer *carrier_anchor_idx,
        ManifoldBuffer *carrier_anchor_weight,
        ManifoldBuffer *accums,
        ManifoldBuffer *bin_starts,
        ManifoldBuffer *carrier_binned_idx,
        ManifoldBuffer *bin_params,
        int64_t num_bins,
        ManifoldBuffer *particle_heat,
        int64_t num_osc,
        ManifoldBuffer *num_carriers_snapshot,
        int64_t max_carriers,
        float dt,
        float metabolic_rate,
        float gate_width_min,
        float gate_width_max,
        float offender_weight_floor,
        float domain_x, float domain_y, float domain_z,
        float spatial_sigma);

    void manifold_coherence_gpe_step(
        ManifoldContext *ctx,
        ManifoldBuffer *osc_phase,
        ManifoldBuffer *osc_omega,
        ManifoldBuffer *osc_amp,
        ManifoldBuffer *carrier_real,
        ManifoldBuffer *carrier_imag,
        ManifoldBuffer *carrier_omega,
        ManifoldBuffer *carrier_gate_width,
        ManifoldBuffer *carrier_anchor_idx,
        ManifoldBuffer *carrier_anchor_weight,
        ManifoldBuffer *accums,
        ManifoldBuffer *num_carriers_snapshot,
        ManifoldBuffer *particle_pos,
        SpectralModeParams prm,
        GPEParams gp,
        ManifoldBuffer *extra_potential);

    void manifold_coherence_update_oscillator_phases(
        ManifoldContext *ctx,
        ManifoldBuffer *osc_phase,
        ManifoldBuffer *osc_omega,
        ManifoldBuffer *osc_amp,
        ManifoldBuffer *carrier_real,
        ManifoldBuffer *carrier_imag,
        ManifoldBuffer *carrier_omega,
        ManifoldBuffer *carrier_gate_width,
        ManifoldBuffer *carrier_anchor_idx,
        ManifoldBuffer *carrier_anchor_weight,
        ManifoldBuffer *num_carriers_snapshot,
        SpectralModeParams prm,
        ManifoldBuffer *bin_starts,
        ManifoldBuffer *carrier_binned_idx,
        ManifoldBuffer *bin_params,
        int64_t num_bins,
        ManifoldBuffer *particle_pos);

    // ----------------------------------------------------------------------------
    // 8. Particle Generation
    // ----------------------------------------------------------------------------
    void manifold_generate_particles(
        ManifoldContext *ctx,
        ManifoldBuffer *positions,
        ManifoldBuffer *velocities,
        ManifoldBuffer *energies,
        ManifoldBuffer *heats,
        ManifoldBuffer *excitations,
        ManifoldBuffer *masses,
        ManifoldBuffer *random_pos,
        ManifoldBuffer *random_props,
        ParticleGenParams prm);

#ifdef __cplusplus
}
#endif

#endif // MANIFOLD_BRIDGE_H