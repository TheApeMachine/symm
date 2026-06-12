#pragma once

#include <stdint.h>

typedef struct ManifoldConfig {
    uint32_t grid_x;
    uint32_t grid_y;
    uint32_t grid_z;
    float domain_x;
    float domain_y;
    float domain_z;
    float dt;
    float gamma;
    float c_v;
    float rho_min;
    float p_min;
    float gas_envelope_rho_min;
    float gas_p_min;
    float k_thermal;
    uint32_t max_carriers;
    float hbar_eff;
    float g_interaction;
    float energy_decay;
    float metabolic_rate;
    float coupling_scale;
    float gate_width_min;
    float gate_width_max;
} ManifoldConfig;

typedef struct ManifoldReading {
    float pressure_grad_x;
    float pressure_grad_y;
    float pressure_grad_z;
    float pressure_grad_norm;
    float divergence;
    float coherence_mag2;
    float guidance_speed;
    float viscosity_proxy;
} ManifoldReading;

typedef struct ManifoldOscillator {
    float phase;
    float omega;
    float amplitude;
    float pos_x;
    float pos_y;
    float pos_z;
    float heat;
    float vel_x;
    float vel_y;
    float vel_z;
} ManifoldOscillator;

void *manifold_solver_create(const ManifoldConfig *config, const void *metallib_bytes, size_t metallib_length, char *err_out, int err_cap);
void manifold_solver_destroy(void *handle);

int manifold_solver_reset_deposits(void *handle, char *err_out, int err_cap);
int manifold_solver_deposit_cell(
    void *handle,
    uint32_t cell_x,
    uint32_t cell_y,
    uint32_t cell_z,
    float rho,
    float mom_x,
    float mom_y,
    float mom_z,
    float e_int,
    char *err_out,
    int err_cap
);

int manifold_solver_set_oscillators(
    void *handle,
    const ManifoldOscillator *oscillators,
    uint32_t count,
    char *err_out,
    int err_cap
);

int manifold_solver_step(void *handle, ManifoldReading *reading, char *err_out, int err_cap);

int manifold_solver_read_rho_projection(
    void *handle,
    float *out,
    uint32_t out_length,
    uint32_t *grid_x,
    uint32_t *grid_z,
    char *err_out,
    int err_cap
);

int manifold_solver_read_projection_reading(
    void *handle,
    ManifoldReading *reading,
    char *err_out,
    int err_cap
);

int manifold_solver_read_oscillators(
    void *handle,
    ManifoldOscillator *out,
    uint32_t count,
    char *err_out,
    int err_cap
);
