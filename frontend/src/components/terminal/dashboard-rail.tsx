import { useSelector } from "@tanstack/react-store";
import { memo, useLayoutEffect, useRef } from "react";
import { type Action, actionStore } from "#/collections/actions";
import { type Execution, executionsStore } from "#/collections/executions";
import { type Position, positionsStore } from "#/collections/positions";
import { type Stop, stopsStore } from "#/collections/stops";
import { ColumnHeader } from "#/components/dashboard/header";
import { fixed } from "#/components/terminal/decision-format";
import { cn } from "#/lib/utils";

const sameSymbols = (left: string[], right: string[]): boolean =>
	left.length === right.length &&
	left.every((symbol, index) => symbol === right[index]);

const DecisionRows = ({ decisions }: { decisions: Action[] }) => (
	<div className="min-h-0 flex-1 overflow-auto">
		<div className="grid grid-cols-[78px_58px_minmax(84px,1fr)_72px] gap-2 border-(--line) border-b px-3 py-2 font-mono text-[9px] text-(--f4) uppercase">
			<span>Symbol</span>
			<span className="text-right">Comb</span>
			<span className="text-right">Fraction</span>
			<span className="text-right">Action</span>
		</div>
		{decisions.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				waiting for decision frames
			</div>
		) : null}
		{decisions.map((decision) => {
			return (
				<div
					key={`${decision.symbol}:${decision.id}:${decision.tick}`}
					data-symbol={decision.symbol}
					className="grid grid-cols-[78px_58px_minmax(84px,1fr)_72px] gap-2 border-(--line) border-b px-3 py-2 font-mono text-[11px]"
				>
					<div className="min-w-0">
						<div className="truncate font-semibold text-(--f1)">
							{decision.symbol}
						</div>
						<div className="truncate text-[9px] text-(--f4)">
							{decision.type} / {decision.side}
						</div>
					</div>
					<span className="text-right text-(--f2)">
						{fixed(Number(decision.score))}
					</span>
					<span className="truncate text-right text-(--f2)">
						{(decision.fraction * 100).toFixed(2)}%
					</span>
					<span className="text-right">
						<span
							className={cn(
								"rounded-[2px] px-1.5 py-0.5 font-semibold text-[9px] uppercase",
								decision.side === "sell"
									? "bg-[color-mix(in_srgb,var(--down)_16%,transparent)] text-(--down)"
									: decision.verdict === "allow"
										? "bg-[color-mix(in_srgb,var(--up)_16%,transparent)] text-(--up)"
										: "bg-[color-mix(in_srgb,var(--down)_16%,transparent)] text-(--down)",
							)}
						>
							{decision.side}
						</span>
					</span>
				</div>
			);
		})}
	</div>
);

// clampPercent maps a value into the card's horizontal domain as a 0-100 percent.
const clampPercent = (value: number, lo: number, hi: number): number => {
	if (!(hi > lo)) {
		return 50;
	}

	return Math.min(100, Math.max(0, ((value - lo) / (hi - lo)) * 100));
};

const positiveFinite = (value: number): number | null =>
	Number.isFinite(value) && value > 0 ? value : null;

type PriceGaugeGeometry = {
	entryPct: number;
	markPct: number;
	stopPct: number | null;
	fillLo: number;
	fillHi: number;
	aboveStop: boolean;
	rawMarkPrice: number | null;
};

type ExecutionAuditRow = {
	reason: string;
	reference: string;
	meta: string;
};

export const executionAuditRow = (execution: Execution): ExecutionAuditRow => {
	const reason = execution.order_status ?? execution.exec_type;

	if (typeof reason !== "string" || reason.length === 0) {
		throw new TypeError("execution requires order_status or exec_type");
	}

	const timestamp =
		typeof execution.timestamp === "string"
			? execution.timestamp.slice(11, 19)
			: "";
	const identifier =
		typeof execution.sequence === "number"
			? String(execution.sequence)
			: (execution.exec_id ?? execution.order_id ?? "");
	const trade =
		execution.last_qty !== undefined && execution.last_price !== undefined
			? `${execution.last_qty} @ ${fixed(Number(execution.last_price))}`
			: "";
	const meta = [
		execution.exec_type,
		execution.side,
		execution.symbol,
		trade,
	].filter((value) => typeof value === "string" && value.length > 0);

	return {
		reason,
		reference: [`#${identifier}`, timestamp].filter(Boolean).join(" · "),
		meta: meta.join(" · "),
	};
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
		const span = Math.abs(markReturn);

		if (!(span > 0)) {
			return null;
		}

		const lo = -span;
		const hi = span;
		const entryPct = clampPercent(0, lo, hi);
		const markPct = clampPercent(markReturn, lo, hi);

		return {
			entryPct,
			markPct,
			stopPct: null,
			fillLo: Math.min(entryPct, markPct),
			fillHi: Math.max(entryPct, markPct),
			aboveStop: position.pnl > 0 || position.return_pct > 0,
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
	const entryPct = clampPercent(0, lo, hi);
	const markPct = clampPercent(markReturn, lo, hi);
	const stopPct = clampPercent(stopReturn, lo, hi);

	return {
		entryPct,
		markPct,
		stopPct,
		fillLo: Math.min(stopPct, markPct),
		fillHi: Math.max(stopPct, markPct),
		aboveStop: markReturn >= stopReturn,
		rawMarkPrice: rawMark,
	};
};

type PositionGaugeRefs = {
	fill: HTMLDivElement | null;
	entryTick: HTMLDivElement | null;
	markTick: HTMLDivElement | null;
	stopTick: HTMLDivElement | null;
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

/*
paintPositionGauge reads symbol's current position/stop straight from the
stores and writes every derived value directly into the DOM via refs. It
runs on every tick from a vanilla store.subscribe callback, never through
React state, so a mark-price update never re-renders PositionGauge, its
siblings, or their parents.
*/
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
	const stopPct = geometry?.stopPct ?? null;
	const rawMark = positiveFinite(position.mark);
	const fillTone = geometry?.aboveStop === false ? downTone : upTone;
	const markLabel = rawMark === null ? "--" : fixed(rawMark);

	const hasMomentum = stop?.momentum_active === true;
	const health = hasMomentum
		? Math.max(0, Math.min(1, stop.momentum_health))
		: 0;
	const momentumTone =
		health > 0.5 ? upTone : health > 0.2 ? warnTone : downTone;

	const hasStagnation = stop?.stagnation_active === true;

	if (refs.fill && refs.entryTick && refs.markTick) {
		const display = geometry !== null ? "" : "none";
		refs.fill.style.display = display;
		refs.entryTick.style.display = display;
		refs.markTick.style.display = display;

		if (geometry !== null) {
			refs.fill.style.left = `${geometry.fillLo}%`;
			refs.fill.style.width = `${Math.max(0, geometry.fillHi - geometry.fillLo)}%`;
			refs.fill.style.background = `color-mix(in srgb, ${fillTone} 14%, transparent)`;
			refs.entryTick.style.left = `${geometry.entryPct}%`;
			refs.markTick.style.left = `${geometry.markPct}%`;
			refs.markTick.style.background = `color-mix(in srgb, ${fillTone} 75%, transparent)`;
		}
	}

	if (refs.stopTick) {
		refs.stopTick.style.display = stopPct !== null ? "" : "none";

		if (stopPct !== null) {
			refs.stopTick.style.left = `${stopPct}%`;
		}
	}

	if (refs.pnl) {
		refs.pnl.style.color = pnlTone;
		refs.pnl.textContent = `P/L ${position.pnl.toFixed(4)} ${quote}`;
	}

	if (refs.summary) {
		const stopSuffix =
			stop !== undefined && stopPct !== null
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
			const stagnationTone = stop.stagnation_pending
				? accentTone
				: stop.stagnation_health > 0.5
					? upTone
					: stop.stagnation_health > 0.2
						? warnTone
						: downTone;

			refs.stagnationBar.style.width = `${(stop.stagnation_health * 100).toFixed(0)}%`;
			refs.stagnationBar.style.background = stagnationTone;
			refs.stagnationFlash.style.display = stop.stagnation_pending
				? ""
				: "none";
		}
	}
};

const PositionGauge = memo(
	({ symbol, quote }: { symbol: string; quote: string }) => {
		const fillRef = useRef<HTMLDivElement>(null);
		const entryTickRef = useRef<HTMLDivElement>(null);
		const markTickRef = useRef<HTMLDivElement>(null);
		const stopTickRef = useRef<HTMLDivElement>(null);
		const pnlRef = useRef<HTMLSpanElement>(null);
		const summaryRef = useRef<HTMLSpanElement>(null);
		const returnRef = useRef<HTMLSpanElement>(null);
		const momentumWrapRef = useRef<HTMLDivElement>(null);
		const momentumBarRef = useRef<HTMLDivElement>(null);
		const stagnationWrapRef = useRef<HTMLDivElement>(null);
		const stagnationBarRef = useRef<HTMLDivElement>(null);
		const stagnationFlashRef = useRef<HTMLSpanElement>(null);

		useLayoutEffect(() => {
			const refs: PositionGaugeRefs = {
				fill: fillRef.current,
				entryTick: entryTickRef.current,
				markTick: markTickRef.current,
				stopTick: stopTickRef.current,
				pnl: pnlRef.current,
				summary: summaryRef.current,
				returnPct: returnRef.current,
				momentumWrap: momentumWrapRef.current,
				momentumBar: momentumBarRef.current,
				stagnationWrap: stagnationWrapRef.current,
				stagnationBar: stagnationBarRef.current,
				stagnationFlash: stagnationFlashRef.current,
			};

			const paint = () => paintPositionGauge(refs, symbol, quote);

			paint();
			const positionsSubscription = positionsStore.subscribe(paint);
			const stopsSubscription = stopsStore.subscribe(paint);

			return () => {
				positionsSubscription.unsubscribe();
				stopsSubscription.unsubscribe();
			};
		}, [symbol, quote]);

		return (
			<div
				data-symbol={symbol}
				className="relative mb-[5px] overflow-hidden rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-1.5 font-mono text-[11px]"
			>
				{/* gauge fill: stop → mark */}
				<div ref={fillRef} className="pointer-events-none absolute inset-y-0" />
				{/* entry reference tick */}
				<div
					ref={entryTickRef}
					className="pointer-events-none absolute inset-y-0 w-px"
					style={{
						background: "color-mix(in srgb, var(--f4) 45%, transparent)",
					}}
				/>
				{/* current mark tick */}
				<div
					ref={markTickRef}
					className="pointer-events-none absolute inset-y-0 w-px"
				/>
				{/* trailing-stop line — the moving exit point */}
				<div
					ref={stopTickRef}
					className="pointer-events-none absolute inset-y-0 w-px"
					style={{
						background: "color-mix(in srgb, var(--down) 85%, transparent)",
						boxShadow:
							"0 0 4px color-mix(in srgb, var(--down) 60%, transparent)",
					}}
				/>

				<div className="relative flex items-start justify-between gap-3">
					<span className="font-semibold text-(--f1)">{symbol}</span>
					<span ref={pnlRef} className="text-right font-semibold" />
				</div>
				<div className="relative mt-1 flex items-center justify-between gap-3 text-[10px] text-(--f4)">
					<span ref={summaryRef} />
					<span ref={returnRef} />
				</div>

				{/* momentum-decay meter: remaining driving energy before a momentum exit */}
				<div
					ref={momentumWrapRef}
					className="relative mt-1.5 flex items-center gap-1.5"
				>
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

				{/* stagnation meter: remaining touches before a stagnation exit */}
				<div
					ref={stagnationWrapRef}
					className="relative mt-1.5 flex items-center gap-1.5"
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
			</div>
		);
	},
);

const PositionRows = ({
	symbols,
	quote,
	observed,
}: {
	symbols: string[];
	quote: string;
	observed: boolean;
}) => (
	<div className="min-h-0 flex-1 overflow-auto p-1.5">
		{symbols.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				{observed ? "no open positions" : "waiting for position frames"}
			</div>
		) : null}
		{symbols.slice(-8).map((symbol) => (
			<PositionGauge key={symbol} symbol={symbol} quote={quote} />
		))}
	</div>
);

const AuditRows = ({ executions }: { executions: Execution[] }) => (
	<div className="min-h-0 flex-1 overflow-auto py-0.5">
		{executions.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				waiting for execution frames
			</div>
		) : null}
		{executions.map((execution) => {
			const row = executionAuditRow(execution);

			return (
				<div
					key={`${execution.exec_id ?? execution.order_id ?? "exec"}:${execution.order_status ?? ""}:${execution.timestamp ?? ""}:${execution.symbol ?? ""}:${execution.side ?? ""}`}
					className="border-(--line) border-b px-3 py-1.5"
				>
					<div className="flex items-start justify-between gap-2">
						<span className="font-medium text-[11px] text-(--f1)">
							{row.reason}
						</span>
						<span className="shrink-0 font-mono text-[9px] text-(--f4)">
							{row.reference}
						</span>
					</div>
					<div className="mt-px truncate font-mono text-[9px] text-(--f4)">
						{row.meta}
					</div>
				</div>
			);
		})}
	</div>
);

export const DashboardRail = () => {
	const actions = useSelector(actionStore, (state) =>
		Object.values(state.actions).flatMap((history) => history.values()),
	);
	const allowed = actions.filter((action) => action.verdict === "allow");
	const denied = actions.filter((action) => action.verdict !== "allow");

	/*
	symbols only changes reference when a position opens or closes, not on
	every mark/pnl tick, so it is the one piece of positions state safe to
	drive React re-renders and row mount/unmount from. Each PositionGauge
	reads its own live values straight from the stores.
	*/
	const symbols = useSelector(
		positionsStore,
		(state) => state.positions.map((position) => position.symbol),
		{ compare: sameSymbols },
	);
	const observed = useSelector(positionsStore, (state) => state.observed);
	const quoteSymbol = useSelector(
		positionsStore,
		(state) => state.positions[0]?.symbol,
	);
	const net = useSelector(positionsStore, (state) =>
		state.positions.reduce((sum, position) => sum + position.pnl, 0),
	);
	const quote = quoteSymbol?.split("/")[1] ?? "USD";
	const executions = useSelector(executionsStore, (state) => state.executions);

	return (
		<div className="flex min-h-0 flex-col bg-(--surface)">
			<div className="flex min-h-0 flex-[1.15] flex-col border-(--line) border-b">
				<ColumnHeader
					title="Decisions"
					meta={
						<span>
							{allowed.length} allow · {denied.length} deny
						</span>
					}
				/>
				<DecisionRows decisions={actions.reverse()} />
			</div>
			<div className="flex min-h-0 flex-1 flex-col border-(--line) border-b">
				<ColumnHeader
					title="Open positions"
					meta={
						<span>
							{symbols.length === 0 ? "" : `net ${net.toFixed(4)} ${quote} · `}
							{symbols.length} open
						</span>
					}
				/>
				<PositionRows symbols={symbols} quote={quote} observed={observed} />
			</div>
			<div className="flex min-h-0 flex-1 flex-col">
				<ColumnHeader title="Audit trail" />
				<AuditRows executions={[...executions].reverse()} />
			</div>
		</div>
	);
};
