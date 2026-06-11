#import "solver_private.h"

static const uint32_t kReduceThreads = 256u;

@implementation ManifoldSolver (UtilPrivate)

- (void)runClearField:(id<MTLBuffer>)field count:(uint32_t)count {
    if (field == nil || count == 0) {
        return;
    }

    id<MTLCommandBuffer> commandBuffer = nil;
    id<MTLBlitCommandEncoder> blitEncoder = nil;
    BOOL priorActive = self.stepDispatchActive;

    if (priorActive && self.stepEncoder != nil) {
        [self.stepEncoder endEncoding];
        self.stepEncoder = nil;
        commandBuffer = self.stepCommandBuffer;
    } else {
        commandBuffer = [self.queue commandBuffer];
    }

    blitEncoder = [commandBuffer blitCommandEncoder];
    [blitEncoder fillBuffer:field range:NSMakeRange(0, (size_t)count * sizeof(float)) value:0];
    [blitEncoder endEncoding];

    if (priorActive) {
        self.stepEncoder = [commandBuffer computeCommandEncoder];
    } else {
        [commandBuffer commit];
        [commandBuffer waitUntilCompleted];
    }
}

- (void)runClearU32:(id<MTLBuffer>)buffer count:(uint32_t)count {
    if (buffer == nil || count == 0) {
        return;
    }

    id<MTLCommandBuffer> commandBuffer = nil;
    id<MTLBlitCommandEncoder> blitEncoder = nil;
    BOOL priorActive = self.stepDispatchActive;

    if (priorActive && self.stepEncoder != nil) {
        [self.stepEncoder endEncoding];
        self.stepEncoder = nil;
        commandBuffer = self.stepCommandBuffer;
    } else {
        commandBuffer = [self.queue commandBuffer];
    }

    blitEncoder = [commandBuffer blitCommandEncoder];
    [blitEncoder fillBuffer:buffer range:NSMakeRange(0, (size_t)count * sizeof(uint32_t)) value:0];
    [blitEncoder endEncoding];

    if (priorActive) {
        self.stepEncoder = [commandBuffer computeCommandEncoder];
    } else {
        [commandBuffer commit];
        [commandBuffer waitUntilCompleted];
    }
}

- (void)runCopyU32:(id<MTLBuffer>)src dst:(id<MTLBuffer>)dst count:(uint32_t)count {
    id<MTLBuffer> countBuf = [self.device newBufferWithBytes:&count length:sizeof(uint32_t) options:MTLResourceStorageModeShared];

    [self dispatchGridKernel:self.pipelineCopyBufferU32
                     buffers:@[src, dst, countBuf]
                 threadCount:count];
}

- (void)runCopyFloat:(id<MTLBuffer>)src dst:(id<MTLBuffer>)dst count:(uint32_t)count {
    id<MTLBuffer> countBuf = [self.device newBufferWithBytes:&count length:sizeof(uint32_t) options:MTLResourceStorageModeShared];

    [self dispatchGridKernel:self.pipelineCopyBufferFloat
                     buffers:@[src, dst, countBuf]
                 threadCount:count];
}

- (void)runCopyBitsToFloat:(id<MTLBuffer>)srcBits dst:(id<MTLBuffer>)dst count:(uint32_t)count {
    id<MTLBuffer> countBuf = [self.device newBufferWithBytes:&count length:sizeof(uint32_t) options:MTLResourceStorageModeShared];

    [self dispatchGridKernel:self.pipelineCopyBitsToFloat
                     buffers:@[srcBits, dst, countBuf]
                 threadCount:count];
}

- (void)runScatterPrefixSeedLast:(id<MTLBuffer>)data count:(uint32_t)count {
    id<MTLBuffer> countBuf = [self.device newBufferWithBytes:&count length:sizeof(uint32_t) options:MTLResourceStorageModeShared];

    [self dispatchGridKernel:self.scatterPrefixSeedLast
                     buffers:@[data, countBuf]
                 threadCount:1];
}

- (void)runClearCarrierAccums:(uint32_t)numCarriers {
    id<MTLBuffer> countBuf = [self.device newBufferWithBytes:&numCarriers length:sizeof(uint32_t) options:MTLResourceStorageModeShared];

    [self dispatchGridKernel:self.clearCarrierAccums
                     buffers:@[self.accums, countBuf]
                 threadCount:numCarriers];
}

- (uint32_t)runDeriveMaxCarrierBin {
    [self runClearU32:self.maxCarrierBinKey count:1];

    [self dispatchGridKernelSynchronized:self.deriveMaxCarrierBin
                                 buffers:@[
                                     self.modeOmega,
                                     self.binParams,
                                     self.maxCarrierBinKey,
                                     self.numCarriers
                                 ]
                             threadCount:self.numOsc];

    return *(uint32_t *)self.maxCarrierBinKey.contents;
}

- (void)runReduceFloatStats:(id<MTLBuffer>)values length:(uint32_t)length statsOut:(float *)statsOut {
    if (length == 0) {
        statsOut[0] = 0.0f;
        statsOut[1] = 0.0f;
        statsOut[2] = 0.0f;
        statsOut[3] = 0.0f;
        return;
    }

    uint32_t numGroups = (length + kReduceThreads - 1u) / kReduceThreads;
    size_t groupBytes = (size_t)numGroups * 4u * sizeof(float);

    if (self.reduceGroupStats.length < groupBytes) {
        self.reduceGroupStats = [self.device newBufferWithLength:groupBytes options:MTLResourceStorageModeShared];
    }

    id<MTLBuffer> lengthBuf = [self.device newBufferWithBytes:&length length:sizeof(uint32_t) options:MTLResourceStorageModeShared];
    id<MTLBuffer> numGroupsBuf = [self.device newBufferWithBytes:&numGroups length:sizeof(uint32_t) options:MTLResourceStorageModeShared];

    [self dispatchThreadgroupKernelSynchronized:self.reduceFloatStatsPass1
                                        buffers:@[values, self.reduceGroupStats, lengthBuf]
                                  threadgroupSize:kReduceThreads
                                 threadgroupCount:numGroups
                         threadgroupMemoryLength:kReduceThreads * 4u * sizeof(float)];

    [self dispatchGridKernelSynchronized:self.reduceFloatStatsFinalize
                                 buffers:@[self.reduceGroupStats, self.reduceStatsOut, numGroupsBuf]
                             threadCount:1];

    float *statsData = (float *)self.reduceStatsOut.contents;
    statsOut[0] = statsData[0];
    statsOut[1] = statsData[1];
    statsOut[2] = statsData[2];
    statsOut[3] = statsData[3];
}

- (void)configureParticleGenParams {
    ParticleGenParamsHost *params = (ParticleGenParamsHost *)self.particleGenParams.contents;
    float spacing = [self gridSpacing];

    params->num_particles = self.numOsc;
    params->grid_x = (float)self.config.grid_x;
    params->grid_y = (float)self.config.grid_y;
    params->grid_z = (float)self.config.grid_z;
    params->energy_scale = self.config.rho_min;
    params->pattern = 3u;
    params->center_x = 0.5f * self.config.domain_x;
    params->center_y = 0.5f * self.config.domain_y;
    params->center_z = 0.5f * self.config.domain_z;
    params->spread = spacing;
    params->dir_x = 1.0f;
    params->dir_y = 0.0f;
    params->dir_z = 0.0f;
}

- (void)seedRandomValuesFromOscillators:(const ManifoldOscillator *)oscillators count:(uint32_t)count {
    float *randomData = (float *)self.particleRandomVals.contents;

    for (uint32_t index = 0; index < count; index++) {
        const ManifoldOscillator *oscillator = &oscillators[index];
        randomData[index * 4 + 0] = fmodf(fabsf(oscillator->phase), 1.0f);
        randomData[index * 4 + 1] = fmodf(fabsf(oscillator->omega) / (float)(2.0 * M_PI), 1.0f);
        randomData[index * 4 + 2] = fmodf(fabsf(oscillator->amplitude), 1.0f);
        randomData[index * 4 + 3] = fmodf(fabsf(oscillator->heat), 1.0f);
    }
}

- (void)runInitializeParticleProperties:(const ManifoldOscillator *)oscillators count:(uint32_t)count {
    [self configureParticleGenParams];
    [self seedRandomValuesFromOscillators:oscillators count:count];

    float centerX = 0.0f;
    float centerY = 0.0f;
    float centerZ = 0.0f;

    for (uint32_t index = 0; index < count; index++) {
        centerX += oscillators[index].pos_x;
        centerY += oscillators[index].pos_y;
        centerZ += oscillators[index].pos_z;
    }

    centerX /= (float)count;
    centerY /= (float)count;
    centerZ /= (float)count;

    id<MTLBuffer> centerXBuf = [self.device newBufferWithBytes:&centerX length:sizeof(float) options:MTLResourceStorageModeShared];
    id<MTLBuffer> centerYBuf = [self.device newBufferWithBytes:&centerY length:sizeof(float) options:MTLResourceStorageModeShared];
    id<MTLBuffer> centerZBuf = [self.device newBufferWithBytes:&centerZ length:sizeof(float) options:MTLResourceStorageModeShared];

    [self dispatchGridKernel:self.initializeParticleProperties
                     buffers:@[
                         self.particlePos, self.particleVel, self.particleEnergy, self.oscHeat,
                         self.particleExcitation, self.particleMass, self.particleRandomVals,
                         self.particleGenParams, centerXBuf, centerYBuf, centerZBuf
                     ]
                 threadCount:count];
}

- (BOOL)runExclusiveScanU32:(id<MTLBuffer>)input
                      output:(id<MTLBuffer>)output
                      length:(uint32_t)length
              writeTotalSlot:(BOOL)writeTotalSlot
                       error:(NSString **)error {
    (void)error;

    if (length == 0) {
        return YES;
    }

    uint32_t numBlocks = (length + kScanThreads - 1u) / kScanThreads;
    size_t blockBytes = (size_t)numBlocks * sizeof(uint32_t);

    if (self.scanBlockSums.length < blockBytes) {
        self.scanBlockSums = [self.device newBufferWithLength:blockBytes options:MTLResourceStorageModeShared];
    }

    if (self.scanBlockPrefix.length < blockBytes) {
        self.scanBlockPrefix = [self.device newBufferWithLength:blockBytes options:MTLResourceStorageModeShared];
    }

    if (self.scanBlockScratch.length < blockBytes) {
        self.scanBlockScratch = [self.device newBufferWithLength:blockBytes options:MTLResourceStorageModeShared];
    }

    id<MTLBuffer> lengthBuf = [self.device newBufferWithBytes:&length length:sizeof(uint32_t) options:MTLResourceStorageModeShared];

    [self dispatchThreadgroupKernel:self.scanPass1
                            buffers:@[input, output, self.scanBlockSums, lengthBuf]
                      threadgroupSize:kScanThreads
                     threadgroupCount:numBlocks
             threadgroupMemoryLength:kScanThreads * sizeof(uint32_t)];

    uint32_t blockCount = numBlocks;
    id<MTLBuffer> blockCountBuf = [self.device newBufferWithBytes:&blockCount length:sizeof(uint32_t) options:MTLResourceStorageModeShared];

    [self dispatchThreadgroupKernel:self.scanPass1
                            buffers:@[self.scanBlockSums, self.scanBlockPrefix, self.scanBlockScratch, blockCountBuf]
                      threadgroupSize:kScanThreads
                     threadgroupCount:1
             threadgroupMemoryLength:kScanThreads * sizeof(uint32_t)];

    [self dispatchThreadgroupKernel:self.scanAddBlockOffsets
                            buffers:@[output, self.scanBlockPrefix, lengthBuf]
                      threadgroupSize:kScanThreads
                     threadgroupCount:numBlocks
             threadgroupMemoryLength:0];

    if (writeTotalSlot) {
        [self dispatchGridKernel:self.scanFinalizeTotal
                         buffers:@[input, output, lengthBuf]
                     threadCount:1];
    }

    return YES;
}

@end
