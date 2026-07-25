/*
manifold-parts holds the latest split manifold packets so fluid/meta painters
can compose gas, particles, and wave without one monolithic DRAW frame.
*/
let particlesPayload: Record<string, unknown> | null = null;
let wavePayload: Record<string, unknown> | null = null;

export const paintManifoldParticles = (value: unknown) => {
	if (value !== null && typeof value === "object" && !Array.isArray(value)) {
		particlesPayload = value as Record<string, unknown>;
		return;
	}

	particlesPayload = null;
};

export const paintManifoldWave = (value: unknown) => {
	if (value !== null && typeof value === "object" && !Array.isArray(value)) {
		wavePayload = value as Record<string, unknown>;
		return;
	}

	wavePayload = null;
};

export const latestManifoldParticles = (): Record<string, unknown> | null =>
	particlesPayload;

export const latestManifoldWave = (): Record<string, unknown> | null =>
	wavePayload;
