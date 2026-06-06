import type { FieldSnapshotEvent } from "#/components/charts/fluid/types";

export type FluidPushBridge = {
	push: (raw: unknown) => void;
	ready: boolean;
	pending: FieldSnapshotEvent | null;
};

export const createFluidPushBridge = (): FluidPushBridge => ({
	push: () => {},
	ready: false,
	pending: null,
});

export const parseFluidWire = (
	raw: Record<string, unknown>,
): FieldSnapshotEvent | null => {
	if (raw.type !== "fluid" || !Array.isArray(raw.symbols)) {
		return null;
	}

	if (raw.symbols.length === 0) {
		return null;
	}

	return raw as FieldSnapshotEvent;
};

export const deliverFluidWire = (
	bridge: FluidPushBridge | null | undefined,
	frame: FieldSnapshotEvent,
) => {
	if (!bridge) {
		return;
	}

	if (bridge.ready) {
		bridge.push(frame);
		return;
	}

	bridge.pending = frame;
};

export const attachFluidPush = (
	bridge: FluidPushBridge,
	push: (raw: unknown) => void,
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
