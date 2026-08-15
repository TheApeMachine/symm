#import "solver_private.h"

@implementation ManifoldSolver (PicPrivate)

- (float)gridSpacing {
    float cellVolume = (self.config.domain_x / (float)self.config.grid_x) *
        (self.config.domain_y / (float)self.config.grid_y) *
        (self.config.domain_z / (float)self.config.grid_z);

    return powf(cellVolume, 1.0f / 3.0f);
}

- (void)initializeParticleStateFromOscillators:(const ManifoldOscillator *)oscillators count:(uint32_t)count {
    float *velData = (float *)self.particleVel.contents;
    float *massData = (float *)self.particleMass.contents;
    float *energyData = (float *)self.particleEnergy.contents;

    for (uint32_t index = 0; index < count; index++) {
        const ManifoldOscillator *oscillator = &oscillators[index];
        massData[index] = oscillator->amplitude;
        energyData[index] = oscillator->heat;
        velData[index * 3 + 0] = oscillator->vel_x;
        velData[index * 3 + 1] = oscillator->vel_y;
        velData[index * 3 + 2] = oscillator->vel_z;
    }
}

- (void)configureSortScatterParams {
    SortScatterParamsHost *params = (SortScatterParamsHost *)self.sortScatterParams.contents;
    float cellX = self.config.domain_x / (float)self.config.grid_x;
    float cellY = self.config.domain_y / (float)self.config.grid_y;
    float cellZ = self.config.domain_z / (float)self.config.grid_z;

    params->num_particles = self.numOsc;
    params->num_cells = self.numCells;
    params->grid_x = self.config.grid_x;
    params->grid_y = self.config.grid_y;
    params->grid_z = self.config.grid_z;
    params->cell_x = cellX;
    params->cell_y = cellY;
    params->cell_z = cellZ;
    params->inv_cell_x = 1.0f / cellX;
    params->inv_cell_y = 1.0f / cellY;
    params->inv_cell_z = 1.0f / cellZ;
    params->boundary_x_low = self.config.boundary_x_low;
    params->boundary_x_high = self.config.boundary_x_high;
    params->boundary_y_low = self.config.boundary_y_low;
    params->boundary_y_high = self.config.boundary_y_high;
    params->boundary_z_low = self.config.boundary_z_low;
    params->boundary_z_high = self.config.boundary_z_high;
}

- (void)configurePicGatherParams {
    PicGatherParamsHost *params = (PicGatherParamsHost *)self.picGatherParams.contents;
    float cellX = self.config.domain_x / (float)self.config.grid_x;
    float cellY = self.config.domain_y / (float)self.config.grid_y;
    float cellZ = self.config.domain_z / (float)self.config.grid_z;

    params->num_particles = self.numOsc;
    params->grid_x = self.config.grid_x;
    params->grid_y = self.config.grid_y;
    params->grid_z = self.config.grid_z;
    params->cell_x = cellX;
    params->cell_y = cellY;
    params->cell_z = cellZ;
    params->inv_cell_x = 1.0f / cellX;
    params->inv_cell_y = 1.0f / cellY;
    params->inv_cell_z = 1.0f / cellZ;
    params->dt = self.controls.dt;
    params->domain_x = self.config.domain_x;
    params->domain_y = self.config.domain_y;
    params->domain_z = self.config.domain_z;
    params->gamma = self.config.gamma;
    params->R_specific = (self.config.gamma - 1.0f) * self.config.c_v;
    params->c_v = self.config.c_v;
    params->rho_min = self.config.rho_min;
    params->p_min = self.config.p_min;
    params->gravity_enabled = self.gravityReady ? 1.0f : 0.0f;
    params->boundary_x_low = self.config.boundary_x_low;
    params->boundary_x_high = self.config.boundary_x_high;
    params->boundary_y_low = self.config.boundary_y_low;
    params->boundary_y_high = self.config.boundary_y_high;
    params->boundary_z_low = self.config.boundary_z_low;
    params->boundary_z_high = self.config.boundary_z_high;
}

- (BOOL)runScatterPrefixSum:(NSString **)error {
    return [self runExclusiveScanU32:self.scatterCellCounts
                              output:self.scatterCellStarts
                              length:self.numCells
                      writeTotalSlot:NO
                               error:error];
}

- (BOOL)runPicScatter:(NSString **)error {
    (void)error;

    if (self.numOsc == 0) {
        return YES;
    }

    [self configureSortScatterParams];

    [self runClearU32:self.scatterCellCounts count:self.numCells];

    [self dispatchGridKernel:self.scatterComputeCellIdx
                     buffers:@[self.particlePos, self.particleCellIdx, self.sortScatterParams]
                 threadCount:self.numOsc];

    [self dispatchGridKernel:self.scatterCountCells
                     buffers:@[self.particleCellIdx, self.scatterCellCounts, self.sortScatterParams]
                 threadCount:self.numOsc];

    if (![self runScatterPrefixSum:error]) {
        return NO;
    }

    [self runCopyU32:self.scatterCellStarts dst:self.scatterCellOffsets count:self.numCells];

    [self dispatchGridKernel:self.scatterReorderParticles
                     buffers:@[
                         self.particlePos, self.particleVel, self.particleMass, self.oscHeat, self.particleEnergy,
                         self.particleCellIdx, self.scatterCellStarts, self.scatterCellOffsets,
                         self.particlePosSorted, self.particleVelSorted, self.particleMassSorted,
                         self.particleHeatSorted, self.particleEnergySorted, self.sortedOriginalIdx,
                         self.sortScatterParams, self.particleCicA, self.particleCicB
                     ]
                 threadCount:self.numOsc];

    [self dispatchGridKernel:self.scatterGatherCells
                     buffers:@[
                         self.particleCicA, self.particleCicB, self.scatterCellStarts,
                         self.momRho, self.eInt, self.sortScatterParams
                     ]
                 threadCount:self.numCells];

    return YES;
}

- (BOOL)runPicGather:(NSString **)error {
    (void)error;

    if (self.numOsc == 0) {
        return YES;
    }

    [self configurePicGatherParams];

    [self dispatchGridKernel:self.picGatherUpdate
                     buffers:@[
                         self.particlePos, self.particleMass,
                         self.particlePosSorted, self.particleVel,
                         self.oscHeat, self.oscHeat,
                         self.momRho, self.eInt,
                         self.gravityPotential, self.picGatherParams,
                         self.dbgHead, self.dbgWords, self.dbgCap
                     ]
                 threadCount:self.numOsc];

    [self runCopyFloat:self.particlePosSorted dst:self.particlePos count:self.numOsc * 3];

    return YES;
}

@end
