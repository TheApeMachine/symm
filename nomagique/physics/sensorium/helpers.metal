
// Helper for integer wrapping
inline int wrap_i32(int v, int dim) {
    int r = v % dim;
    return (r < 0) ? r + dim : r;
}

// Wrapper for trilinear sampling from position
inline float sample_trilinear(
    device const float* field,
    float3 pos,
    uint gx, uint gy, uint gz,
    float spacing,
    float inv_spacing
) {
    uint3 base; float3 frac;
    trilinear_coords(pos, inv_spacing, uint3(gx, gy, gz), base, frac);
    return sample_field_trilinear(field, base, frac, uint3(gx, gy, gz));
}

// Wrapper for trilinear gradient sampling from position
inline float3 sample_gradient_trilinear_pos(
    device const float* field,
    float3 pos,
    uint gx, uint gy, uint gz,
    float spacing,
    float inv_spacing
) {
    uint3 base; float3 frac;
    trilinear_coords(pos, inv_spacing, uint3(gx, gy, gz), base, frac);
    return sample_gradient_trilinear(field, base, frac, uint3(gx, gy, gz), inv_spacing);
}

// Overload to match call signature if needed (careful with ambiguity)
inline float3 sample_gradient_trilinear(
    device const float* field,
    float3 pos,
    uint gx, uint gy, uint gz,
    float spacing,
    float inv_spacing
) {
    return sample_gradient_trilinear_pos(field, pos, gx, gy, gz, spacing, inv_spacing);
}
