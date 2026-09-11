/* Shared Blelloch block core: tg_size must be a power of two; all lanes call. */
    uint idx = tg_id * tg_size + tid;
    shared[tid] = (idx < n) ? in[idx] : 0u;
    MS_SCAN_BARRIER();

    // Up-sweep
    for (uint stride = 1; stride < tg_size; stride <<= 1) {
        uint ai = ((tid + 1u) * stride * 2u) - 1u;
        if (ai < tg_size) shared[ai] += shared[ai - stride];
        MS_SCAN_BARRIER();
    }

    if (tid == tg_size - 1u) {
        block_sums[tg_id] = shared[tg_size - 1u];
        shared[tg_size - 1u] = 0u; // exclusive
    }
    MS_SCAN_BARRIER();

    // Down-sweep
    for (uint stride = tg_size >> 1; stride > 0; stride >>= 1) {
        uint ai = ((tid + 1u) * stride * 2u) - 1u;
        if (ai < tg_size) {
            uint t = shared[ai - stride];
            shared[ai - stride] = shared[ai];
            shared[ai] += t;
        }
        MS_SCAN_BARRIER();
    }

    if (idx < n) out[idx] = shared[tid];
