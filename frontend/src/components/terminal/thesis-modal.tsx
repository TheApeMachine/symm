import { terminalStore } from "#/collections/terminal";
import type { Holding, TickFrame } from "#/collections/types";
import { ThesisDetailRail } from "#/components/terminal/thesis-detail-rail";
import { writeThesisDetailRail } from "#/components/terminal/thesis-detail-paint";
import {
	paintThesisEvidence,
	ThesisEvidenceCanvas,
} from "#/components/terminal/thesis-evidence-canvas";
import {
	accumulateThesisSnapshot,
	thesisSnapshotFor,
} from "#/components/terminal/thesis-snapshot";
import { cn } from "#/lib/utils";
import type { Category, Measurement } from "#/types/measurement";
import type {
	Finding,
	Graph,
	StrategyDecision,
	ThesisForecast,
	ThesisHypothesis,
} from "#/types/thesis";
import { badgeVariants } from "@/components/ui/badge";

type ThesisParts = {
	root: HTMLElement;
	symbol: HTMLElement | null;
	lifecycle: HTMLElement | null;
	meta: HTMLElement | null;
	rail: HTMLElement | null;
};

let thesis: ThesisParts | null = null;
let retained: ReturnType<typeof thesisSnapshotFor> | null = null;

let measurements: Measurement[] = [];
let forecasts: ThesisForecast[] = [];
let hypotheses: ThesisHypothesis[] = [];
let categories: Array<Category & { symbol?: string }> = [];
let decisions: StrategyDecision[] = [];
let lifecycle: Array<{ symbol: string; state: string }> = [];
let holdings: Holding[] = [];
let findings: Finding[] = [];
let graphs: Graph[] = [];
let tick: TickFrame | undefined;

const asRows = <T,>(value: unknown): T[] =>
	(Array.isArray(value)
		? value
		: value !== null && typeof value === "object"
			? Object.values(value as Record<string, T>)
			: value != null
				? [value]
				: []) as T[];

/*
bindThesis caches chrome/rail nodes once. Open/close is painted from click
handlers and DRAW via paintThesis — no store subscription.
*/
export const bindThesis = (root: HTMLElement | null) => {
	if (root === null) {
		thesis = null;
		return;
	}

	thesis = {
		root,
		symbol: root.querySelector('[data-thesis="symbol"]'),
		lifecycle: root.querySelector('[data-thesis="lifecycle"]'),
		meta: root.querySelector('[data-thesis="meta"]'),
		rail: root.querySelector('[data-thesis="rail"]'),
	};
	root.hidden = true;
	paintThesis({}, "");
};

/*
paintThesis is the only DRAW path for this modal: merge the frame, then paint
the open shell (or hide it).
*/
export const paintThesis = (
	frame: Record<string, unknown>,
	_focusSymbol: string,
) => {
	if (frame.measurements !== undefined) {
		measurements = asRows<Measurement>(frame.measurements);
	}

	if (frame.forecasts !== undefined) {
		forecasts = asRows<ThesisForecast>(frame.forecasts);
	}

	if (frame.hypotheses !== undefined) {
		hypotheses = asRows<ThesisHypothesis>(frame.hypotheses);
	}

	if (frame.categories !== undefined) {
		categories = asRows<Category & { symbol?: string }>(frame.categories);
	}

	if (frame.decisions !== undefined) {
		decisions = asRows<StrategyDecision>(frame.decisions);
	}

	if (frame.lifecycle !== undefined) {
		lifecycle = asRows<{ symbol: string; state: string }>(frame.lifecycle);
	}

	if (frame.holdings !== undefined) {
		holdings = asRows<Holding>(frame.holdings);
	}

	if (frame.findings !== undefined) {
		findings = asRows<Finding>(frame.findings);
	}

	if (frame.graphs !== undefined) {
		graphs = asRows<Graph>(frame.graphs);
	}

	if (frame.tick !== undefined) {
		tick = asRows<TickFrame>(frame.tick).at(-1);
	}

	if (thesis === null) {
		return;
	}

	const symbol = terminalStore.state.thesisSymbol;

	if (symbol === null) {
		retained = null;
		thesis.root.hidden = true;
		return;
	}

	const count = tick?.count;
	const next = accumulateThesisSnapshot(
		retained,
		thesisSnapshotFor({
			symbol,
			tick: typeof count === "number" && Number.isFinite(count) ? count : null,
			lifecycle: lifecycle.find((row) => row.symbol === symbol)?.state ?? null,
			graph: graphs.find((frame) => frame.symbol === symbol) ?? null,
			measurements,
			decision:
				decisions.find((decision) => decision.symbol === symbol) ?? null,
			forecasts,
			hypotheses,
			categories,
			holdings,
			findings,
		}),
	);
	retained = next;

	if (thesis.symbol !== null) {
		thesis.symbol.textContent = next.symbol;
	}

	if (thesis.lifecycle !== null) {
		thesis.lifecycle.textContent = next.lifecycle ?? "observing";
		thesis.lifecycle.className = badgeVariants({
			variant: "info",
			size: "xs",
		});
	}

	if (thesis.meta !== null) {
		thesis.meta.textContent = `thesis carrier · tick ${next.tick ?? "—"} · ${next.measurements.length} measurements · ${next.graph?.nodes.length ?? 0} graph nodes`;
	}

	if (thesis.rail !== null) {
		writeThesisDetailRail(thesis.rail, next);
	}

	paintThesisEvidence(graphs, symbol);
	thesis.root.hidden = false;
};

/*
openThesisShell opens the thesis carrier and paints immediately.
*/
export const openThesisShell = (symbol: string) => {
	terminalStore.actions.openThesis(symbol);
	paintThesis({}, "");
};

/*
closeThesisShell closes the thesis carrier and paints immediately.
*/
export const closeThesisShell = () => {
	terminalStore.actions.closeThesis();
	paintThesis({}, "");
};

/*
ThesisModal is the static shell. ref={bindThesis}, DRAW → paintThesis.
*/
export const ThesisModal = () => (
	<div
		ref={bindThesis}
		hidden
		className="absolute inset-0 z-20 flex animate-[symFade_0.18s_ease] items-center justify-center bg-[color-mix(in_srgb,var(--sunken)_52%,transparent)] p-6 backdrop-blur-sm"
	>
		<button
			type="button"
			aria-label="Close thesis modal"
			className="absolute inset-0"
			onClick={closeThesisShell}
		/>
		<div
			className={cn(
				"pointer-events-auto relative z-10 flex h-[min(88vh,920px)] w-[min(1180px,96vw)] flex-col overflow-hidden",
				"rounded-[8px] border border-(--line2) bg-(--surface) shadow-[0_28px_72px_-18px_rgba(0,0,0,0.78)]",
			)}
		>
			<div className="flex shrink-0 items-start justify-between gap-3 border-(--line) border-b px-5 py-4">
				<div className="min-w-0">
					<div className="flex flex-wrap items-center gap-2">
						<span
							data-thesis="symbol"
							className="font-serif font-semibold text-[24px] text-(--f1) leading-[1.05]"
						/>
						<span data-thesis="lifecycle" />
					</div>
					<div
						data-thesis="meta"
						className="mt-1 font-mono text-[10px] text-(--f4)"
					/>
				</div>
				<button
					type="button"
					onClick={closeThesisShell}
					className="flex size-[28px] shrink-0 cursor-pointer items-center justify-center rounded-[3px] border border-(--line) bg-(--raised) p-0 text-(--f3) hover:border-(--line2) hover:text-(--f1)"
				>
					<svg
						width="13"
						height="13"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						strokeWidth="2"
						aria-hidden="true"
					>
						<path d="M6 6l12 12M18 6L6 18" />
					</svg>
				</button>
			</div>

			<div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1.55fr)_minmax(280px,360px)]">
				<div className="relative min-h-0 border-(--line) border-r">
					<ThesisEvidenceCanvas />
					<div className="pointer-events-none absolute top-3.5 left-4">
						<div className="font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">
							Evidence graph
						</div>
						<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
							measurement nodes · typed Gonum relationships
						</div>
					</div>
				</div>
				<div className="min-h-0 overflow-y-auto p-3.5">
					<ThesisDetailRail />
				</div>
			</div>
		</div>
	</div>
);
