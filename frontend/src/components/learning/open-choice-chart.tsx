import { lazy, memo, Suspense, useMemo, useRef, useState } from "react";
import { positionReadout } from "#/components/book-readout";
import { cn } from "#/lib/utils";
import { openChoiceLegend, openChoiceSeries } from "./open-choice-trail";

type ChartApi = {
	setIsGridLayoutMode: (value: boolean) => void;
	syncSeries: (lots: Record<string, unknown>[]) => void;
};

/*
SciChart's ESM is broken under Node SSR (extensionless imports). Lazy-load the
surface so the route module can evaluate without pulling scichart-react in.
*/
const OpenChoiceSciChart = lazy(() =>
	import("./open-choice-scichart").then((module) => ({
		default: module.OpenChoiceSciChart,
	})),
);

const pctText = (value: number) => {
	const digits = Math.abs(value) >= 10 ? 0 : 1;
	const sign = value > 0 ? "+" : "";
	return `${sign}${value.toFixed(digits)}%`;
};

const moneyText = (value: number) => {
	const abs = Math.abs(value);
	const digits = abs >= 100 ? 0 : abs >= 10 ? 1 : 2;
	return value >= 0 ? `$+${abs.toFixed(digits)}` : `$-${abs.toFixed(digits)}`;
};

const roomText = (room: number) => {
	const digits = Math.abs(room) >= 10 ? 0 : 1;
	return `${room.toFixed(digits)}pp`;
};

const nest = (
	row: Record<string, unknown>,
	key: string,
): Record<string, unknown> => {
	const value = row[key];
	return value && typeof value === "object"
		? (value as Record<string, unknown>)
		: {};
};

const lotKey = (lots: Record<string, unknown>[]) =>
	lots
		.map((row) => {
			const structural = nest(row, "structural");
			const learning = nest(row, "learning");

			return [
				row.id ?? row.symbol,
				learning.active === true ? 1 : 0,
				Array.isArray(learning.pnl_trace) ? learning.pnl_trace.length : 0,
				row.official_exit_at ? 1 : 0,
				row.official_exit_pct,
				learning.exit_at ? 1 : 0,
				learning.exit_pct,
				row.pnl_pct,
				row.pnl,
				structural.floor_pct,
				learning.floor_pct,
				structural.proximity,
				structural.room_pct,
				learning.proximity,
				learning.room_pct,
				Array.isArray(structural.floor_trace)
					? structural.floor_trace.length
					: 0,
				Array.isArray(learning.floor_trace) ? learning.floor_trace.length : 0,
			].join(":");
		})
		.join("|");

export const OpenChoiceChart = memo(
	function OpenChoiceChart({
		lots,
		trailGen = 0,
	}: {
		lots: Record<string, unknown>[];
		trailGen?: number;
	}) {
		const apiRef = useRef<ChartApi | null>(null);
		const [isGrid, setIsGrid] = useState(false);

		const series = useMemo(() => {
			void trailGen;
			return openChoiceSeries(lots);
		}, [lots, trailGen]);
		const legend = useMemo(() => openChoiceLegend(series), [series]);
		const watching = series.some((item) => item.values.length > 0);

		const setLayout = (grid: boolean) => {
			apiRef.current?.setIsGridLayoutMode(grid);
			setIsGrid(grid);
		};

		return (
			<section className="flex h-full min-h-0 w-full max-w-none flex-1 flex-col bg-(--surface)">
				<div className="flex shrink-0 flex-wrap items-baseline justify-between gap-3 border-b border-(--line) px-3 py-2">
					<div>
						<div className="text-[11px] font-bold uppercase tracking-wider text-(--f2)">
							Open choices
						</div>
						<div className="text-[10px] text-(--f4)">
							Active floor band under the mark. Dashed path only when the
							learning shadow beats structural. Toggle grid per lot.
						</div>
					</div>
					<fieldset className="m-0 flex items-center gap-0.5 rounded border border-(--line) bg-(--sunken) p-0.5">
						<legend className="sr-only">Chart layout</legend>
						<button
							type="button"
							aria-pressed={!isGrid}
							onClick={() => setLayout(false)}
							className={cn(
								"rounded px-2 py-0.5 text-[11px] transition-colors cursor-pointer",
								!isGrid
									? "bg-(--raised) font-bold text-(--f1) shadow-xs"
									: "text-(--f3) hover:text-(--f1)",
							)}
						>
							Single
						</button>
						<button
							type="button"
							aria-pressed={isGrid}
							onClick={() => setLayout(true)}
							className={cn(
								"rounded px-2 py-0.5 text-[11px] transition-colors cursor-pointer",
								isGrid
									? "bg-(--raised) font-bold text-(--f1) shadow-xs"
									: "text-(--f3) hover:text-(--f1)",
							)}
						>
							Grid
						</button>
					</fieldset>
				</div>
				<div className="relative min-h-0 w-full flex-1">
					{watching ? (
						<Suspense
							fallback={
								<div className="flex h-full w-full items-center justify-center text-[11px] text-(--f4)">
									loading chart…
								</div>
							}
						>
							<OpenChoiceSciChart
								lots={lots}
								trailGen={trailGen}
								apiRef={apiRef}
							/>
						</Suspense>
					) : (
						<div className="relative flex h-full w-full items-center justify-center text-[11px] text-(--f4)">
							close-now return is not on these lots yet
						</div>
					)}
				</div>
				{legend.length > 0 ? (
					<div className="flex max-h-24 shrink-0 flex-wrap gap-x-3 gap-y-1 overflow-auto border-t border-(--line) px-3 py-1.5 text-[10px] text-(--f3)">
						{legend.map((item) => {
							const row = lots.find(
								(lot) => String(lot.id ?? lot.symbol) === item.id,
							);
							const pnl = row
								? positionReadout(row).returnPctDisplay
								: null;
							return (
								<span
									key={item.id}
									className="inline-flex items-center gap-1"
								>
									<span
										className="inline-block h-1.5 w-3 rounded-sm"
										style={{
											background: item.stroke,
											backgroundImage:
												item.dashFrom !== undefined
													? `repeating-linear-gradient(90deg, ${item.stroke} 0 4px, transparent 4px 7px)`
													: undefined,
										}}
									/>
									{item.name}
									<span className="tabular-nums text-(--f2)">
										{item.pnlMoney !== undefined
											? moneyText(item.pnlMoney)
											: "…"}
									</span>
									<span className="tabular-nums text-(--f4)">
										{pnl === null ? "" : pctText(pnl)}
									</span>
									{item.shadowActive ? (
										<span className="rounded border border-(--line) px-1 text-[9px] uppercase tracking-wide text-(--f4)">
											shadow live
										</span>
									) : null}
									{item.stopRoomPct !== undefined ? (
										<span
											className={cn(
												"tabular-nums",
												(item.stopProximity ?? 0) >= 0.7
													? "text-(--warn)"
													: "text-(--f4)",
											)}
										>
											{(item.stopProximity ?? 0) >= 1
												? "stop"
												: `${roomText(item.stopRoomPct)} to stop`}
											{item.stopFloor !== undefined
												? ` @ ${pctText(item.stopFloor)}`
												: ""}
										</span>
									) : item.stopFloor !== undefined ? (
										<span className="tabular-nums text-(--f4)">
											stop {pctText(item.stopFloor)}
										</span>
									) : null}
									{item.shadowRoomPct !== undefined ? (
										<span className="tabular-nums text-(--f4)">
											{(item.shadowProximity ?? 0) >= 1
												? "shadow"
												: `${roomText(item.shadowRoomPct)} shadow`}
										</span>
									) : item.shadowFloor !== undefined ? (
										<span className="tabular-nums text-(--f4)">
											shadow {pctText(item.shadowFloor)}
										</span>
									) : null}
								</span>
							);
						})}
					</div>
				) : null}
			</section>
		);
	},
	(prev, next) =>
		prev.trailGen === next.trailGen &&
		lotKey(prev.lots) === lotKey(next.lots),
);
