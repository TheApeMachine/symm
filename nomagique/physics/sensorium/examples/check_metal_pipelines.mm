// Native Metal-only acceptance check. It creates every pipeline referenced by
// the supplied host sources; it does not execute kernels or prove numerical fidelity.
#import <Foundation/Foundation.h>
#import <Metal/Metal.h>
#include <cstdio>
#include <fstream>
#include <string>

int main(int argc, char** argv) {
    if (argc != 3) {
        std::fprintf(stderr, "usage: check_metal_pipelines library.metallib kernel_names.txt\n");
        return 2;
    }
    @autoreleasepool {
        id<MTLDevice> device = MTLCreateSystemDefaultDevice();
        if (!device || ![device supportsFamily:MTLGPUFamilyApple8]) {
            std::fprintf(stderr, "Requires the macOS Apple8+ feature set for ulong atomic min/max\n");
            return 2;
        }
        NSError* error = nil;
        NSURL* url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:argv[1]]];
        id<MTLLibrary> library = [device newLibraryWithURL:url error:&error];
        if (!library) {
            std::fprintf(stderr, "Metal library load failed: %s\n", [[error localizedDescription] UTF8String]);
            return 1;
        }
        std::ifstream input(argv[2]);
        if (!input) {
            std::fprintf(stderr, "Cannot open kernel list\n");
            return 2;
        }
        std::string name;
        unsigned count = 0;
        while (std::getline(input, name)) {
            if (name.empty()) continue;
            id<MTLFunction> function = [library newFunctionWithName:[NSString stringWithUTF8String:name.c_str()]];
            if (!function) {
                std::fprintf(stderr, "Missing Metal kernel: %s\n", name.c_str());
                return 1;
            }
            error = nil;
            id<MTLComputePipelineState> pipeline = [device newComputePipelineStateWithFunction:function error:&error];
            if (!pipeline) {
                std::fprintf(stderr, "Pipeline creation failed for %s: %s\n", name.c_str(), [[error localizedDescription] UTF8String]);
                return 1;
            }
            std::printf("PASS native Metal pipeline %s (max threads %lu, static threadgroup bytes %lu)\n", name.c_str(),
                        (unsigned long)[pipeline maxTotalThreadsPerThreadgroup], (unsigned long)[pipeline staticThreadgroupMemoryLength]);
            ++count;
        }
        if (input.bad() || count == 0) {
            std::fprintf(stderr, "Invalid or empty kernel list\n");
            return 2;
        }
        std::printf("PASS created %u host-referenced Metal pipelines\n", count);
    }
    return 0;
}
