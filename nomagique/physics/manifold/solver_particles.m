#import "solver_private.h"

static const uint32_t kDirectInteractionLimit = 64u;

@implementation ManifoldSolver (ParticlesPrivate)

- (void)configureSpatialHashParams {
    SpatialHashParamsHost *params = (SpatialHashParamsHost *)self.spatialHashParams.contents;
    float spacing = [self gridSpacing];

    params->num_particles = self.numOsc;
    params->grid_x = self.config.grid_x;
    params->grid_y = self.config.grid_y;
    params->grid_z = self.config.grid_z;
    params->cell_size = spacing;
    params->inv_cell_size = 1.0f / spacing;
    params->domain_min_x = 0.0f;
    params->domain_min_y = 0.0f;
    params->domain_min_z = 0.0f;
}

- (void)configureSpatialCollisionParams {
    SpatialCollisionParamsHost *params = (SpatialCollisionParamsHost *)self.spatialCollisionParams.contents;
    float spacing = [self gridSpacing];

    params->num_particles = self.numOsc;
    params->grid_x = self.config.grid_x;
    params->grid_y = self.config.grid_y;
    params->grid_z = self.config.grid_z;
    params->cell_size = spacing;
    params->inv_cell_size = 1.0f / spacing;
    params->domain_min_x = 0.0f;
    params->domain_min_y = 0.0f;
    params->domain_min_z = 0.0f;
    params->dt = self.controls.dt;
    params->particle_radius = 0.5f * spacing;
    params->young_modulus = self.config.rho_min / (self.controls.dt * self.controls.dt);
    params->thermal_conductivity = self.config.k_thermal;
    params->specific_heat = self.config.c_v;
    params->restitution = 1.0f - self.config.energy_decay * self.controls.dt;
}

- (void)configureParticleInteractionParams {
    ParticleInteractionParamsHost *params = (ParticleInteractionParamsHost *)self.particleInteractionParams.contents;
    float spacing = [self gridSpacing];

    params->num_particles = self.numOsc;
    params->dt = self.controls.dt;
    params->particle_radius = 0.5f * spacing;
    params->young_modulus = self.config.rho_min / (self.controls.dt * self.controls.dt);
    params->thermal_conductivity = self.config.k_thermal;
    params->specific_heat = self.config.c_v;
    params->restitution = 1.0f - self.config.energy_decay * self.controls.dt;
}

- (BOOL)runDirectParticleInteractions:(NSString **)error {
    (void)error;

    [self runCopyFloat:self.particleVel dst:self.particleVelIn count:(uint32_t)(self.particleVel.length / sizeof(float))];
    [self runCopyFloat:self.oscHeat dst:self.particleHeatIn count:self.numOsc];

    [self dispatchGridKernel:self.particleInteractions
                     buffers:@[
                         self.particlePos, self.particleVel, self.particleExcitation, self.particleMass,
                         self.oscHeat, self.particleVelIn, self.particleHeatIn, self.particleInteractionParams
                     ]
                 threadCount:self.numOsc];

    return YES;
}

- (BOOL)runSpatialHashCollisions:(NSString **)error {
    (void)error;

    [self configureSpatialHashParams];
    [self configureSpatialCollisionParams];

    [self runClearU32:self.hashCellCounts count:self.numCells];

    [self dispatchGridKernel:self.spatialHashAssign
                     buffers:@[self.particlePos, self.hashParticleCellIdx, self.hashCellCounts, self.spatialHashParams]
                 threadCount:self.numOsc];

    if (![self runExclusiveScanU32:self.hashCellCounts
                            output:self.hashCellStarts
                            length:self.numCells
                    writeTotalSlot:YES
                             error:error]) {
        return NO;
    }

    [self runCopyU32:self.hashCellStarts dst:self.hashCellOffsets count:self.numCells];

    *(uint32_t *)self.hashNumParticlesBuf.contents = self.numOsc;

    [self dispatchGridKernel:self.spatialHashScatter
                     buffers:@[self.hashParticleCellIdx, self.hashSortedIdx, self.hashCellOffsets, self.hashNumParticlesBuf]
                 threadCount:self.numOsc];

    [self runCopyFloat:self.particleVel dst:self.particleVelIn count:(uint32_t)(self.particleVel.length / sizeof(float))];
    [self runCopyFloat:self.oscHeat dst:self.particleHeatIn count:self.numOsc];

    [self dispatchGridKernel:self.spatialHashCollisions
                     buffers:@[
                         self.particlePos, self.particleVel, self.particleExcitation, self.particleMass, self.oscHeat,
                         self.hashSortedIdx, self.hashCellStarts, self.hashParticleCellIdx,
                         self.particleVelIn, self.particleHeatIn, self.spatialCollisionParams
                     ]
                 threadCount:self.numOsc];

    return YES;
}

- (BOOL)runParticleCollisions:(NSString **)error {
    if (self.numOsc == 0) {
        return YES;
    }

    if (self.numOsc <= kDirectInteractionLimit) {
        return [self runDirectParticleInteractions:error];
    }

    return [self runSpatialHashCollisions:error];
}

- (void)configureModeProjectParams {
    ModeProjectParamsHost *params = (ModeProjectParamsHost *)self.modeProjectParams.contents;
    float cellX = self.config.domain_x / (float)self.config.grid_x;
    float cellY = self.config.domain_y / (float)self.config.grid_y;
    float cellZ = self.config.domain_z / (float)self.config.grid_z;

    params->num_modes = self.numOsc;
    params->num_particles = self.numOsc;
    params->anchors_per_mode = kModeAnchors;
    params->grid_x = self.config.grid_x;
    params->grid_y = self.config.grid_y;
    params->grid_z = self.config.grid_z;
    params->cell_x = cellX;
    params->cell_y = cellY;
    params->cell_z = cellZ;
    params->inv_cell_x = 1.0f / cellX;
    params->inv_cell_y = 1.0f / cellY;
    params->inv_cell_z = 1.0f / cellZ;
    params->domain_x = self.config.domain_x;
    params->dt = self.controls.dt;
}

- (void)configurePilotWaveParams {
    PilotWaveParamsHost *params = (PilotWaveParamsHost *)self.pilotWaveParams.contents;
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
    params->hbar_eff = self.config.hbar_eff;
    // Wave regulariser must sit far below typical |Ψ|². rho_min² is a gas
    // floor and dominated the Bohm denominator, collapsing guidance to ~0.
    float ampScale = 1.0f / fmaxf((float)self.numOsc, 1.0f);
    params->eps_denom = fmaxf(1e-12f, ampScale * ampScale * 1e-8f);
    // Floor only against numerical zero; carrier mass is oscillator amplitude.
    params->mass_min = fmaxf(ampScale * 1e-3f, 1e-6f);
    params->boundary_x_low = self.config.boundary_x_low;
    params->boundary_x_high = self.config.boundary_x_high;
    params->boundary_y_low = self.config.boundary_y_low;
    params->boundary_y_high = self.config.boundary_y_high;
    params->boundary_z_low = self.config.boundary_z_low;
    params->boundary_z_high = self.config.boundary_z_high;
}

- (void)clearPsiFields {
    [self runClearField:self.psiReAtomic count:self.numCells];
    [self runClearField:self.psiImAtomic count:self.numCells];
}

- (void)copyPsiAtomicsToFields {
    [self runCopyBitsToFloat:self.psiReAtomic dst:self.psiReField count:self.numCells];
    [self runCopyBitsToFloat:self.psiImAtomic dst:self.psiImField count:self.numCells];
}

- (BOOL)runProjectModesToSpatialPsi:(NSString **)error {
    (void)error;

    if (self.numOsc == 0) {
        return YES;
    }

    [self configureModeProjectParams];
    [self clearPsiFields];

    uint32_t totalAnchors = self.numOsc * kModeAnchors;

    [self dispatchGridKernel:self.projectModesToSpatialPsi
                     buffers:@[
                         self.modeReal, self.modeImag, self.modeAnchorIdx, self.modeAnchorWeight,
                         self.particlePos, self.psiReAtomic, self.psiImAtomic, self.modeProjectParams,
                         self.modeOmega
                     ]
                 threadCount:totalAnchors];

    [self copyPsiAtomicsToFields];

    return YES;
}

- (BOOL)runPilotWaveGather:(NSString **)error {
    (void)error;

    if (self.numOsc == 0) {
        return YES;
    }

    [self configurePilotWaveParams];

    [self dispatchGridKernel:self.picGatherPilotWave
                     buffers:@[
                         self.particlePos, self.particleMass,
                         self.particlePosSorted, self.particleVel,
                         self.psiReField, self.psiImField, self.pilotWaveParams
                     ]
                 threadCount:self.numOsc];

    [self runCopyFloat:self.particlePosSorted dst:self.particlePos count:self.numOsc * 3];

    return YES;
}

@end
