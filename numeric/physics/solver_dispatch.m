#import "solver_private.h"

void manifold_write_error(char *err_out, int err_cap, NSString *message) {
    if (err_out == NULL || err_cap <= 0) {
        return;
    }

    const char *utf8 = message.UTF8String;

    if (utf8 == NULL) {
        err_out[0] = '\0';
        return;
    }

    strncpy(err_out, utf8, (size_t)err_cap - 1);
    err_out[err_cap - 1] = '\0';
}

uint32_t manifold_cell_index(uint32_t x, uint32_t y, uint32_t z, uint32_t gx, uint32_t gy, uint32_t gz) {
    return x * (gy * gz) + y * gz + z;
}

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

@implementation ManifoldSolver (DispatchPrivate)

- (void)beginStepDispatches {
    if (self.stepDispatchActive) {
        return;
    }

    self.stepCommandBuffer = [self.queue commandBuffer];
    self.stepEncoder = [self.stepCommandBuffer computeCommandEncoder];
    self.stepDispatchActive = YES;
}

- (void)flushStepDispatches {
    if (!self.stepDispatchActive) {
        return;
    }

    [self.stepEncoder endEncoding];
    self.stepEncoder = nil;
    [self.stepCommandBuffer commit];
    [self.stepCommandBuffer waitUntilCompleted];

    if (self.stepCommandBuffer.status == MTLCommandBufferStatusError) {
        NSLog(@"[METAL ERROR] flushStepDispatches failed: %@", self.stepCommandBuffer.error);
    }

    self.stepCommandBuffer = [self.queue commandBuffer];
    self.stepEncoder = [self.stepCommandBuffer computeCommandEncoder];
}

- (void)endStepDispatches {
    if (!self.stepDispatchActive) {
        return;
    }

    [self.stepEncoder endEncoding];
    self.stepEncoder = nil;
    [self.stepCommandBuffer commit];
    [self.stepCommandBuffer waitUntilCompleted];

    if (self.stepCommandBuffer.status == MTLCommandBufferStatusError) {
        NSLog(@"[METAL ERROR] endStepDispatches failed: %@", self.stepCommandBuffer.error);
    }

    self.stepCommandBuffer = nil;
    self.stepDispatchActive = NO;
}

- (void)dispatchGasBrickSynchronized:(id<MTLComputePipelineState>)pipeline
                             buffers:(NSArray<id<MTLBuffer>> *)buffers {
    BOOL priorActive = self.stepDispatchActive;
    self.stepDispatchActive = NO;
    [self dispatchGasBrickKernel:pipeline buffers:buffers];
    self.stepDispatchActive = priorActive;
}

- (id<MTLBuffer>)newSharedBufferWithLength:(size_t)length {
    return [self.device newBufferWithLength:length options:MTLResourceStorageModeShared];
}

- (id<MTLBuffer>)newGPUBufferWithLength:(size_t)length {
    MTLResourceOptions options = MTLResourceStorageModePrivate | MTLResourceHazardTrackingModeUntracked;

    if (self.gpuHeap != nil) {
        id<MTLBuffer> heapBuffer = [self.gpuHeap newBufferWithLength:length options:options];

        if (heapBuffer != nil) {
            return heapBuffer;
        }
    }

    return [self.device newBufferWithLength:length options:options];
}

- (void)dispatchGridKernelSynchronized:(id<MTLComputePipelineState>)pipeline
                               buffers:(NSArray<id<MTLBuffer>> *)buffers
                           threadCount:(NSUInteger)threadCount {
    BOOL priorActive = self.stepDispatchActive;
    self.stepDispatchActive = NO;
    [self dispatchGridKernel:pipeline buffers:buffers threadCount:threadCount];
    self.stepDispatchActive = priorActive;
}

- (void)dispatchThreadgroupKernelSynchronized:(id<MTLComputePipelineState>)pipeline
                                      buffers:(NSArray<id<MTLBuffer>> *)buffers
                                threadgroupSize:(NSUInteger)threadgroupSize
                               threadgroupCount:(NSUInteger)threadgroupCount
                       threadgroupMemoryLength:(NSUInteger)threadgroupMemoryLength {
    BOOL priorActive = self.stepDispatchActive;
    self.stepDispatchActive = NO;
    [self dispatchThreadgroupKernel:pipeline
                            buffers:buffers
                      threadgroupSize:threadgroupSize
                     threadgroupCount:threadgroupCount
             threadgroupMemoryLength:threadgroupMemoryLength];
    self.stepDispatchActive = priorActive;
}

- (void)dispatchGridKernel:(id<MTLComputePipelineState>)pipeline
                   buffers:(NSArray<id<MTLBuffer>> *)buffers
               threadCount:(NSUInteger)threadCount {
    id<MTLComputeCommandEncoder> encoder = nil;
    id<MTLCommandBuffer> commandBuffer = nil;
    BOOL ownedDispatch = NO;

    if (self.stepDispatchActive && self.stepEncoder != nil) {
        encoder = self.stepEncoder;
    } else {
        commandBuffer = [self.queue commandBuffer];
        encoder = [commandBuffer computeCommandEncoder];
        ownedDispatch = YES;
    }

    [encoder setComputePipelineState:pipeline];

    for (NSUInteger index = 0; index < buffers.count; index++) {
        [encoder setBuffer:buffers[index] offset:0 atIndex:(NSUInteger)index];
    }

    NSUInteger width = kHeavyKernelThreads;

    if (width > threadCount) {
        width = pipeline.threadExecutionWidth;

        if (width == 0) {
            width = 1;
        }

        if (width > threadCount) {
            width = threadCount;
        }

        width = ((width + pipeline.threadExecutionWidth - 1) / pipeline.threadExecutionWidth) * pipeline.threadExecutionWidth;

        if (width > threadCount) {
            width = threadCount;
        }
    }

    MTLSize gridSize = MTLSizeMake(threadCount, 1, 1);
    MTLSize threadgroupSize = MTLSizeMake(width, 1, 1);
    [encoder dispatchThreads:gridSize threadsPerThreadgroup:threadgroupSize];

    if (self.stepDispatchActive && !ownedDispatch) {
        [encoder memoryBarrierWithScope:MTLBarrierScopeBuffers];
    }

    if (ownedDispatch) {
        [encoder endEncoding];
        [commandBuffer commit];
        [commandBuffer waitUntilCompleted];
    }
}

- (void)dispatchGasBrickKernel:(id<MTLComputePipelineState>)pipeline
                       buffers:(NSArray<id<MTLBuffer>> *)buffers {
    id<MTLComputeCommandEncoder> encoder = nil;
    id<MTLCommandBuffer> commandBuffer = nil;
    BOOL ownedDispatch = NO;

    if (self.stepDispatchActive && self.stepEncoder != nil) {
        encoder = self.stepEncoder;
    } else {
        commandBuffer = [self.queue commandBuffer];
        encoder = [commandBuffer computeCommandEncoder];
        ownedDispatch = YES;
    }

    [encoder setComputePipelineState:pipeline];

    for (NSUInteger index = 0; index < buffers.count; index++) {
        [encoder setBuffer:buffers[index] offset:0 atIndex:(NSUInteger)index];
    }

    uint32_t gridX = self.config.grid_x;
    uint32_t gridY = self.config.grid_y;
    uint32_t gridZ = self.config.grid_z;
    MTLSize threadsPerThreadgroup = MTLSizeMake(kGasBrickZ, kGasBrickY, kGasBrickX);
    MTLSize threadgroups = MTLSizeMake(
        (gridZ + kGasBrickZ - 1u) / kGasBrickZ,
        (gridY + kGasBrickY - 1u) / kGasBrickY,
        (gridX + kGasBrickX - 1u) / kGasBrickX
    );
    [encoder dispatchThreadgroups:threadgroups threadsPerThreadgroup:threadsPerThreadgroup];

    if (self.stepDispatchActive && !ownedDispatch) {
        [encoder memoryBarrierWithScope:MTLBarrierScopeBuffers];
    }

    if (ownedDispatch) {
        [encoder endEncoding];
        [commandBuffer commit];
        [commandBuffer waitUntilCompleted];
    }
}

- (void)dispatchThreadgroupKernel:(id<MTLComputePipelineState>)pipeline
                          buffers:(NSArray<id<MTLBuffer>> *)buffers
                    threadgroupSize:(NSUInteger)threadgroupSize
                    threadgroupCount:(NSUInteger)threadgroupCount
            threadgroupMemoryLength:(NSUInteger)threadgroupMemoryLength {
    NSArray<NSNumber *> *lengths = nil;

    if (threadgroupMemoryLength > 0) {
        lengths = @[ @(threadgroupMemoryLength) ];
    }

    [self dispatchThreadgroupKernel:pipeline
                            buffers:buffers
                      threadgroupSize:threadgroupSize
                     threadgroupCount:threadgroupCount
            threadgroupMemoryLengths:lengths];
}

- (void)dispatchThreadgroupKernel:(id<MTLComputePipelineState>)pipeline
                          buffers:(NSArray<id<MTLBuffer>> *)buffers
                    threadgroupSize:(NSUInteger)threadgroupSize
                    threadgroupCount:(NSUInteger)threadgroupCount
           threadgroupMemoryLengths:(NSArray<NSNumber *> *)threadgroupMemoryLengths {
    id<MTLComputeCommandEncoder> encoder = nil;
    id<MTLCommandBuffer> commandBuffer = nil;
    BOOL ownedDispatch = NO;

    if (self.stepDispatchActive && self.stepEncoder != nil) {
        encoder = self.stepEncoder;
    } else {
        commandBuffer = [self.queue commandBuffer];
        encoder = [commandBuffer computeCommandEncoder];
        ownedDispatch = YES;
    }

    [encoder setComputePipelineState:pipeline];

    for (NSUInteger index = 0; index < buffers.count; index++) {
        [encoder setBuffer:buffers[index] offset:0 atIndex:(NSUInteger)index];
    }

    for (NSUInteger index = 0; index < threadgroupMemoryLengths.count; index++) {
        NSUInteger memoryLength = threadgroupMemoryLengths[index].unsignedIntegerValue;

        if (memoryLength > 0) {
            [encoder setThreadgroupMemoryLength:memoryLength atIndex:index];
        }
    }

    MTLSize threadsPerThreadgroup = MTLSizeMake(threadgroupSize, 1, 1);
    MTLSize threadgroups = MTLSizeMake(threadgroupCount, 1, 1);
    [encoder dispatchThreadgroups:threadgroups threadsPerThreadgroup:threadsPerThreadgroup];

    if (self.stepDispatchActive && !ownedDispatch) {
        [encoder memoryBarrierWithScope:MTLBarrierScopeBuffers];
    }

    if (ownedDispatch) {
        [encoder endEncoding];
        [commandBuffer commit];
        [commandBuffer waitUntilCompleted];
    }
}

@end
