import type {
	CausalFrame,
	ManifoldFrame,
	ResonanceFrame,
} from "#/collections/types";
import { writeCandidateRow } from "#/components/terminal/candidate-paint";
import { buildCandidate } from "#/components/terminal/decision-candidate";
import { cn } from "#/lib/utils";
import type { StrategyDecision } from "#/types/thesis";
import { meterTrackVariants } from "@/components/ui/meter";

export const candidateRows = new Map<string, HTMLButtonElement>();

const lastDecisions: Record<string, StrategyDecision | undefined> = {};
const lastCausal: Record<string, CausalFrame | undefined> = {};
const lastManifold: Record<string, ManifoldFrame | undefined> = {};
const lastResonance: Record<string, ResonanceFrame | undefined> = {};

const paintBound = (symbol: string) => {
	const root = candidateRows.get(symbol);

	if (root === undefined) {
		return;
	}

	writeCandidateRow(
		root,
		buildCandidate(
			symbol,
			lastDecisions[symbol],
			lastCausal[symbol],
			lastResonance[symbol],
			lastManifold[symbol],
		),
	);
};

const mergeAndPaint = <T extends { symbol: string }>(
	value: unknown,
	sink: Record<string, T | undefined>,
) => {
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as T[];

	for (const row of rows) {
		sink[row.symbol] = row;
		paintBound(row.symbol);
	}
};

const createInlineMeterShell = (src: string): HTMLElement => {
	const bar = document.createElement("div");
	bar.dataset.candidateBar = src;
	bar.className = "flex min-w-0 items-center gap-2 font-mono text-[9px]";
	bar.style.display = "none";

	const label = document.createElement("span");
	label.className = "w-16 text-(--f4)";
	label.textContent = src;

	const track = document.createElement("div");
	track.dataset.meter = "track";
	track.dataset.meterLayout = "inline";
	track.className = cn(
		meterTrackVariants({ variant: "info", size: "s" }),
		"flex-1",
	);

	const fill = document.createElement("div");
	fill.dataset.meter = "fill";
	fill.className = "h-full bg-(--meter-tone)";
	fill.style.width = "0%";
	track.append(fill);

	const value = document.createElement("span");
	value.dataset.meter = "value";
	value.className = "w-[30px] text-(--f3)";

	bar.append(label, track, value);

	return bar;
};

/*
createCandidateRowShell builds one candidate ladder button with the markers
writeCandidateRow expects.
*/
const createCandidateRowShell = (
	symbol: string,
	onSelect: (symbol: string) => void,
): HTMLButtonElement => {
	const button = document.createElement("button");
	button.type = "button";
	button.dataset.candidate = symbol;
	button.dataset.symbol = symbol;
	button.className =
		"cursor-pointer overflow-hidden rounded border border-(--line) bg-(--surface) text-left font-[inherit]";
	button.addEventListener("click", () => onSelect(symbol));

	const grid = document.createElement("div");
	grid.className =
		"grid grid-cols-[78px_1fr_132px_92px] items-center gap-3 px-3 py-2.5";

	const symbolCol = document.createElement("div");
	const symbolLabel = document.createElement("div");
	symbolLabel.className = "font-mono font-semibold text-[13px] text-(--f1)";
	symbolLabel.textContent = symbol;

	const support = document.createElement("div");
	support.dataset.candidate = "support";
	support.className = "font-mono text-[9px] text-(--f4)";
	symbolCol.append(symbolLabel, support);

	const barsCol = document.createElement("div");
	barsCol.className = "flex min-w-0 flex-col gap-1 font-mono text-[9px]";

	const barsWaiting = document.createElement("div");
	barsWaiting.dataset.candidate = "bars-waiting";
	barsWaiting.className = "text-(--f4)";
	barsWaiting.textContent = "waiting for ladder frames";
	barsCol.append(
		barsWaiting,
		createInlineMeterShell("causal"),
		createInlineMeterShell("predict"),
		createInlineMeterShell("manifold"),
	);

	const scoreCol = document.createElement("div");
	const score = document.createElement("div");
	score.dataset.candidate = "score";

	const scoreHeader = document.createElement("div");
	scoreHeader.className =
		"mb-1 flex justify-between font-mono text-[9.5px] text-(--f4)";

	const scoreLabel = document.createElement("span");
	scoreLabel.dataset.meter = "label";
	scoreLabel.textContent = "combined";

	const scoreValue = document.createElement("span");
	scoreValue.dataset.meter = "value";
	scoreValue.className = "text-(--f1)";
	scoreHeader.append(scoreLabel, scoreValue);

	const scoreTrack = document.createElement("div");
	scoreTrack.dataset.meter = "track";
	scoreTrack.dataset.meterSize = "m";
	scoreTrack.className = cn(meterTrackVariants({ variant: "info", size: "m" }));

	const scoreFill = document.createElement("div");
	scoreFill.dataset.meter = "fill";
	scoreFill.className = "h-full bg-(--meter-tone)";
	scoreFill.style.width = "0%";
	scoreTrack.append(scoreFill);
	score.append(scoreHeader, scoreTrack);

	const edge = document.createElement("div");
	edge.dataset.candidate = "edge";
	edge.className = "mt-1 font-mono text-[9px]";
	scoreCol.append(score, edge);

	const verdictCol = document.createElement("div");
	verdictCol.className = "text-right";

	const verdict = document.createElement("span");
	verdict.dataset.candidate = "verdict";

	const why = document.createElement("div");
	why.dataset.candidate = "why";
	why.className = "mt-1 font-mono text-[9px] text-(--f4)";
	verdictCol.append(verdict, why);

	grid.append(symbolCol, barsCol, scoreCol, verdictCol);
	button.append(grid);

	const detail = document.createElement("div");
	detail.dataset.candidate = "detail";
	detail.className =
		"grid grid-cols-2 gap-5 border-(--line) border-t bg-(--sunken) px-3.5 py-3 font-mono text-[9.5px]";
	detail.style.display = "none";

	const attribution = document.createElement("div");
	const attributionTitle = document.createElement("div");
	attributionTitle.className =
		"mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-widest";
	attributionTitle.textContent = "Score attribution";

	const waterfall = document.createElement("div");
	waterfall.className = "flex flex-col gap-1.5";

	for (const src of ["causal", "predict", "field"] as const) {
		const row = document.createElement("div");
		row.dataset.waterfall = src;
		row.className = "flex items-center gap-2";

		const label = document.createElement("span");
		label.className = "w-[60px] text-(--f4)";
		label.textContent = src;

		const track = document.createElement("div");
		track.className = "relative h-3 flex-1 rounded-sm bg-(--line)";

		const midline = document.createElement("div");
		midline.className = "absolute top-0 bottom-0 left-1/2 w-px bg-(--f4)";

		const bar = document.createElement("div");
		bar.dataset.waterfall = "bar";
		bar.className = "absolute top-px bottom-px rounded-[1px]";

		const delta = document.createElement("span");
		delta.dataset.waterfall = "delta";
		delta.className = "w-[50px] text-right";

		track.append(midline, bar);
		row.append(label, track, delta);
		waterfall.append(row);
	}

	const branch = document.createElement("div");
	branch.dataset.candidate = "branch";
	branch.className = "mt-2 text-[9px] text-(--f4)";
	attribution.append(attributionTitle, waterfall, branch);

	const probesCol = document.createElement("div");
	const probesTitle = document.createElement("div");
	probesTitle.className =
		"mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-widest";
	probesTitle.textContent = "Counterfactual probes · do(·)";

	const probes = document.createElement("div");
	probes.className = "flex flex-col gap-1.5";

	for (const label of ["beta", "panic", "residual", "intervention"]) {
		const row = document.createElement("div");
		row.className =
			"flex items-center justify-between gap-2 rounded-sm border border-(--line) bg-(--surface) px-2 py-1.5";

		const name = document.createElement("span");
		name.className = "text-(--f2)";
		name.textContent = label;

		const value = document.createElement("span");
		value.dataset.probe = label;
		value.className = "text-(--f1)";

		row.append(name, value);
		probes.append(row);
	}

	probesCol.append(probesTitle, probes);
	detail.append(attribution, probesCol);
	button.append(detail);

	candidateRows.set(symbol, button);
	paintBound(symbol);

	return button;
};

/*
syncCandidateRowShells creates or removes candidate rows when symbols appear.
*/
export const syncCandidateRowShells = (
	root: HTMLElement | null,
	symbols: string[],
	onSelect: (symbol: string) => void,
): void => {
	if (root === null) {
		return;
	}

	const host = root.querySelector("[data-decision-host='candidates']");

	if (!(host instanceof HTMLElement)) {
		return;
	}

	const next = new Set(symbols);
	const ordered: HTMLElement[] = [];

	for (const symbol of symbols) {
		let row = candidateRows.get(symbol);

		if (row === undefined) {
			row = createCandidateRowShell(symbol, onSelect);
		}

		ordered.push(row);
	}

	for (const symbol of [...candidateRows.keys()]) {
		if (next.has(symbol)) {
			continue;
		}

		candidateRows.get(symbol)?.remove();
		candidateRows.delete(symbol);
	}

	const waiting = host.querySelector("[data-decision='waiting']");
	const currentRows = Array.from(host.children).filter((child) => child !== waiting);
	const orderMatches =
		ordered.length === currentRows.length &&
		ordered.every((row, index) => currentRows[index] === row);

	if (!orderMatches) {
		host.replaceChildren(...(waiting ? [waiting] : []), ...ordered);
	}
};

/*
paintCandidateSelection toggles row chrome and detail panels for the active symbol.
*/
export const paintCandidateSelection = (
	selectedSymbol: string | null,
): void => {
	for (const [symbol, row] of candidateRows) {
		const selected = symbol === selectedSymbol;

		row.className = cn(
			"cursor-pointer overflow-hidden rounded border bg-(--surface) text-left font-[inherit]",
			selected
				? "border-[color-mix(in_srgb,var(--up)_30%,transparent)]"
				: "border-(--line)",
		);

		const detail = row.querySelector("[data-candidate='detail']");

		if (detail instanceof HTMLElement) {
			detail.style.display = selected ? "" : "none";
		}
	}
};

/*
paintCandidateDecisions merges the DRAW decisions batch into lastDecisions and
repaints every bound CandidateRow whose symbol appears in the batch.
*/
export const paintCandidateDecisions = (
	value: unknown,
	_focusSymbol: string,
) => {
	mergeAndPaint<StrategyDecision>(value, lastDecisions);
};

/*
paintCandidateCausal merges the DRAW causal batch into lastCausal and repaints
matching bound CandidateRow shells.
*/
export const paintCandidateCausal = (value: unknown, _focusSymbol: string) => {
	mergeAndPaint<CausalFrame>(value, lastCausal);
};

/*
paintCandidateManifold merges the DRAW manifold batch into lastManifold and
repaints matching bound CandidateRow shells.
*/
export const paintCandidateManifold = (
	value: unknown,
	_focusSymbol: string,
) => {
	mergeAndPaint<ManifoldFrame>(value, lastManifold);
};

/*
paintCandidateResonance merges the DRAW resonance batch into lastResonance and
repaints matching bound CandidateRow shells.
*/
export const paintCandidateResonance = (
	value: unknown,
	_focusSymbol: string,
) => {
	mergeAndPaint<ResonanceFrame>(value, lastResonance);
};
