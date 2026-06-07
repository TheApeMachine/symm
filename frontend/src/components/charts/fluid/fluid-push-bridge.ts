import type { FieldSnapshotEvent } from "#/components/charts/fluid/types";

export type FluidPushBridge = {
	push: (frame: FieldSnapshotEvent) => void;
	ready: boolean;
	pending: FieldSnapshotEvent | null;
};

export const createFluidPushBridge = (): FluidPushBridge => ({
	push: () => {},
	ready: false,
	pending: null,
});

export const isFieldSnapshot = (raw: unknown): raw is FieldSnapshotEvent => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return (
		row.type === "fluid" && Array.isArray(row.symbols) && row.symbols.length > 0
	);
};

export const ingestFluidWire = (
	bridge: FluidPushBridge | null | undefined,
	raw: unknown,
): void => {
	if (!bridge || !isFieldSnapshot(raw)) {
		return;
	}

	if (bridge.ready) {
		bridge.push(raw);
		return;
	}

	bridge.pending = raw;
};

export const attachFluidPush = (
	bridge: FluidPushBridge,
	push: (frame: FieldSnapshotEvent) => void,
) => {
	bridge.push = push;
	bridge.ready = true;

	if (bridge.pending !== null) {
		push(bridge.pending);
	}

	bridge.pending = null;
};

export const detachFluidPush = (bridge: FluidPushBridge) => {
	bridge.push = () => {};
	bridge.ready = false;
};
