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
    float *rhoData,
    float *momData,
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
    float rho = rhoData[index];

    if (!(rho > 0.0f)) {
        *ux = 0.0f;
        *uy = 0.0f;
        *uz = 0.0f;
        return;
    }

    uint32_t momBase = index * 3;
    *ux = momData[momBase + 0] / rho;
    *uy = momData[momBase + 1] / rho;
    *uz = momData[momBase + 2] / rho;
}

@implementation ManifoldSolver (DispatchPrivate)

- (void)dispatchGridKernel:(id<MTLComputePipelineState>)pipeline
                   buffers:(NSArray<id<MTLBuffer>> *)buffers
               threadCount:(NSUInteger)threadCount {
    id<MTLCommandBuffer> commandBuffer = [self.queue commandBuffer];
    id<MTLComputeCommandEncoder> encoder = [commandBuffer computeCommandEncoder];
    [encoder setComputePipelineState:pipeline];

    for (NSUInteger index = 0; index < buffers.count; index++) {
        [encoder setBuffer:buffers[index] offset:0 atIndex:(NSUInteger)index];
    }

    NSUInteger width = pipeline.threadExecutionWidth;

    if (width > threadCount) {
        width = threadCount;
    }

    if (width == 0) {
        width = 1;
    }

    MTLSize gridSize = MTLSizeMake(threadCount, 1, 1);
    MTLSize threadgroupSize = MTLSizeMake(width, 1, 1);
    [encoder dispatchThreads:gridSize threadsPerThreadgroup:threadgroupSize];
    [encoder endEncoding];
    [commandBuffer commit];
    [commandBuffer waitUntilCompleted];
}

- (void)dispatchThreadgroupKernel:(id<MTLComputePipelineState>)pipeline
                          buffers:(NSArray<id<MTLBuffer>> *)buffers
                    threadgroupSize:(NSUInteger)threadgroupSize
                    threadgroupCount:(NSUInteger)threadgroupCount
            threadgroupMemoryLength:(NSUInteger)threadgroupMemoryLength {
    id<MTLCommandBuffer> commandBuffer = [self.queue commandBuffer];
    id<MTLComputeCommandEncoder> encoder = [commandBuffer computeCommandEncoder];
    [encoder setComputePipelineState:pipeline];

    for (NSUInteger index = 0; index < buffers.count; index++) {
        [encoder setBuffer:buffers[index] offset:0 atIndex:(NSUInteger)index];
    }

    if (threadgroupMemoryLength > 0) {
        [encoder setThreadgroupMemoryLength:threadgroupMemoryLength atIndex:0];
    }

    MTLSize threadsPerThreadgroup = MTLSizeMake(threadgroupSize, 1, 1);
    MTLSize threadgroups = MTLSizeMake(threadgroupCount, 1, 1);
    [encoder dispatchThreadgroups:threadgroups threadsPerThreadgroup:threadsPerThreadgroup];
    [encoder endEncoding];
    [commandBuffer commit];
    [commandBuffer waitUntilCompleted];
}

@end
