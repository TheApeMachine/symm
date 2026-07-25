/*
manifold-parts holds the latest split manifold packets so fluid/meta painters
can compose gas, particles, and wave without one monolithic DRAW frame.
*/
let particlesPayload: Record<string, unknown> | null = null;
let wavePayload: Record<string, unknown> | null = null;

const asPacket = (value: unknown): Record<string, unknown> | null => {
	const candidate = Array.isArray(value) ? value[0] : value;

	if (candidate === null || typeof candidate !== "object") {
		return null;
	}

	return candidate as Record<string, unknown>;
};

export const paintManifoldParticles = (value: unknown) => {
	particlesPayload = asPacket(value);
};

export const paintManifoldWave = (value: unknown) => {
	wavePayload = asPacket(value);
};

export const latestManifoldParticles = (): Record<string, unknown> | null =>
	particlesPayload;

export const latestManifoldWave = (): Record<string, unknown> | null =>
	wavePayload;
