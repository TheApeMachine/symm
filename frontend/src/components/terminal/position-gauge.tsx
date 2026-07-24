import type { Holding, Stop } from "#/collections/types";
import { fixed } from "#/components/terminal/decision-format";
import { buildPositionGaugePanel } from "#/components/terminal/position-gauge-shell";
import {
	positionGaugeGeometry,
	positionStop,
} from "#/components/terminal/position-geometry";

export type { PriceGaugeGeometry } from "#/components/terminal/position-geometry";
export { positionGaugeGeometry } from "#/components/terminal/position-geometry";

type PositionGaugeParts = {
	quote: string;
	track: HTMLDivElement | null;
	progress: HTMLDivElement | null;
	stopMarker: HTMLDivElement | null;
	peakMarker: HTMLDivElement | null;
	entryMarker: HTMLDivElement | null;
	markMarker: HTMLDivElement | null;
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

const lastHoldings: Record<string, Holding | undefined> = {};
const lastStops: Record<string, Stop | undefined> = {};

const upTone = "var(--up)";
const downTone = "var(--down)";
const neutralTone = "var(--f3)";
const warnTone = "var(--warn)";
const accentTone = "var(--acc)";

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
	position: Holding | undefined,
	legacyStop: Stop | undefined,
) => {
	if (!position) {
		return;
	}

	const stop = positionStop(position, legacyStop);
	// Color each figure by its own sign — fee-dragged PnL can be red while
	// return_pct is green when price lifted but fees dominate dollars.
	const pnlTone =
		position.pnl > 0 ? upTone : position.pnl < 0 ? downTone : neutralTone;
	const returnTone =
		position.return_pct > 0
			? upTone
			: position.return_pct < 0
				? downTone
				: neutralTone;
	const geometry = positionGaugeGeometry(position, stop);
	const rawMark =
		Number.isFinite(position.mark) && position.mark > 0 ? position.mark : null;
	const markLabel = rawMark === null ? "--" : fixed(rawMark);
	const progressTone =
		geometry !== null &&
		geometry.stopPct !== null &&
		geometry.markPct >= geometry.stopPct
			? upTone
			: downTone;
	const hasMomentum = stop?.momentum_active;
	const health = hasMomentum
		? Math.max(0, Math.min(1, stop.momentum_health ?? 0))
		: 0;
	const momentumTone =
		health > 0.5 ? upTone : health > 0.2 ? warnTone : downTone;
	const hasStagnation = stop?.stagnation_active;
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
		parts.pnl.textContent = `P/L ${position.pnl.toFixed(4)} ${parts.quote}`;
	}

	if (parts.summary) {
		const stopSuffix =
			stop !== undefined && geometry !== null && geometry.stopPct !== null
				? ` / stop ${fixed(stop.stop_price)}`
				: "";
		parts.summary.textContent = `entry ${fixed(position.entry_price)} / mark ${markLabel}${stopSuffix}`;
	}

	if (parts.returnPct) {
		parts.returnPct.style.color = returnTone;
		const returnPct = Number.isFinite(position.return_pct)
			? position.return_pct
			: null;
		parts.returnPct.textContent =
			returnPct === null ? "—" : `${(returnPct * 100).toFixed(4)}%`;
	}

	if (parts.momentumWrap && parts.momentumBar) {
		parts.momentumWrap.style.display = hasMomentum ? "" : "none";
		parts.momentumBar.style.width = `${health * 100}%`;
		parts.momentumBar.style.background = momentumTone;
	}

	if (parts.stagnationWrap && parts.stagnationBar && parts.stagnationFlash) {
		parts.stagnationWrap.style.display = hasStagnation ? "" : "none";

		if (hasStagnation) {
			const stagnationHealth = Math.max(
				0,
				Math.min(1, stop.stagnation_health ?? 0),
			);
			const stagnationTone = stop.stagnation_pending
				? accentTone
				: stagnationHealth > 0.5
					? upTone
					: stagnationHealth > 0.2
						? warnTone
						: downTone;

			parts.stagnationBar.style.width = `${(stagnationHealth * 100).toFixed(0)}%`;
			parts.stagnationBar.style.background = stagnationTone;
			parts.stagnationFlash.style.display = stop.stagnation_pending
				? ""
				: "none";
		}
	}
};

const paintBound = (symbol: string) => {
	const parts = positionGauges.get(symbol);

	if (parts === undefined) {
		return;
	}

	writePositionGauge(parts, lastHoldings[symbol], lastStops[symbol]);
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
paintPositionHoldings merges the DRAW holdings batch into lastHoldings and
repaints every bound position gauge whose symbol appears in the batch.
*/
export const paintPositionHoldings = (value: unknown, _focusSymbol: string) => {
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as Holding[];

	for (const row of rows) {
		lastHoldings[row.symbol] = row;
		paintBound(row.symbol);
	}
};

/*
paintPositionStops merges the DRAW stops batch into lastStops and repaints
matching bound position gauge shells.
*/
export const paintPositionStops = (value: unknown, _focusSymbol: string) => {
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as Stop[];

	for (const row of rows) {
		lastStops[row.symbol] = row;
		paintBound(row.symbol);
	}
};
