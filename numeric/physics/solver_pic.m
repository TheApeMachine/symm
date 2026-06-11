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
        float mass = oscillator->amplitude * self.config.rho_min;

        if (!(mass > 0.0f)) {
            mass = self.config.rho_min;
        }

        massData[index] = mass;
        energyData[index] = oscillator->heat;
        velData[index * 3 + 0] = 0.0f;
        velData[index * 3 + 1] = 0.0f;
        velData[index * 3 + 2] = 0.0f;
    }
}

- (void)configureSortScatterParams {
    SortScatterParamsHost *params = (SortScatterParamsHost *)self.sortScatterParams.contents;
    float spacing = [self gridSpacing];

    params->num_particles = self.numOsc;
    params->num_cells = self.numCells;
    params->grid_x = self.config.grid_x;
    params->grid_y = self.config.grid_y;
    params->grid_z = self.config.grid_z;
    params->grid_spacing = spacing;
    params->inv_grid_spacing = 1.0f / spacing;
}

- (void)configurePicGatherParams {
    PicGatherParamsHost *params = (PicGatherParamsHost *)self.picGatherParams.contents;
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
    params->gamma = self.config.gamma;
    params->R_specific = (self.config.gamma - 1.0f) * self.config.c_v;
    params->c_v = self.config.c_v;
    params->rho_min = self.config.rho_min;
    params->p_min = self.config.p_min;
    params->gravity_enabled = self.gravityReady ? 1.0f : 0.0f;
}

- (BOOL)runScatterPrefixSum:(NSString **)error {
    (void)error;
    uint32_t numCells = self.numCells;

    if (numCells == 0) {
        return YES;
    }

    [self runCopyU32:self.scatterCellCounts dst:self.scatterCellStarts count:numCells];

    for (uint32_t stride = 1; stride < numCells; stride <<= 1) {
        id<MTLBuffer> strideBuf = [self.device newBufferWithBytes:&stride length:sizeof(uint32_t) options:MTLResourceStorageModeShared];
        id<MTLBuffer> numCellsBuf = [self.device newBufferWithBytes:&numCells length:sizeof(uint32_t) options:MTLResourceStorageModeShared];

        [self dispatchGridKernel:self.scatterPrefixUpsweep
                         buffers:@[self.scatterCellStarts, strideBuf, numCellsBuf]
                     threadCount:numCells];
    }

    [self runScatterPrefixSeedLast:self.scatterCellStarts count:numCells];

    for (uint32_t stride = numCells >> 1; stride > 0; stride >>= 1) {
        id<MTLBuffer> strideBuf = [self.device newBufferWithBytes:&stride length:sizeof(uint32_t) options:MTLResourceStorageModeShared];
        id<MTLBuffer> numCellsBuf = [self.device newBufferWithBytes:&numCells length:sizeof(uint32_t) options:MTLResourceStorageModeShared];

        [self dispatchGridKernel:self.scatterPrefixDownsweep
                         buffers:@[self.scatterCellStarts, strideBuf, numCellsBuf]
                     threadCount:numCells];
    }

    return YES;
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
