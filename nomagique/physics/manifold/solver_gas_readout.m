#import "solver_private.h"

float manifold_pressure_at(
    float *eData,
    float gamma,
    uint32_t x,
    uint32_t y,
    uint32_t z,
    uint32_t gx,
    uint32_t gy,
    uint32_t gz
) {
    uint32_t index = manifold_cell_index(x, y, z, gx, gy, gz);
    return (gamma - 1.0f) * eData[index];
}

void manifold_velocity_at(
    float *momRhoData,
    uint32_t x,
    uint32_t y,
    uint32_t z,
    uint32_t gx,
    uint32_t gy,
    uint32_t gz,
    float *ux,
    float *uy,
    float *uz
) {
    uint32_t index = manifold_cell_index(x, y, z, gx, gy, gz);
    uint32_t base = index * 4;
    float rho = momRhoData[base + 3];

    if (!(rho > 0.0f)) {
        *ux = 0.0f;
        *uy = 0.0f;
        *uz = 0.0f;
        return;
    }

    *ux = momRhoData[base + 0] / rho;
    *uy = momRhoData[base + 1] / rho;
    *uz = momRhoData[base + 2] / rho;
}

uint32_t manifold_gas_boundary_mode(
    const ManifoldConfig *config,
    uint32_t axis,
    bool high_face
) {
    if (config == NULL) {
        return 0;
    }

    if (axis == 0u) {
        return high_face ? config->boundary_x_high : config->boundary_x_low;
    }

    if (axis == 1u) {
        return high_face ? config->boundary_y_high : config->boundary_y_low;
    }

    return high_face ? config->boundary_z_high : config->boundary_z_low;
}

ManifoldGasNeighborCoord manifold_gas_neighbor_coord(
    uint32_t x,
    uint32_t y,
    uint32_t z,
    uint32_t axis,
    bool high_face,
    uint32_t gx,
    uint32_t gy,
    uint32_t gz,
    uint32_t boundary
) {
    ManifoldGasNeighborCoord neighbor = {.x = x, .y = y, .z = z, .is_ghost = false};
    uint32_t coordinate = axis == 0u ? x : (axis == 1u ? y : z);
    uint32_t extent = axis == 0u ? gx : (axis == 1u ? gy : gz);
    bool edge = high_face ? (coordinate + 1u >= extent) : (coordinate == 0u);

    neighbor.is_ghost = edge && boundary != 0u;

    if (neighbor.is_ghost) {
        return neighbor;
    }

    if (high_face) {
        coordinate = (coordinate + 1u) % extent;
    } else {
        coordinate = (coordinate == 0u) ? (extent - 1u) : (coordinate - 1u);
    }

    if (axis == 0u) {
        neighbor.x = coordinate;
    } else if (axis == 1u) {
        neighbor.y = coordinate;
    } else {
        neighbor.z = coordinate;
    }

    return neighbor;
}

float manifold_gas_pressure_at(
    float *eData,
    float gamma,
    uint32_t x,
    uint32_t y,
    uint32_t z,
    uint32_t gx,
    uint32_t gy,
    uint32_t gz,
    bool is_ghost,
    uint32_t ghost_axis,
    uint32_t ghost_boundary,
    uint32_t cx,
    uint32_t cy,
    uint32_t cz
) {
    (void)ghost_axis;
    (void)ghost_boundary;
    (void)cx;
    (void)cy;
    (void)cz;

    if (!is_ghost) {
        return manifold_pressure_at(eData, gamma, x, y, z, gx, gy, gz);
    }

    uint32_t face_index = manifold_cell_index(x, y, z, gx, gy, gz);
    return (gamma - 1.0f) * eData[face_index];
}

void manifold_gas_velocity_at(
    float *momRhoData,
    uint32_t x,
    uint32_t y,
    uint32_t z,
    uint32_t gx,
    uint32_t gy,
    uint32_t gz,
    bool is_ghost,
    uint32_t ghost_axis,
    uint32_t ghost_boundary,
    uint32_t cx,
    uint32_t cy,
    uint32_t cz,
    float *ux,
    float *uy,
    float *uz
) {
    if (!is_ghost) {
        manifold_velocity_at(momRhoData, x, y, z, gx, gy, gz, ux, uy, uz);
        return;
    }

    (void)cx;
    (void)cy;
    (void)cz;

    uint32_t face_index = manifold_cell_index(x, y, z, gx, gy, gz);
    uint32_t base = face_index * 4;
    float rho = momRhoData[base + 3];

    if (!(rho > 0.0f)) {
        *ux = 0.0f;
        *uy = 0.0f;
        *uz = 0.0f;
        return;
    }

    float momX = momRhoData[base + 0];
    float momY = momRhoData[base + 1];
    float momZ = momRhoData[base + 2];

    if (ghost_boundary == 2u) {
        if (ghost_axis == 0u) {
            momX = -momX;
        } else if (ghost_axis == 1u) {
            momY = -momY;
        } else {
            momZ = -momZ;
        }
    }

    *ux = momX / rho;
    *uy = momY / rho;
    *uz = momZ / rho;
}
