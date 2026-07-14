import { memo, useRef } from "react";
import { type Position, positionsStore } from "#/collections/positions";
import { type Stop, stopsStore } from "#/collections/stops";
import { terminalStore } from "#/collections/terminal";
import { fixed } from "#/components/terminal/decision-format";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";

import { Panel } from "@/components/ui/panel";

const clampPercent = (value: number, lo: number, hi: number): number => {
	if (!(hi > lo)) {
		return 50;
	}

	return Math.min(100, Math.max(0, ((value - lo) / (hi - lo)) * 100));
};

const positiveFinite = (value: number): number | null =>
	Number.isFinite(value) && value > 0 ? value : null;

export type PriceGaugeGeometry = {
	entryPct: number;
	markPct: number;
	stopPct: number | null;
	peakPct: number | null;
	rawMarkPrice: number | null;
};

export const positionGaugeGeometry = (
	position: Position,
	stop?: Stop,
): PriceGaugeGeometry | null => {
	const entry = positiveFinite(position.entry_price);

	if (entry === null) {
		return null;
	}

	const rawMark = positiveFinite(position.mark);
	const derivedMark =
		rawMark ??
		(Number.isFinite(position.return_pct) && position.return_pct > -1
			? positiveFinite(entry * (1 + position.return_pct))
			: null);
	const markReturn = derivedMark === null ? 0 : derivedMark / entry - 1;
	const hasStop =
		stop !== undefined &&
		positiveFinite(stop.stop_price) !== null &&
		Number.isFinite(stop.stop_return) &&
		Number.isFinite(stop.peak_return);

	if (!hasStop) {
		const lo = Math.min(0, markReturn);
		const hi = Math.max(0, markReturn);

		if (!(hi > lo)) {
			return null;
		}

		const pad = (hi - lo) * 0.15;
		const domainLo = lo - pad;
		const domainHi = hi + pad;

		return {
			entryPct: clampPercent(0, domainLo, domainHi),
			markPct: clampPercent(markReturn, domainLo, domainHi),
			stopPct: null,
			peakPct: null,
			rawMarkPrice: rawMark,
		};
	}

	const stopReturn = stop.stop_return;
	const peakReturn = stop.peak_return;
	const trailSpan = peakReturn - stopReturn;
	const rawLo = Math.min(0, stopReturn, markReturn);
	const rawHi = Math.max(0, peakReturn, markReturn);
	const padding = trailSpan > 0 ? trailSpan : rawHi - rawLo;

	if (!(padding > 0)) {
		return null;
	}

	const lo = rawLo - padding;
	const hi = rawHi + padding;

	return {
		entryPct: clampPercent(0, lo, hi),
		markPct: clampPercent(markReturn, lo, hi),
		stopPct: clampPercent(stopReturn, lo, hi),
		peakPct: clampPercent(peakReturn, lo, hi),
		rawMarkPrice: rawMark,
	};
};

type PositionGaugeRefs = {
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

const paintPositionGauge = (
	refs: PositionGaugeRefs,
	symbol: string,
	quote: string,
) => {
	const position = positionsStore.state.positions.find(
		(candidate) => candidate.symbol === symbol,
	);

	if (!position) {
		return;
	}

	const stop = stopsStore.state.stops[symbol];
	const profitable = position.pnl > 0 || position.return_pct > 0;
	const pnlTone = profitable
		? upTone
		: position.pnl < 0 || position.return_pct < 0
			? downTone
			: neutralTone;

	const geometry = positionGaugeGeometry(position, stop);
	const rawMark = positiveFinite(position.mark);
	const markLabel = rawMark === null ? "--" : fixed(rawMark);
	const progressTone =
		geometry !== null &&
		geometry.stopPct !== null &&
		geometry.markPct >= geometry.stopPct
			? upTone
			: downTone;

	const hasMomentum = stop?.momentum_active;
	const health = hasMomentum
		? Math.max(0, Math.min(1, stop.momentum_health))
		: 0;
	const momentumTone =
		health > 0.5 ? upTone : health > 0.2 ? warnTone : downTone;

	const hasStagnation = stop?.stagnation_active;
	const showGauge = geometry !== null;

	if (refs.track) {
		refs.track.style.display = showGauge ? "" : "none";
	}

	if (showGauge && geometry !== null) {
		setMarkerPosition(
			refs.stopMarker,
			geometry.stopPct,
			geometry.stopPct !== null,
		);
		setMarkerPosition(
			refs.peakMarker,
			geometry.peakPct,
			geometry.peakPct !== null,
		);
		setMarkerPosition(refs.entryMarker, geometry.entryPct, true);
		setMarkerPosition(refs.markMarker, geometry.markPct, true);

		if (refs.progress) {
			if (geometry.stopPct !== null) {
				const progressLo = Math.min(geometry.stopPct, geometry.markPct);
				const progressHi = Math.max(geometry.stopPct, geometry.markPct);

				refs.progress.style.display = "";
				refs.progress.style.left = `${progressLo}%`;
				refs.progress.style.width = `${Math.max(0, progressHi - progressLo)}%`;
				refs.progress.style.background = `color-mix(in srgb, ${progressTone} 18%, transparent)`;
			} else {
				refs.progress.style.display = "none";
			}
		}

		if (refs.markMarker) {
			refs.markMarker.style.background = `color-mix(in srgb, ${pnlTone} 72%, var(--f1))`;
		}
	}

	if (refs.pnl) {
		refs.pnl.style.color = pnlTone;
		refs.pnl.textContent = `P/L ${position.pnl.toFixed(4)} ${quote}`;
	}

	if (refs.summary) {
		const stopSuffix =
			stop !== undefined && geometry !== null && geometry.stopPct !== null
				? ` / stop ${fixed(stop.stop_price)}`
				: "";
		refs.summary.textContent = `entry ${fixed(position.entry_price)} / mark ${markLabel}${stopSuffix}`;
	}

	if (refs.returnPct) {
		refs.returnPct.style.color = pnlTone;
		refs.returnPct.textContent = `${position.return_pct.toFixed(4)}%`;
	}

	if (refs.momentumWrap && refs.momentumBar) {
		refs.momentumWrap.style.display = hasMomentum ? "" : "none";
		refs.momentumBar.style.width = `${health * 100}%`;
		refs.momentumBar.style.background = momentumTone;
	}

	if (refs.stagnationWrap && refs.stagnationBar && refs.stagnationFlash) {
		refs.stagnationWrap.style.display = hasStagnation ? "" : "none";

		if (hasStagnation) {
			const stagnationHealth = Math.max(0, Math.min(1, stop.stagnation_health));
			const stagnationTone = stop.stagnation_pending
				? accentTone
				: stagnationHealth > 0.5
					? upTone
					: stagnationHealth > 0.2
						? warnTone
						: downTone;

			refs.stagnationBar.style.width = `${(stagnationHealth * 100).toFixed(0)}%`;
			refs.stagnationBar.style.background = stagnationTone;
			refs.stagnationFlash.style.display = stop.stagnation_pending
				? ""
				: "none";
		}
	}
};

export const PositionGauge = memo(
	({ symbol, quote }: { symbol: string; quote: string }) => {
		const trackRef = useRef<HTMLDivElement>(null);
		const progressRef = useRef<HTMLDivElement>(null);
		const stopMarkerRef = useRef<HTMLDivElement>(null);
		const peakMarkerRef = useRef<HTMLDivElement>(null);
		const entryMarkerRef = useRef<HTMLDivElement>(null);
		const markMarkerRef = useRef<HTMLDivElement>(null);
		const pnlRef = useRef<HTMLSpanElement>(null);
		const summaryRef = useRef<HTMLSpanElement>(null);
		const returnRef = useRef<HTMLSpanElement>(null);
		const momentumWrapRef = useRef<HTMLDivElement>(null);
		const momentumBarRef = useRef<HTMLDivElement>(null);
		const stagnationWrapRef = useRef<HTMLDivElement>(null);
		const stagnationBarRef = useRef<HTMLDivElement>(null);
		const stagnationFlashRef = useRef<HTMLSpanElement>(null);

		useDirectStorePaint(
			() =>
				paintPositionGauge(
					{
						track: trackRef.current,
						progress: progressRef.current,
						stopMarker: stopMarkerRef.current,
						peakMarker: peakMarkerRef.current,
						entryMarker: entryMarkerRef.current,
						markMarker: markMarkerRef.current,
						pnl: pnlRef.current,
						summary: summaryRef.current,
						returnPct: returnRef.current,
						momentumWrap: momentumWrapRef.current,
						momentumBar: momentumBarRef.current,
						stagnationWrap: stagnationWrapRef.current,
						stagnationBar: stagnationBarRef.current,
						stagnationFlash: stagnationFlashRef.current,
					},
					symbol,
					quote,
				),
			[positionsStore, stopsStore],
			[symbol, quote],
		);

		return (
			<Panel
				data-symbol={symbol}
				size="s"
				className="mb-[5px] cursor-pointer font-mono text-[11px] transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)]"
				onClick={() => terminalStore.actions.openThesis(symbol)}
				onKeyDown={(event) => {
					if (event.key === "Enter" || event.key === " ") {
						event.preventDefault();
						terminalStore.actions.openThesis(symbol);
					}
				}}
				role="button"
				tabIndex={0}
				aria-label={`Open thesis for ${symbol}`}
			>
				<div className="flex items-start justify-between gap-3">
					<span className="font-semibold text-(--f1)">{symbol}</span>
					<span ref={pnlRef} className="text-right font-semibold" />
				</div>
				<div className="mt-1 flex items-center justify-between gap-3 text-[10px] text-(--f4)">
					<span ref={summaryRef} />
					<span ref={returnRef} />
				</div>

				<div
					ref={trackRef}
					className="relative mt-1.5 h-[3px] overflow-visible rounded-full bg-[color-mix(in_srgb,var(--f4)_18%,transparent)]"
				>
					<div
						ref={progressRef}
						className="pointer-events-none absolute inset-y-0 rounded-full"
					/>
					<div
						ref={stopMarkerRef}
						className="pointer-events-none absolute top-1/2 h-2 w-px -translate-x-1/2 -translate-y-1/2"
						style={{
							background: "color-mix(in srgb, var(--down) 42%, transparent)",
						}}
						title="Stop"
					/>
					<div
						ref={peakMarkerRef}
						className="pointer-events-none absolute top-1/2 h-2 w-px -translate-x-1/2 -translate-y-1/2"
						style={{
							background: "color-mix(in srgb, var(--up) 42%, transparent)",
						}}
						title="Peak"
					/>
					<div
						ref={entryMarkerRef}
						className="pointer-events-none absolute top-1/2 h-1.5 w-px -translate-x-1/2 -translate-y-1/2"
						style={{
							background: "color-mix(in srgb, var(--f4) 38%, transparent)",
						}}
						title="Entry"
					/>
					<div
						ref={markMarkerRef}
						className="pointer-events-none absolute top-1/2 h-[7px] w-[7px] -translate-x-1/2 -translate-y-1/2 rounded-full border border-[color-mix(in_srgb,var(--surface)_70%,transparent)]"
						title="Mark"
					/>
				</div>

				<div ref={momentumWrapRef} className="mt-1.5 flex items-center gap-1.5">
					<span className="text-[8px] text-(--f4) uppercase tracking-wide">
						mom
					</span>
					<div className="h-[3px] flex-1 overflow-hidden rounded-full bg-[color-mix(in_srgb,var(--f4)_25%,transparent)]">
						<div
							ref={momentumBarRef}
							className="h-full rounded-full transition-[width]"
						/>
					</div>
				</div>

				<div
					ref={stagnationWrapRef}
					className="mt-1.5 flex items-center gap-1.5"
				>
					<span className="text-[8px] text-(--f4) uppercase tracking-wide">
						stall
					</span>
					<div className="h-[3px] flex-1 overflow-hidden rounded-full bg-[color-mix(in_srgb,var(--f4)_25%,transparent)]">
						<div
							ref={stagnationBarRef}
							className="h-full rounded-full transition-[width]"
						/>
					</div>
					<span
						ref={stagnationFlashRef}
						className="text-[8px] font-semibold text-(--acc) uppercase tracking-wide"
					>
						⚡
					</span>
				</div>
			</Panel>
		);
	},
);
