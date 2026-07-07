import { useSelector } from "@tanstack/react-store";
import { type Action, actionStore } from "#/collections/actions";
import { type Execution, executionsStore } from "#/collections/executions";
import { type Position, positionsStore } from "#/collections/positions";
import { type Stop, stopsStore } from "#/collections/stops";
import { ColumnHeader } from "#/components/dashboard/header";
import { fixed } from "#/components/terminal/decision-format";
import { cn } from "#/lib/utils";

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
					key={decision.id}
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

const PositionGauge = ({
	position,
	stop,
	quote,
}: {
	position: Position;
	stop?: Stop;
	quote: string;
}) => {
	const profitable = position.pnl > 0 || position.return_pct > 0;
	const pnlTone = profitable
		? "text-(--up)"
		: position.pnl < 0 || position.return_pct < 0
			? "text-(--down)"
			: "text-(--f3)";

	const geometry = positionGaugeGeometry(position, stop);
	const stopPct = geometry?.stopPct ?? null;
	const rawMark = positiveFinite(position.mark);

	// Fill spans the gap between the stop and the current mark: a wide green band
	// means plenty of room above the stop, a thin red sliver means it is close.
	const fillTone = geometry?.aboveStop === false ? "var(--down)" : "var(--up)";
	const markLabel = rawMark === null ? "--" : fixed(rawMark);

	// Momentum-decay exit has no price: it fires when the driving energy falls to
	// the decay floor. Show it as a thin track whose fill is the remaining energy
	// (momentum_health, 1 = at peak, 0 = at the floor / exit imminent), tinted from
	// green through amber toward red as it approaches the floor.
	const hasMomentum = stop?.momentum_active === true;
	const health = hasMomentum
		? Math.max(0, Math.min(1, stop.momentum_health))
		: 0;
	const momentumTone =
		health > 0.5 ? "var(--up)" : health > 0.2 ? "var(--warn)" : "var(--down)";

	return (
		<div
			key={`${position.symbol}:${position.entry_price}:${position.qty}`}
			className="relative overflow-hidden border-(--line) border-b px-3 py-2.5 font-mono text-[11px]"
		>
			{/* gauge fill: stop → mark */}
			{geometry !== null ? (
				<>
					<div
						className="pointer-events-none absolute inset-y-0"
						style={{
							left: `${geometry.fillLo}%`,
							width: `${Math.max(0, geometry.fillHi - geometry.fillLo)}%`,
							background: `color-mix(in srgb, ${fillTone} 14%, transparent)`,
						}}
					/>
					{/* entry reference tick */}
					<div
						className="pointer-events-none absolute inset-y-0 w-px"
						style={{
							left: `${geometry.entryPct}%`,
							background: "color-mix(in srgb, var(--f4) 45%, transparent)",
						}}
					/>
					{/* current mark tick */}
					<div
						className="pointer-events-none absolute inset-y-0 w-px"
						style={{
							left: `${geometry.markPct}%`,
							background: `color-mix(in srgb, ${fillTone} 75%, transparent)`,
						}}
					/>
				</>
			) : null}
			{/* trailing-stop line — the moving exit point */}
			{stopPct !== null ? (
				<div
					className="pointer-events-none absolute inset-y-0 w-px"
					style={{
						left: `${stopPct}%`,
						background: "color-mix(in srgb, var(--down) 85%, transparent)",
						boxShadow:
							"0 0 4px color-mix(in srgb, var(--down) 60%, transparent)",
					}}
				/>
			) : null}

			<div className="relative flex items-start justify-between gap-3">
				<span className="font-semibold text-(--f1)">{position.symbol}</span>
				<span className={cn("text-right font-semibold", pnlTone)}>
					P/L {position.pnl.toFixed(4)} {quote}
				</span>
			</div>
			<div className="relative mt-1 flex items-center justify-between gap-3 text-[10px] text-(--f4)">
				<span>
					entry {fixed(position.entry_price)} / mark {markLabel}
					{stop !== undefined && stopPct !== null
						? ` / stop ${fixed(stop.stop_price)}`
						: ""}
				</span>
				<span className={pnlTone}>
					{(position.return_pct * 100).toFixed(2)}%
				</span>
			</div>

			{/* momentum-decay meter: remaining driving energy before a momentum exit */}
			{hasMomentum ? (
				<div className="relative mt-1.5 flex items-center gap-1.5">
					<span className="text-[8px] text-(--f4) uppercase tracking-wide">
						mom
					</span>
					<div className="h-[3px] flex-1 overflow-hidden rounded-full bg-[color-mix(in_srgb,var(--f4)_25%,transparent)]">
						<div
							className="h-full rounded-full transition-[width]"
							style={{
								width: `${health * 100}%`,
								background: momentumTone,
							}}
						/>
					</div>
				</div>
			) : null}

			{/* stagnation meter: remaining touches before a stagnation exit */}
			{stop?.stagnation_active === true ? (
				<div className="relative mt-1.5 flex items-center gap-1.5">
					<span className="text-[8px] text-(--f4) uppercase tracking-wide">
						stall
					</span>
					<div className="h-[3px] flex-1 overflow-hidden rounded-full bg-[color-mix(in_srgb,var(--f4)_25%,transparent)]">
						<div
							className="h-full rounded-full transition-[width]"
							style={{
								width: `${(stop.stagnation_health * 100).toFixed(0)}%`,
								background: stop.stagnation_pending
									? "var(--acc)"
									: stop.stagnation_health > 0.5
										? "var(--up)"
										: stop.stagnation_health > 0.2
											? "var(--warn)"
											: "var(--down)",
							}}
						/>
					</div>
					{stop.stagnation_pending ? (
						<span className="text-[8px] font-semibold text-(--acc) uppercase tracking-wide">
							⚡
						</span>
					) : null}
				</div>
			) : null}
		</div>
	);
};

const PositionRows = ({
	positions,
	stops,
	quote,
	observed,
}: {
	positions: Position[];
	stops: Record<string, Stop>;
	quote: string;
	observed: boolean;
}) => (
	<div className="min-h-0 flex-1 overflow-auto">
		{positions.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				{observed ? "no open positions" : "waiting for position frames"}
			</div>
		) : null}
		{positions.slice(-8).map((position) => (
			<PositionGauge
				key={`${position.symbol}:${position.entry_price}:${position.qty}`}
				position={position}
				stop={stops[position.symbol]}
				quote={quote}
			/>
		))}
	</div>
);

const AuditRows = ({ executions }: { executions: Execution[] }) => (
	<div className="min-h-0 flex-1 overflow-auto">
		{executions.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				waiting for execution frames
			</div>
		) : null}
		{executions.map((execution) => (
			<div
				key={execution.exec_id}
				className="border-(--line) border-b px-3 py-2.5 font-mono text-[11px]"
			>
				<div className="flex items-center justify-between gap-3">
					<span className="font-semibold text-(--f2)">
						{execution.order_status}
					</span>
					<span className="text-[9px] text-(--f4)">#{execution.exec_id}</span>
				</div>
				<div className="mt-1 truncate text-[10px] text-(--f4)">
					{execution.side} / {execution.symbol}
				</div>
			</div>
		))}
	</div>
);

export const DashboardRail = () => {
	const actions = useSelector(actionStore, (state) =>
		Object.values(state.actions).flatMap((history) => history.values()),
	);
	const allowed = actions.filter((action) => action.verdict === "allow");
	const denied = actions.filter((action) => action.verdict !== "allow");
	const positionsState = useSelector(positionsStore, (state) => state);
	const executions = useSelector(executionsStore, (state) =>
		state.executions.flat(),
	);
	const stops = useSelector(stopsStore, (state) => state.stops);
	const positions = positionsState.positions;
	const quote = positions[0]?.symbol.split("/")[1] ?? "USD";
	const net = positions.reduce((sum, position) => sum + position.pnl, 0);

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
							{positions.length === 0
								? ""
								: `net ${net.toFixed(4)} ${quote} · `}
							{positions.length} open
						</span>
					}
				/>
				<PositionRows
					positions={positions}
					stops={stops}
					quote={quote}
					observed={positionsState.observed}
				/>
			</div>
			<div className="flex min-h-0 flex-1 flex-col">
				<ColumnHeader title="Audit trail" />
				<AuditRows executions={executions.reverse()} />
			</div>
		</div>
	);
};
