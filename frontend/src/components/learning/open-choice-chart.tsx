import { lazy, memo, Suspense, useMemo, useRef, useState } from "react";
import { positionReadout } from "#/components/book-readout";
import { cn } from "#/lib/utils";
import {
	indexMean,
	openChoiceLegend,
	openChoiceSeries,
	openChoiceTrail,
} from "./open-choice-trail";

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

const lotKey = (lots: Record<string, unknown>[]) =>
	lots.map((row) => String(row.id ?? row.symbol)).join("|");

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
		const meanSeries = useMemo(() => {
			const trails = series
				.filter((item) => item.values.length > 0)
				.map((item) => openChoiceTrail(item.id));
			return indexMean(trails);
		}, [series]);
		const latestMean =
			[...meanSeries].reverse().find((value) => Number.isFinite(value)) ??
			null;
		const bookTone =
			latestMean === null
				? "flat"
				: latestMean >= 2
					? "ahead"
					: latestMean <= -2
						? "behind"
						: "flat";

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
							Close-now P&amp;L trails. Toggle grid for one chart per lot.
						</div>
					</div>
					<div className="flex items-center gap-3">
						<div
							className="min-w-22 text-right text-[12px] font-bold tabular-nums"
							style={{
								color:
									latestMean === null
										? "var(--f4)"
										: bookTone === "ahead"
											? "var(--up)"
											: bookTone === "behind"
												? "var(--down)"
												: "var(--warn)",
							}}
						>
							{latestMean === null
								? "book …"
								: `book ${pctText(latestMean)}`}
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
										style={{ background: item.stroke }}
									/>
									{item.name}
									<span className="tabular-nums text-(--f2)">
										{pnl === null ? "…" : pctText(pnl)}
									</span>
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
