import type { ManifoldFieldSnapshot } from "#/components/charts/manifold/types";

export type ManifoldPushBridge = {
	push: (frame: ManifoldFieldSnapshot) => void;
	ready: boolean;
	pending: ManifoldFieldSnapshot | null;
};

export const createManifoldPushBridge = (): ManifoldPushBridge => ({
	push: () => {},
	ready: false,
	pending: null,
});

export const isManifoldSnapshot = (
	raw: unknown,
): raw is ManifoldFieldSnapshot => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return (
		row.type === "manifold" &&
		Array.isArray(row.rho) &&
		(row.rho as unknown[]).length > 0 &&
		typeof row.reading === "object" &&
		row.reading !== null
	);
};

export const ingestManifoldWire = (
	bridge: ManifoldPushBridge | null | undefined,
	raw: unknown,
): void => {
	if (!bridge || !isManifoldSnapshot(raw)) {
		return;
	}

	if (bridge.ready) {
		bridge.push(raw);
		return;
	}

	bridge.pending = raw;
};

export const attachManifoldPush = (
	bridge: ManifoldPushBridge,
	push: (frame: ManifoldFieldSnapshot) => void,
) => {
	bridge.push = push;
	bridge.ready = true;

	if (bridge.pending !== null) {
		push(bridge.pending);
	}

	bridge.pending = null;
};

export const detachManifoldPush = (bridge: ManifoldPushBridge) => {
	bridge.push = () => {};
	bridge.ready = false;
};
