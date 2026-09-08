#pragma once
#include <cuda_runtime.h>
#include <cmath>

// Small, private componentwise helpers for CUDA's native vector structs. Arrays
// at the boundary remain tightly packed scalar float/uint buffers.
namespace sensorium::kernels {
#define HD __host__ __device__ __forceinline__
HD float2 make_float2(float a) { return ::make_float2(a, a); }
HD float3 make_float3(float a) { return ::make_float3(a, a, a); }
HD float3 make_float3(uint3 a) { return ::make_float3(float(a.x), float(a.y), float(a.z)); }
HD float3 make_float3(int3 a) { return ::make_float3(float(a.x), float(a.y), float(a.z)); }
HD float4 make_float4(float a) { return ::make_float4(a, a, a, a); }
HD uint3 make_uint3(float3 a) { return ::make_uint3(unsigned(a.x), unsigned(a.y), unsigned(a.z)); }
HD uint3 make_uint3(int3 a) { return ::make_uint3(unsigned(a.x), unsigned(a.y), unsigned(a.z)); }
HD int3 make_int3(uint3 a) { return ::make_int3(int(a.x), int(a.y), int(a.z)); }
using ::make_float2;
using ::make_float3;
using ::make_float4;
using ::make_int3;
using ::make_uint3;

HD float3 operator+(float3 a, float3 b) { return ::make_float3(a.x+b.x, a.y+b.y, a.z+b.z); }
HD float3 operator-(float3 a, float3 b) { return ::make_float3(a.x-b.x, a.y-b.y, a.z-b.z); }
HD float3 operator*(float3 a, float3 b) { return ::make_float3(a.x*b.x, a.y*b.y, a.z*b.z); }
HD float3 operator/(float3 a, float3 b) { return ::make_float3(a.x/b.x, a.y/b.y, a.z/b.z); }
HD float3 operator-(float3 a) { return ::make_float3(-a.x, -a.y, -a.z); }
HD float3 operator+(float3 a, float b) { return a+make_float3(b); }
HD float3 operator-(float3 a, float b) { return a-make_float3(b); }
HD float3 operator*(float3 a, float b) { return ::make_float3(a.x*b, a.y*b, a.z*b); }
HD float3 operator*(float a, float3 b) { return b*a; }
HD float3 operator/(float3 a, float b) { return ::make_float3(a.x/b, a.y/b, a.z/b); }
HD float3& operator+=(float3& a, float3 b) { a=a+b; return a; }
HD float4& operator+=(float4& a, float4 b) { a.x+=b.x; a.y+=b.y; a.z+=b.z; a.w+=b.w; return a; }
HD int3 operator+(int3 a, int3 b) { return ::make_int3(a.x+b.x,a.y+b.y,a.z+b.z); }
HD uint3 operator-(uint3 a, unsigned b) { return ::make_uint3(a.x-b,a.y-b,a.z-b); }
HD uint3 min(uint3 a, uint3 b) { return ::make_uint3(::min(a.x,b.x),::min(a.y,b.y),::min(a.z,b.z)); }
using ::min;
using ::max;
HD float dot(float3 a,float3 b) { return a.x*b.x+a.y*b.y+a.z*b.z; }
HD float length(float3 a) { return sqrtf(dot(a,a)); }
HD float floor_value(float a) { return floorf(a); }
HD float3 floor_value(float3 a) { return ::make_float3(floorf(a.x),floorf(a.y),floorf(a.z)); }
#undef HD
} // namespace sensorium::kernels
