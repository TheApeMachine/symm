import { useSelector } from "@tanstack/react-store";
import { useRef, useState } from "react";
import { appStore } from "#/collections/app";
import type {
	CausalFrame,
	Instrument,
	ManifoldFrame,
	Measurement,
	ResonanceFrame,
} from "#/collections/types";
import type { StrategyDecision } from "#/types/thesis";
import { CandidateRow } from "#/components/terminal/candidate-row";
import { buildCandidate } from "#/components/terminal/decision-candidate";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { cn } from "#/lib/utils";
import { getWorker } from "#/providers/websocket";
import { Panel } from "@/components/ui/panel";
import { DecisionSideRail, LiveDecisionsEntryLine } from "./decision-side";
import { StrategyDecisionRows } from "./strategy-decisions";

const sameSymbols = (left: string[], right: string[]): boolean =>
	left.length === right.length &&
	left.every((symbol, index) => symbol === right[index]);

const latestBySymbol = <T extends { symbol: string }>(
	rows: T[],
): Record<string, T> => {
	const map: Record<string, T> = {};

	for (const row of rows) {
		map[row.symbol] = row;
	}

	return map;
};

const setText = (node: Element | null | undefined, value: string): void => {
	if (node instanceof HTMLElement) {
		node.textContent = value;
	}
};

const paintDecisionStats = (
	root: HTMLElement | null,
	stats: { field: number; measured: number; inPlay: number; allowed: number },
): void => {
	if (root === null) {
		return;
	}

	setText(root.querySelector("[data-decision='field']"), String(stats.field));
	setText(
		root.querySelector("[data-decision='measured']"),
		String(stats.measured),
	);
	setText(root.querySelector("[data-decision='inPlay']"), String(stats.inPlay));
	setText(
		root.querySelector("[data-decision='allowed']"),
		String(stats.allowed),
	);

	const waiting = root.querySelector("[data-decision='waiting']");

	if (waiting instanceof HTMLElement) {
		waiting.style.display = stats.field === 0 ? "" : "none";
	}
};

const STAT_CARDS = [
	["field", "Field", "evaluated", "text-(--acc)"],
	["measured", "Measured", "signal symbols", "text-(--info)"],
	["inPlay", "In Play", "≥ entry line", "text-(--acc)"],
	["allowed", "Allowed", "edge clears", "text-(--up)"],
] as const;

/*
DecisionsSurface evaluates live ladder candidates. Symbol shells remount only
when the candidate set changes; live scores paint through DOM refs each tick.
*/
export const DecisionsSurface = () => {
	const [selectedSymbol, setSelectedSymbol] = useState<string | null>(null);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const online = useSelector(appStore, (state) => state.online);
	const rootRef = useRef<HTMLDivElement>(null);
	const [symbols, setSymbols] = useState<string[]>([]);

	useDirectStorePaint(
		getWorker(),
		[
			{ store: "decisions", key: "" },
			{ store: "causal", key: "" },
			{ store: "manifold", key: "" },
			{ store: "resonance", key: "" },
			{ store: "measurements", key: "" },
			{ store: "instruments", key: "" },
		],
		(buffers) => {
			const decisions = latestBySymbol(
				(buffers["decisions:"] ?? []) as StrategyDecision[],
			);
			const causal = latestBySymbol(
				(buffers["causal:"] ?? []) as CausalFrame[],
			);
			const manifold = latestBySymbol(
				(buffers["manifold:"] ?? []) as ManifoldFrame[],
			);
			const resonance = latestBySymbol(
				(buffers["resonance:"] ?? []) as ResonanceFrame[],
			);
			const instrumentSymbols = (
				(buffers["instruments:"] ?? []) as Instrument[]
			)
				.map((instrument) => instrument.symbol)
				.sort();
			const symbolSet = new Set(
				[
					...Object.keys(causal),
					...Object.keys(resonance),
					...Object.keys(manifold),
					...Object.keys(decisions),
				].filter((symbol) => symbol.includes("/")),
			);
			const nextSymbols = [
				...instrumentSymbols.filter((symbol) => symbolSet.has(symbol)),
				...[...symbolSet]
					.filter((symbol) => !instrumentSymbols.includes(symbol))
					.sort(),
			];
			const measured = new Set(
				((buffers["measurements:"] ?? []) as Measurement[]).map(
					(measurement) => measurement.symbol,
				),
			).size;
			let inPlay = 0;
			let allowed = 0;

			for (const symbol of nextSymbols) {
				const model = buildCandidate(
					symbol,
					decisions[symbol],
					causal[symbol],
					resonance[symbol],
					manifold[symbol],
				);

				if (model.inPlay) {
					inPlay += 1;
				}

				if (model.verdict === "allow") {
					allowed += 1;
				}
			}

			setSymbols((previous) =>
				sameSymbols(previous, nextSymbols) ? previous : nextSymbols,
			);
			paintDecisionStats(rootRef.current, {
				field: nextSymbols.length,
				measured,
				inPlay,
				allowed,
			});
		},
		[online],
	);

	const current =
		selectedSymbol ??
		(symbols.includes(focusSymbol) ? focusSymbol : symbols[0]);

	return (
		<div
			ref={rootRef}
			className="grid h-full min-h-0 min-w-[1040px] grid-cols-[minmax(640px,1fr)_332px]"
		>
			<div className="min-h-0 overflow-auto px-5 py-[18px]">
				<div className="mb-[18px] grid grid-cols-4 gap-2.5">
					{STAT_CARDS.map(([key, title, subtitle, tone]) => (
						<div
							key={key}
							className="rounded border border-(--line) bg-(--surface) px-3 py-2.5"
						>
							<div className="text-[9.5px] text-(--f4) uppercase tracking-widest">
								{title}
							</div>
							<div
								data-decision={key}
								className={cn(
									"mt-0.5 font-mono font-semibold text-[26px] leading-[1.1]",
									tone,
								)}
							>
								0
							</div>
							<div className="mt-px font-mono text-[9.5px] text-(--f4)">
								{subtitle}
							</div>
						</div>
					))}
				</div>

				<LiveDecisionsEntryLine symbol={current} />

				<div className="mb-2 flex items-baseline justify-between">
					<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Candidate evaluation
					</span>
					<span className="font-mono text-[9.5px] text-(--f4)">
						click a row to inspect attribution + counterfactuals
					</span>
				</div>

				<div className="flex flex-col gap-[7px]">
					<Panel
						variant="surface"
						size="bare"
						data-decision="waiting"
						className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
					>
						waiting for backend decision frames
					</Panel>
					{symbols.map((symbol) => (
						<CandidateRow
							key={symbol}
							symbol={symbol}
							selected={symbol === current}
							onSelect={(next) =>
								setSelectedSymbol(selectedSymbol === next ? null : next)
							}
						/>
					))}
				</div>
			</div>

			<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
				<DecisionSideRail symbol={current} />
				<StrategyDecisionRows symbol={current} />
			</div>
		</div>
	);
};
