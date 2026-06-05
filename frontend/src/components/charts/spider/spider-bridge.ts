export const REGIME_CHART_SYMBOL = "BTC/EUR";

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
parseRegimeWire accepts only anchor-symbol regime frames so the radar is not
overwritten by every symbol's classification tick.
*/
export const parseRegimeWire = (
	raw: Record<string, unknown>,
	axes: readonly string[],
	anchorSymbol = REGIME_CHART_SYMBOL,
): Record<string, number> | null => {
	if (raw.chart !== "regime") {
		return null;
	}

	const symbol = raw.symbol;

	if (typeof symbol === "string" && symbol !== anchorSymbol) {
		return null;
	}

	return regimeAxisValues(raw, axes);
};

export const deliverRegimeWire = (
	bridge: SpiderBridge | null | undefined,
	values: Record<string, number>,
) => {
	if (!bridge) {
		return;
	}

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
