// Included inside namespace sensorium::kernels.
__global__ void coherence_gpe_fft_fused(
    float* mode_real, float* mode_imag, CarrierAccumulators* accums,
    const uint* num_modes_in, CoherenceModeParams p, GPEParams gp,
    float* kinetic_real, float* kinetic_imag, uint scratch_complex, float* ledger, const float* extra_potential, const float* metric_volume, uint geometry_flags);
__global__ void coherence_gpe_fft_fused_global(
    float* mode_real, float* mode_imag, CarrierAccumulators* accums,
    const uint* num_modes_in, CoherenceModeParams p, GPEParams gp,
    float* kinetic_real, float* kinetic_imag, uint scratch_complex, void* global_scratch, float* ledger, const float* extra_potential, const float* metric_volume, uint geometry_flags);
