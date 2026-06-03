export type FluidPushBridge = {
	push: (raw: unknown) => void;
	ready: boolean;
	pending: unknown[];
};

export const createFluidPushBridge = (): FluidPushBridge => ({
	push: () => {},
	ready: false,
	pending: [],
});

export const deliverFluidWire = (
	bridge: FluidPushBridge,
	raw: unknown,
) => {
	if (bridge.ready) {
		bridge.push(raw);
		return;
	}

	bridge.pending.push(raw);
};

export const attachFluidPush = (
	bridge: FluidPushBridge,
	push: (raw: unknown) => void,
) => {
	bridge.push = push;
	bridge.ready = true;

	for (const frame of bridge.pending) {
		push(frame);
	}

	bridge.pending = [];
};

export const detachFluidPush = (bridge: FluidPushBridge) => {
	bridge.push = () => {};
	bridge.ready = false;
	bridge.pending = [];
};
