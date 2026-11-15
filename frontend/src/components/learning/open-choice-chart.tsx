import { useSelector } from "@tanstack/react-store";
import { lazy, Suspense, useCallback, useRef, useState } from "react";
import { bookAtom, type Book, isBook } from "#/collections/book";
import { moneyText, pctText } from "#/lib/format";
import { cn } from "#/lib/utils";
import { trySendCommand } from "#/providers/websocket";
import type {
	OpenChoiceChartApi,
	OpenChoiceLegendItem,
} from "./open-choice-draw";

/*
SciChart's ESM is broken under Node SSR. Lazy-load so the route module can
evaluate without pulling scichart-react in. Hub marks push into series;
React only hosts chrome (layout toggle + legend).
*/
const OpenChoiceSciChart = lazy(() =>
	import("./open-choice-scichart").then((module) => ({
		default: module.OpenChoiceSciChart,
	})),
);

const exitBook = (item: OpenChoiceLegendItem, selected: Book): Book =>
	isBook(item.tier) ? item.tier : selected;

export const OpenChoiceChart = () => {
	const book = useSelector(bookAtom, (state) => state);
	const [isGrid, setIsGrid] = useState(false);
	const [legend, setLegend] = useState<OpenChoiceLegendItem[]>([]);
	const [exiting, setExiting] = useState<Record<string, boolean>>({});
	const legendKey = useRef("");
	const apiRef = useRef<OpenChoiceChartApi | null>(null);

	const onLegend = useCallback((items: OpenChoiceLegendItem[]) => {
		const key = items
			.map(
				(item) =>
					`${item.id}:${item.pnlMoney ?? ""}:${item.grossMoney ?? ""}:${item.pnlPct ?? ""}`,
			)
			.join("|");
		if (key === legendKey.current) return;
		legendKey.current = key;
		setLegend(items);
		setExiting((prev) => {
			const next: Record<string, boolean> = {};
			for (const item of items) {
				if (prev[item.id]) next[item.id] = true;
			}
			return next;
		});
	}, []);

	const setLayout = (grid: boolean) => {
		apiRef.current?.setIsGridLayoutMode(grid);
		setIsGrid(grid);
	};

	const exitPosition = (item: OpenChoiceLegendItem) => {
		if (exiting[item.id]) return;

		setExiting((prev) => ({ ...prev, [item.id]: true }));
		const ok = trySendCommand({
			type: "exit",
			id: item.id,
			symbol: item.name,
			book: exitBook(item, book),
		});

		if (!ok) {
			setExiting((prev) => {
				const next = { ...prev };
				delete next[item.id];
				return next;
			});
		}
	};

	return (
		<section className="flex h-full min-h-0 w-full max-w-none flex-1 flex-col bg-(--surface)">
			<div className="flex shrink-0 flex-wrap items-baseline justify-between gap-3 border-b border-(--line) px-3 py-2">
				<div>
					<div className="text-[11px] font-bold uppercase tracking-wider text-(--f2)">
						Open choices
					</div>
					<div className="text-[10px] text-(--f4)">
						Full open-position snapshots → one SciChart series each. Toggle grid per position.
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
				<Suspense
					fallback={
						<div className="flex h-full w-full items-center justify-center text-[11px] text-(--f4)">
							loading chart…
						</div>
					}
				>
					<OpenChoiceSciChart
						onLegend={onLegend}
						onReady={(api) => {
							apiRef.current = api;
						}}
					/>
				</Suspense>
			</div>
			{legend.length > 0 ? (
				<div className="flex max-h-28 shrink-0 flex-wrap gap-x-3 gap-y-1 overflow-auto border-t border-(--line) px-3 py-1.5 text-[10px] text-(--f3)">
					{legend.map((item) => {
						const title = [
							item.entry !== undefined && item.mark !== undefined
								? `entry ${item.entry} → bid ${item.mark}`
								: null,
							item.grossMoney !== undefined
								? `gross ${moneyText(item.grossMoney)}`
								: null,
							item.feeMoney !== undefined
								? `fees ${moneyText(-item.feeMoney)}`
								: null,
							item.pnlMoney !== undefined
								? `net ${moneyText(item.pnlMoney)}`
								: null,
						]
							.filter(Boolean)
							.join(" · ");

						return (
							<span
								key={item.id}
								title={title}
								className="inline-flex items-center gap-1"
							>
								<span
									className="inline-block h-1.5 w-3 rounded-sm"
									style={{ background: item.stroke }}
								/>
								{item.name}
								<span className="tabular-nums text-(--f4)">
									{item.grossMoney !== undefined
										? `g${moneyText(item.grossMoney)}`
										: ""}
								</span>
								<span className="tabular-nums text-(--f2)">
									{item.pnlMoney !== undefined
										? moneyText(item.pnlMoney)
										: "…"}
								</span>
								<span className="tabular-nums text-(--f4)">
									{item.pnlPct === undefined ? "" : pctText(item.pnlPct)}
								</span>
								<button
									type="button"
									aria-label={`Exit ${item.name}`}
									title={`Manually exit ${item.name}`}
									disabled={Boolean(exiting[item.id])}
									onClick={() => exitPosition(item)}
									className={cn(
										"ml-0.5 rounded border border-(--line) px-1 py-px text-[9px] uppercase tracking-wide transition-colors",
										exiting[item.id]
											? "cursor-wait text-(--f4)"
											: "cursor-pointer text-(--f3) hover:border-(--down) hover:text-(--down)",
									)}
								>
									{exiting[item.id] ? "…" : "exit"}
								</button>
							</span>
						);
					})}
				</div>
			) : null}
		</section>
	);
};
