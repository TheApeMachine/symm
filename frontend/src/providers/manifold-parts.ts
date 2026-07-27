/*
manifold-parts holds the latest split manifold wave packet so the phase dial
can compose without one monolithic DRAW frame. Lattice textures arrive as
binary SMF1 frames.
*/
type ManifoldWavePacket = Record<string, unknown> & {
	symbol?: string;
	wave?: Array<Record<string, unknown>>;
};

const waveBySymbol: Record<string, ManifoldWavePacket> = {};

const rowPacket = (value: unknown): ManifoldWavePacket | null => {
	if (value === null || typeof value !== "object") {
		return null;
	}

	return value as ManifoldWavePacket;
};

const symbolKey = (packet: ManifoldWavePacket): string =>
	typeof packet.symbol === "string" ? packet.symbol : "";

const clearStore = (store: Record<string, ManifoldWavePacket>) => {
	for (const key of Object.keys(store)) {
		delete store[key];
	}
};

const ingestPackets = (
	store: Record<string, ManifoldWavePacket>,
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

export const latestManifoldWave = (symbol = ""): ManifoldWavePacket | null =>
	waveBySymbol[symbol] ?? null;
