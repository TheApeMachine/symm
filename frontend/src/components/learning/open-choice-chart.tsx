import { useSelector } from "@tanstack/react-store";
import { lazy, Suspense, useRef, useState } from "react";
import { signals } from "#/collections/app";
import { type Book, bookAtom, isBook } from "#/collections/book";
import type { Position } from "#/collections/types";
import { comparePositionRows } from "#/lib/frame-position";
import { asFinite, moneyText, pctText } from "#/lib/format";
import { cn } from "#/lib/utils";
import { trySendCommand } from "#/providers/websocket";
import type { OpenChoiceChartApi } from "./open-choice-draw";
import { strokeFor } from "./open-choice-stroke";

/*
SciChart's ESM is broken under Node SSR. Lazy-load so the route module can
evaluate without pulling scichart-react in.
*/
const OpenChoiceSciChart = lazy(() =>
	import("./open-choice-scichart").then((module) => ({
		default: module.OpenChoiceSciChart,
	})),
);

const exitBook = (position: Position, book: Book): Book =>
	isBook(position.tier_key) ? position.tier_key : book;

export const OpenChoiceChart = () => {
	const book = useSelector(bookAtom, (state) => state);
	const allPositions = useSelector(
		signals.positions,
		(state) => state as Record<string, Position>,
	);
	const positions = Object.values(allPositions)
		.filter((pos) => {
			const tier = pos.tier_key && pos.tier_key !== "" ? pos.tier_key : "paper";
			return !pos.closed_at && tier === book;
		})
		.sort(comparePositionRows);
	const chartApi = useRef<OpenChoiceChartApi | null>(null);
	const [isGrid, setIsGrid] = useState(false);
	const [exiting, setExiting] = useState<Record<string, boolean>>({});

	const setLayout = (grid: boolean) => {
		chartApi.current?.setIsGridLayoutMode(grid);
		setIsGrid(grid);
	};

	const exitPosition = (position: Position) => {
		if (exiting[position.id]) return;

		setExiting((prev) => ({ ...prev, [position.id]: true }));
		const ok = trySendCommand({
			type: "exit",
			id: position.id,
			symbol: position.symbol,
			book: exitBook(position, book),
		});

		if (!ok) {
			setExiting((prev) => {
				const next = { ...prev };
				delete next[position.id];
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
						onReady={(ready) => {
							chartApi.current = ready;
							ready.setIsGridLayoutMode(isGrid);
						}}
					/>
				</Suspense>
			</div>
			{positions.length > 0 ? (
				<div className="flex max-h-28 shrink-0 flex-wrap gap-x-3 gap-y-1 overflow-auto border-t border-(--line) px-3 py-1.5 text-[10px] text-(--f3)">
					{positions.map((position) => {
						const pnl = asFinite(position.pnl);
						const pct = asFinite(position.pnl_pct);
						const title = [
							position.entry !== undefined && position.mark !== undefined
								? `entry ${position.entry} → bid ${position.mark}`
								: null,
							pnl !== null ? `net ${moneyText(pnl)}` : null,
						]
							.filter(Boolean)
							.join(" · ");

						return (
							<span
								key={position.id}
								title={title}
								className="inline-flex items-center gap-1"
							>
								<span
									className="inline-block h-1.5 w-3 rounded-sm"
									style={{ background: strokeFor(position.id) }}
								/>
								{position.symbol}
								<span className="tabular-nums text-(--f4)">
									{pnl !== null ? `g${moneyText(pnl)}` : ""}
								</span>
								<span className="tabular-nums text-(--f2)">
									{pnl !== null ? moneyText(pnl) : "…"}
								</span>
								<span className="tabular-nums text-(--f4)">
									{pct === null ? "" : pctText(pct)}
								</span>
								<button
									type="button"
									aria-label={`Exit ${position.symbol}`}
									title={`Manually exit ${position.symbol}`}
									disabled={Boolean(exiting[position.id])}
									onClick={() => exitPosition(position)}
									className={cn(
										"ml-0.5 rounded border border-(--line) px-1 py-px text-[9px] uppercase tracking-wide transition-colors",
										exiting[position.id]
											? "cursor-wait text-(--f4)"
											: "cursor-pointer text-(--f3) hover:border-(--down) hover:text-(--down)",
									)}
								>
									{exiting[position.id] ? "…" : "exit"}
								</button>
							</span>
						);
					})}
				</div>
			) : null}
		</section>
	);
};
