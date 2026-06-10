import { parseManifoldSnapshot } from "#/components/charts/manifold/manifold-snapshot";
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
): raw is ManifoldFieldSnapshot => parseManifoldSnapshot(raw) !== null;

export const ingestManifoldWire = (
	bridge: ManifoldPushBridge | null | undefined,
	raw: unknown,
): void => {
	const frame = parseManifoldSnapshot(raw);

	if (!bridge || frame === null) {
		return;
	}

	if (bridge.ready) {
		bridge.push(frame);
		return;
	}

	bridge.pending = frame;
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
