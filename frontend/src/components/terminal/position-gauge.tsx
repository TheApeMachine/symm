import type { Position, Stoploss } from "#/collections/types";
import { fixed } from "#/components/terminal/decision-format";
import { buildPositionGaugePanel } from "#/components/terminal/position-gauge-shell";

type PositionGaugeParts = {
	quote: string;
	track: HTMLDivElement | null;
	progress: HTMLDivElement | null;
	stopMarker: HTMLDivElement | null;
	peakMarker: HTMLDivElement | null;
	entryMarker: HTMLDivElement | null;
	markMarker: HTMLDivElement | null;
	floorLabel: HTMLSpanElement | null;
	peakLabel: HTMLSpanElement | null;
	markLabel: HTMLSpanElement | null;
	pnl: HTMLSpanElement | null;
	summary: HTMLSpanElement | null;
	returnPct: HTMLSpanElement | null;
	momentumWrap: HTMLDivElement | null;
	momentumBar: HTMLDivElement | null;
	stagnationWrap: HTMLDivElement | null;
	stagnationBar: HTMLDivElement | null;
	stagnationFlash: HTMLSpanElement | null;
};

export const positionGauges = new Map<string, PositionGaugeParts>();

const lastPositions: Record<string, Position | undefined> = {};
const lastStops: Record<string, Stoploss | undefined> = {};

const upTone = "var(--up)";
const downTone = "var(--down)";
const neutralTone = "var(--f3)";
const warnTone = "var(--warn)";
const accentTone = "var(--acc)";

const numberValue = (value: number | string | undefined): number | null => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : null;
};

type GaugeGeometry = {
	entryPct: number;
	markPct: number;
	stopPct: number | null;
	peakPct: number | null;
};

const clampPercent = (value: number, lo: number, hi: number): number => {
	if (!(hi > lo)) {
		return 50;
	}

	return Math.min(100, Math.max(0, ((value - lo) / (hi - lo)) * 100));
};

const gaugeGeometry = (
	entry: number | null,
	mark: number | null,
	floor: number | null,
	peak: number | null,
): GaugeGeometry | null => {
	if (entry === null || entry <= 0 || mark === null || mark <= 0) {
		return null;
	}

	const points = [entry, mark].filter((value): value is number => value !== null);

	if (floor !== null && floor > 0) {
		points.push(floor);
	}

	if (peak !== null && peak > 0) {
		points.push(peak);
	}

	const lo = Math.min(...points);
	const hi = Math.max(...points);

	if (!(hi > lo)) {
		return null;
	}

	const pad = Math.max((hi - lo) * 0.15, 1e-6);
	const domainLo = lo - pad;
	const domainHi = hi + pad;

	return {
		entryPct: clampPercent(entry, domainLo, domainHi),
		markPct: clampPercent(mark, domainLo, domainHi),
		stopPct: floor !== null && floor > 0 ? clampPercent(floor, domainLo, domainHi) : null,
		peakPct: peak !== null && peak > 0 ? clampPercent(peak, domainLo, domainHi) : null,
	};
};

export const positionGaugeGeometry = (
	holding: Position["holding"],
	stoploss?: Stoploss,
): GaugeGeometry | null =>
	gaugeGeometry(
		numberValue(holding?.entry_price),
		numberValue(holding?.mark),
		numberValue(stoploss?.floor),
		numberValue(stoploss?.peak),
	);

const setMarkerPosition = (
	element: HTMLElement | null,
	percent: number | null,
	visible: boolean,
) => {
	if (element === null) {
		return;
	}

	element.style.display = visible ? "" : "none";

	if (!visible || percent === null) {
		return;
	}

	element.style.left = `${percent}%`;
};

const writePositionGauge = (
	parts: PositionGaugeParts,
	position: Position | undefined,
) => {
	if (!position || !position.holding) {
		return;
	}

	const holding = position.holding;
	const stoploss = holding.stoploss ?? lastStops[holding.symbol];

	const pnl = numberValue(holding.pnl);
	const returnPct = numberValue(holding.return_pct);
	const mark = numberValue(holding.mark);
	const peak = numberValue(stoploss?.peak);
	const floor = numberValue(stoploss?.floor);
	const entry = numberValue(holding.entry_price);
	// Color each figure by its own sign — fee-dragged PnL can be red while
	// return_pct is green when price lifted but fees dominate dollars.
	const pnlTone =
		pnl === null ? neutralTone : pnl > 0 ? upTone : pnl < 0 ? downTone : neutralTone;
	const returnTone =
		returnPct !== null && returnPct > 0
			? upTone
			: returnPct !== null && returnPct < 0
				? downTone
				: neutralTone;

	const geometry = gaugeGeometry(entry, mark, floor, peak);
	const rawMark = mark !== null && mark > 0 ? mark : null;
	const markLabel = rawMark === null ? "--" : fixed(rawMark);
	const peakPrice = peak !== null && peak > 0 ? fixed(peak) : "--";
	const floorPrice = floor !== null && floor > 0 ? fixed(floor) : "--";

	const progressTone =
		geometry !== null &&
		geometry.stopPct !== null &&
		geometry.markPct >= geometry.stopPct
			? upTone
			: downTone;

			const showGauge = geometry !== null;

	if (parts.track) {
		parts.track.style.display = showGauge ? "" : "none";
	}

	if (showGauge && geometry !== null) {
		setMarkerPosition(
			parts.stopMarker,
			geometry.stopPct,
			geometry.stopPct !== null,
		);
		setMarkerPosition(
			parts.peakMarker,
			geometry.peakPct,
			geometry.peakPct !== null,
		);
		setMarkerPosition(parts.entryMarker, geometry.entryPct, true);
		setMarkerPosition(parts.markMarker, geometry.markPct, true);

		if (parts.progress) {
			if (geometry.stopPct !== null) {
				const progressLo = Math.min(geometry.stopPct, geometry.markPct);
				const progressHi = Math.max(geometry.stopPct, geometry.markPct);

				parts.progress.style.display = "";
				parts.progress.style.left = `${progressLo}%`;
				parts.progress.style.width = `${Math.max(0, progressHi - progressLo)}%`;
				parts.progress.style.background = `color-mix(in srgb, ${progressTone} 18%, transparent)`;
			} else {
				parts.progress.style.display = "none";
			}
		}

		if (parts.markMarker) {
			parts.markMarker.style.background = `color-mix(in srgb, ${pnlTone} 72%, var(--f1))`;
		}
	}

	if (parts.pnl) {
		parts.pnl.style.color = pnlTone;
		parts.pnl.textContent =
			pnl === null ? `P/L — ${parts.quote}` : `P/L ${pnl.toFixed(4)} ${parts.quote}`;
	}

	if (parts.summary) {
		parts.summary.textContent =
			entry === null
				? `entry — / mark ${markLabel}`
				: `entry ${fixed(entry)} / mark ${markLabel}`;
	}

	if (parts.floorLabel) {
		parts.floorLabel.textContent = `floor ${floorPrice}`;
		parts.floorLabel.style.color = downTone;
	}

	if (parts.peakLabel) {
		parts.peakLabel.textContent = "";
		parts.peakLabel.style.color = upTone;
	}

	if (parts.markLabel) {
		parts.markLabel.textContent = `peak ${peakPrice}`;
		parts.markLabel.style.color = upTone;
	}

	if (parts.returnPct) {
		parts.returnPct.style.color = returnTone;
		parts.returnPct.textContent =
			returnPct === null ? "—" : `${(returnPct * 100).toFixed(4)}%`;
	}

	if (parts.momentumWrap) {
		parts.momentumWrap.style.display = "none";
	}

	if (parts.stagnationWrap) {
		parts.stagnationWrap.style.display = "none";
	}

	if (parts.stagnationFlash) {
		parts.stagnationFlash.style.display = "none";
	}
};

const paintBound = (symbol: string) => {
	const parts = positionGauges.get(symbol);

	if (parts === undefined) {
		return;
	}

	writePositionGauge(parts, lastPositions[symbol]);
};

const bindGauge = (symbol: string, quote: string, root: HTMLElement | null) => {
	if (root === null) {
		positionGauges.delete(symbol);
		return;
	}

	positionGauges.set(symbol, {
		quote,
		track: root.querySelector('[data-gauge="track"]'),
		progress: root.querySelector('[data-gauge="progress"]'),
		stopMarker: root.querySelector('[data-gauge="stop"]'),
		peakMarker: root.querySelector('[data-gauge="peak"]'),
		entryMarker: root.querySelector('[data-gauge="entry"]'),
		markMarker: root.querySelector('[data-gauge="mark"]'),
		floorLabel: root.querySelector('[data-gauge="floor-label"]'),
		peakLabel: root.querySelector('[data-gauge="peak-label"]'),
		markLabel: root.querySelector('[data-gauge="mark-label"]'),
		pnl: root.querySelector('[data-gauge="pnl"]'),
		summary: root.querySelector('[data-gauge="summary"]'),
		returnPct: root.querySelector('[data-gauge="return"]'),
		momentumWrap: root.querySelector('[data-gauge="momentum-wrap"]'),
		momentumBar: root.querySelector('[data-gauge="momentum-bar"]'),
		stagnationWrap: root.querySelector('[data-gauge="stagnation-wrap"]'),
		stagnationBar: root.querySelector('[data-gauge="stagnation-bar"]'),
		stagnationFlash: root.querySelector('[data-gauge="stagnation-flash"]'),
	});
	paintBound(symbol);
};

/*
createPositionGaugeElement builds one open-lot shell and binds paint targets
into positionGauges for DRAW updates.
*/
export const createPositionGaugeElement = (
	symbol: string,
	quote: string,
): HTMLElement => {
	const root = document.createElement("div");
	root.append(buildPositionGaugePanel(symbol));
	bindGauge(symbol, quote, root);

	return root;
};

/*
removePositionGauge drops one symbol from the positionGauges paint registry.
*/
export const removePositionGauge = (symbol: string): void => {
	positionGauges.delete(symbol);
};

/*
 paintPositions merges the DRAW positions batch into lastPositions and repaints
 every bound position gauge whose symbol appears in the batch.
 */
export const paintPositions = (value: unknown, _focusSymbol: string) => {
	const rows = (Array.isArray(value)
		? value
		: value !== null && typeof value === "object"
			? Object.values(value as Record<string, Position>)
			: value != null
				? [value]
				: []) as Position[];

	for (const row of rows) {
		if (!row.holding) {
			continue
		}

		lastPositions[row.holding.symbol] = row;
		paintBound(row.holding.symbol);
	}
};

export const paintPositionHoldings = paintPositions;

/*
paintPositionStops merges the DRAW stops batch into lastStops and repaints
matching bound position gauge shells.
*/
export const paintPositionStops = (value: unknown, _focusSymbol: string) => {
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as Stoploss[];

	for (const row of rows) {
		lastStops[row.symbol] = row;
		paintBound(row.symbol);
	}
};
