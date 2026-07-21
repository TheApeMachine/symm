import { createRef } from "react";
import { appStore } from "#/collections/app";
import type {
	CausalFrame,
	Instrument,
	ManifoldFrame,
	Measurement,
	ResonanceFrame,
} from "#/collections/types";
import {
	paintCandidateSelection,
	syncCandidateRowShells,
} from "#/components/terminal/candidate-row";
import { buildCandidate } from "#/components/terminal/decision-candidate";
import {
	DecisionSideRail,
	LiveDecisionsEntryLine,
	setDecisionsScopeSymbol,
} from "#/components/terminal/decision-side";
import { StrategyDecisionRows } from "#/components/terminal/strategy-decisions";
import { cn } from "#/lib/utils";
import { frameRows } from "#/providers/frame-history";
import type { StrategyDecision } from "#/types/thesis";
import { Panel } from "@/components/ui/panel";

const rootRef = createRef<HTMLDivElement>();

let lastDecisions: StrategyDecision[] = [];
let lastCausal: CausalFrame[] = [];
let lastManifold: ManifoldFrame[] = [];
let lastResonance: ResonanceFrame[] = [];
let lastMeasurements: Measurement[] = [];
let lastInstruments: Instrument[] = [];
let selectedSymbol: string | null = null;

const asRows = <T,>(value: unknown): T[] =>
	(Array.isArray(value) ? value : value != null ? [value] : []) as T[];

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

const selectCandidate = (symbol: string) => {
	selectedSymbol = selectedSymbol === symbol ? null : symbol;
	paint();
};

/*
paintDecisionStats writes field / measured / in-play / allowed into the shell.
*/
export const paintDecisionStats = (
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
paint rebuilds decision stats from module caches and paints the shell.
*/
const paint = () => {
	const root = rootRef.current;

	if (root === null) {
		return;
	}

	const decisions = latestBySymbol(lastDecisions);
	const causal = latestBySymbol(lastCausal);
	const manifold = latestBySymbol(lastManifold);
	const resonance = latestBySymbol(lastResonance);
	const instrumentSymbols = lastInstruments
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
		lastMeasurements.map((measurement) => measurement.symbol),
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

	const focusSymbol = appStore.state.focusSymbol;
	const currentSymbol =
		selectedSymbol ??
		(nextSymbols.includes(focusSymbol) ? focusSymbol : nextSymbols[0]);

	setDecisionsScopeSymbol(currentSymbol);
	syncCandidateRowShells(root, nextSymbols, selectCandidate);
	paintCandidateSelection(selectedSymbol);
	paintDecisionStats(root, {
		field: nextSymbols.length,
		measured,
		inPlay,
		allowed,
	});
};

/*
paintDecisions refreshes decision stats from the current DRAW decisions batch.
*/
export const paintDecisions = (value: unknown, _focusSymbol: string) => {
	lastDecisions = asRows<StrategyDecision>(value);
	paint();
};

/*
paintDecisionsCausal refreshes decision stats from the current DRAW causal batch.
*/
export const paintDecisionsCausal = (value: unknown, _focusSymbol: string) => {
	lastCausal = asRows<CausalFrame>(value);
	paint();
};

/*
paintDecisionsManifold refreshes decision stats from the current DRAW manifold batch.
*/
export const paintDecisionsManifold = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastManifold = asRows<ManifoldFrame>(value);
	paint();
};

/*
paintDecisionsResonance refreshes decision stats from the current DRAW resonance batch.
*/
export const paintDecisionsResonance = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastResonance = asRows<ResonanceFrame>(value);
	paint();
};

/*
paintDecisionsMeasurements refreshes decision coverage from each entity's
latest retained measurement.
*/
export const paintDecisionsMeasurements = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastMeasurements = frameRows<Measurement>(value);
	paint();
};

/*
paintDecisionsInstruments refreshes decision stats from the current DRAW instruments.
*/
export const paintDecisionsInstruments = (
	value: unknown,
	_focusSymbol: string,
) => {
	lastInstruments = asRows<Instrument>(value);
	paint();
};

/*
DecisionsSurface is the static candidate-ladder shell. DRAW paints stats via
paintDecisions* exports without React reconciliation each tick.
*/
export const DecisionsSurface = () => (
	<div
		ref={rootRef}
		className="grid h-full min-h-0 min-w-260 grid-cols-[minmax(640px,1fr)_332px]"
	>
		<div className="min-h-0 overflow-auto px-5 py-4.5">
			<div className="mb-4.5 grid grid-cols-4 gap-2.5">
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

			<LiveDecisionsEntryLine />

			<div className="mb-2 flex items-baseline justify-between">
				<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Candidate evaluation
				</span>
				<span className="font-mono text-[9.5px] text-(--f4)">
					click a row to inspect attribution + counterfactuals
				</span>
			</div>

			<div className="flex flex-col gap-1.75" data-decision-host="candidates">
				<Panel
					variant="surface"
					size="bare"
					data-decision="waiting"
					className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
				>
					waiting for backend decision frames
				</Panel>
			</div>
		</div>

		<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
			<DecisionSideRail />
			<StrategyDecisionRows />
		</div>
	</div>
);
