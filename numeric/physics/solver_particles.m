#import "solver_private.h"

static const uint32_t kDirectInteractionLimit = 64u;
static const uint32_t kModeAnchors = 8u;

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
    params->dt = self.config.dt;
    params->particle_radius = 0.5f * spacing;
    params->young_modulus = self.config.rho_min / (self.config.dt * self.config.dt);
    params->thermal_conductivity = self.config.k_thermal;
    params->specific_heat = self.config.c_v;
    params->restitution = 1.0f - self.config.energy_decay * self.config.dt;
}

- (void)configureParticleInteractionParams {
    ParticleInteractionParamsHost *params = (ParticleInteractionParamsHost *)self.particleInteractionParams.contents;
    float spacing = [self gridSpacing];

    params->num_particles = self.numOsc;
    params->dt = self.config.dt;
    params->particle_radius = 0.5f * spacing;
    params->young_modulus = self.config.rho_min / (self.config.dt * self.config.dt);
    params->thermal_conductivity = self.config.k_thermal;
    params->specific_heat = self.config.c_v;
    params->restitution = 1.0f - self.config.energy_decay * self.config.dt;
}

- (BOOL)runDirectParticleInteractions:(NSString **)error {
    (void)error;

    memcpy(self.particleVelIn.contents, self.particleVel.contents, self.particleVel.length);
    memcpy(self.particleHeatIn.contents, self.oscHeat.contents, self.oscHeat.length);

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

    memset(self.hashCellCounts.contents, 0, self.hashCellCounts.length);

    [self dispatchGridKernel:self.spatialHashAssign
                     buffers:@[self.particlePos, self.hashParticleCellIdx, self.hashCellCounts, self.spatialHashParams]
                 threadCount:self.numOsc];

    if (self.numCells >= kParallelHashCellThreshold) {
        if (![self runSpatialHashPrefixSumParallel:error]) {
            return NO;
        }

        memcpy(self.hashCellStarts.contents, self.hashCellCounts.contents, (size_t)self.numCells * sizeof(uint32_t));
    }

    if (self.numCells < kParallelHashCellThreshold) {
        [self dispatchGridKernel:self.spatialHashPrefixSum
                         buffers:@[self.hashCellCounts, self.hashCellStarts, self.hashNumCellsBuf]
                     threadCount:1];
    }

    memcpy(self.hashCellOffsets.contents, self.hashCellStarts.contents, (size_t)self.numCells * sizeof(uint32_t));

    *(uint32_t *)self.hashNumParticlesBuf.contents = self.numOsc;

    [self dispatchGridKernel:self.spatialHashScatter
                     buffers:@[self.hashParticleCellIdx, self.hashSortedIdx, self.hashCellOffsets, self.hashNumParticlesBuf]
                 threadCount:self.numOsc];

    memcpy(self.particleVelIn.contents, self.particleVel.contents, self.particleVel.length);
    memcpy(self.particleHeatIn.contents, self.oscHeat.contents, self.oscHeat.length);

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
    float spacing = [self gridSpacing];

    params->num_modes = self.numOsc;
    params->num_particles = self.numOsc;
    params->anchors_per_mode = kModeAnchors;
    params->grid_x = self.config.grid_x;
    params->grid_y = self.config.grid_y;
    params->grid_z = self.config.grid_z;
    params->grid_spacing = spacing;
    params->inv_grid_spacing = 1.0f / spacing;
}

- (void)configurePilotWaveParams {
    PilotWaveParamsHost *params = (PilotWaveParamsHost *)self.pilotWaveParams.contents;
    float spacing = [self gridSpacing];

    params->num_particles = self.numOsc;
    params->grid_x = self.config.grid_x;
    params->grid_y = self.config.grid_y;
    params->grid_z = self.config.grid_z;
    params->grid_spacing = spacing;
    params->inv_grid_spacing = 1.0f / spacing;
    params->dt = self.config.dt;
    params->domain_x = self.config.domain_x;
    params->domain_y = self.config.domain_y;
    params->domain_z = self.config.domain_z;
    params->hbar_eff = self.config.hbar_eff;
    params->eps_denom = self.config.rho_min * self.config.rho_min;
    params->mass_min = self.config.rho_min;
}

- (void)clearPsiFields {
    [self runClearField:self.psiReAtomic count:self.numCells];
    [self runClearField:self.psiImAtomic count:self.numCells];
}

- (void)copyPsiAtomicsToFields {
    uint32_t *reAtomic = (uint32_t *)self.psiReAtomic.contents;
    uint32_t *imAtomic = (uint32_t *)self.psiImAtomic.contents;
    float *reField = (float *)self.psiReField.contents;
    float *imField = (float *)self.psiImField.contents;

    for (uint32_t index = 0; index < self.numCells; index++) {
        memcpy(&reField[index], &reAtomic[index], sizeof(float));
        memcpy(&imField[index], &imAtomic[index], sizeof(float));
    }
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
                         self.particlePos, self.psiReAtomic, self.psiImAtomic, self.modeProjectParams
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

    memcpy(self.particlePos.contents, self.particlePosSorted.contents, (size_t)self.numOsc * 3 * sizeof(float));

    return YES;
}

@end
