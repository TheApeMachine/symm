export const REGIME_MARKET_SYMBOL = "market";

export type SpiderBridge = {
	setAll: (values: Record<string, number>) => void;
	ready: boolean;
	pending: Record<string, number> | null;
	latest: Record<string, number>;
};

export const createSpiderBridge = (): SpiderBridge => ({
	setAll: () => {},
	ready: false,
	pending: null,
	latest: {},
});

export const scaleSpiderRadarValues = (
	sources: readonly string[],
	values: Record<string, number>,
): number[] => sources.map((axis) => (values[axis] ?? 0) * 100);

const regimeAxisValues = (
	raw: Record<string, unknown>,
	axes: readonly string[],
): Record<string, number> =>
	Object.fromEntries(axes.map((axis) => [axis, (raw[axis] as number) ?? 0]));

/*
ingestRegimeWire accepts only the cross-section market regime frame so per-symbol
classifications cannot overwrite the averaged radar shape.
*/
export const ingestRegimeWire = (
	bridge: SpiderBridge | null | undefined,
	raw: Record<string, unknown>,
	axes: readonly string[],
	marketSymbol = REGIME_MARKET_SYMBOL,
): void => {
	if (!bridge || raw.chart !== "regime") {
		return;
	}

	const symbol = raw.symbol;

	if (typeof symbol === "string" && symbol !== marketSymbol) {
		return;
	}

	const values = regimeAxisValues(raw, axes);

	Object.assign(bridge.latest, values);

	if (bridge.ready) {
		bridge.setAll(values);
		return;
	}

	bridge.pending = { ...bridge.latest };
};

export const attachSpiderBridge = (
	bridge: SpiderBridge,
	setAll: (values: Record<string, number>) => void,
) => {
	bridge.setAll = setAll;
	bridge.ready = true;

	const snapshot =
		bridge.pending !== null ? bridge.pending : { ...bridge.latest };

	if (Object.keys(snapshot).length > 0) {
		setAll(snapshot);
	}

	bridge.pending = null;
};

export const detachSpiderBridge = (bridge: SpiderBridge) => {
	bridge.setAll = () => {};
	bridge.ready = false;
};
