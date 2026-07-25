/*
manifold-parts holds the latest split manifold wave packet so the phase dial
can compose without one monolithic DRAW frame. Lattice textures arrive as
binary SMF1 frames; the oscillator cloud is not wired.
*/
const waveBySymbol: Record<string, Record<string, unknown>> = {};

const rowPacket = (value: unknown): Record<string, unknown> | null => {
	if (value === null || typeof value !== "object") {
		return null;
	}

	return value as Record<string, unknown>;
};

const symbolKey = (packet: Record<string, unknown>): string =>
	typeof packet.symbol === "string" ? packet.symbol : "";

const clearStore = (store: Record<string, Record<string, unknown>>) => {
	for (const key of Object.keys(store)) {
		delete store[key];
	}
};

const ingestPackets = (
	store: Record<string, Record<string, unknown>>,
	value: unknown,
) => {
	if (value === null || value === undefined) {
		clearStore(store);
		return;
	}

	const rows = Array.isArray(value) ? value : [value];

	if (rows.length === 0) {
		clearStore(store);
		return;
	}

	for (const row of rows) {
		const packet = rowPacket(row);

		if (packet === null) {
			continue;
		}

		store[symbolKey(packet)] = packet;
	}
};

export const paintManifoldWave = (value: unknown) => {
	ingestPackets(waveBySymbol, value);
};

export const latestManifoldWave = (
	symbol = "",
): Record<string, unknown> | null => waveBySymbol[symbol] ?? null;
