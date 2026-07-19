import type { Holding } from "#/collections/types";
import { fixed } from "#/components/terminal/decision-format";
import { cn } from "#/lib/utils";
import type { Finding, LifecycleState } from "#/types/thesis";
import {
	LIFECYCLE_MANAGING,
	LIFECYCLE_STAGES,
	LIFECYCLE_TERMINAL,
	lifecycleStageIndex,
} from "#/types/thesis";
import { badgeVariants } from "@/components/ui/badge";

const stageLabel = (stage: string): string => stage.replaceAll("_", " ");

const stageTone = (stage: LifecycleState, current: LifecycleState): string => {
	if (stage === current) {
		return "bg-[color-mix(in_srgb,var(--acc)_28%,transparent)] text-(--acc) border-[color-mix(in_srgb,var(--acc)_40%,transparent)]";
	}

	const currentIndex = lifecycleStageIndex(current);
	const stageIndex = lifecycleStageIndex(stage);

	if (currentIndex < 0 || stageIndex < 0) {
		return "bg-(--line) text-(--f4) border-transparent";
	}

	if (stageIndex < currentIndex) {
		return "bg-[color-mix(in_srgb,var(--up)_14%,transparent)] text-(--up) border-transparent";
	}

	return "bg-(--sunken) text-(--f4) border-transparent";
};

const badgeVariantFor = (
	state: LifecycleState,
): "success" | "error" | "warning" | "info" => {
	if (LIFECYCLE_TERMINAL.has(state)) {
		if (state === "evaluated") {
			return "success";
		}

		return "error";
	}

	if (LIFECYCLE_MANAGING.has(state)) {
		return "warning";
	}

	return "info";
};

/*
paintLifecycleTrack writes the live lifecycle badge and stage rail into a mounted
LifecycleTrack shell so journal ticks never re-render the React tree.
*/
export const paintLifecycleTrack = (
	root: HTMLElement | null,
	state: LifecycleState,
): void => {
	if (root === null) {
		return;
	}

	const badge = root.querySelector("[data-lifecycle='badge']");

	if (badge instanceof HTMLElement) {
		badge.textContent = stageLabel(state);
		badge.className = cn(
			badgeVariants({ variant: badgeVariantFor(state), size: "xs" }),
		);
	}

	for (const stage of LIFECYCLE_STAGES) {
		const node = root.querySelector(`[data-lifecycle-stage='${stage}']`);

		if (!(node instanceof HTMLElement)) {
			continue;
		}

		node.className = cn(
			"rounded-[2px] border px-1 py-px font-mono text-[8px] uppercase tracking-wide",
			stageTone(stage, state),
		);
	}
};

/*
LifecycleTrack renders one symbol's backend lifecycle state against the ordered
stage rail so progress is visible without inferring it from execution frames.
*/
export const LifecycleTrack = ({
	symbol,
	state = "observing",
}: {
	symbol: string;
	state?: LifecycleState;
}) => (
	<div
		data-lifecycle-track={symbol}
		className="rounded border border-(--line) bg-(--surface) px-3 py-2.5"
	>
		<div className="mb-2 flex items-center justify-between gap-2">
			<span className="font-mono font-semibold text-[12px] text-(--f1)">
				{symbol}
			</span>
			<span
				data-lifecycle="badge"
				className={cn(
					badgeVariants({ variant: badgeVariantFor(state), size: "xs" }),
				)}
			>
				{stageLabel(state)}
			</span>
		</div>
		<div className="flex flex-wrap gap-1">
			{LIFECYCLE_STAGES.map((stage) => (
				<span
					key={stage}
					data-lifecycle-stage={stage}
					title={stageLabel(stage)}
					className={cn(
						"rounded-[2px] border px-1 py-px font-mono text-[8px] uppercase tracking-wide",
						stageTone(stage, state),
					)}
				>
					{stage.split("_")[0]}
				</span>
			))}
		</div>
	</div>
);

const isOpenLot = (holding: Holding): boolean =>
	holding.qty > 0 &&
	holding.status !== "closed" &&
	holding.status !== "canceled";

const setText = (node: Element | null | undefined, value: string): void => {
	if (node instanceof HTMLElement) {
		node.textContent = value;
	}
};

const paintHoldingRow = (row: HTMLElement, holding: Holding): void => {
	setText(row.querySelector("[data-journal='holding-symbol']"), holding.symbol);
	setText(
		row.querySelector("[data-journal='holding-meta']"),
		[
			`qty ${fixed(holding.qty)}`,
			`mark ${fixed(holding.mark)}`,
			`pnl ${fixed(holding.pnl)}`,
			isOpenLot(holding) ? "" : "closed",
		]
			.filter(Boolean)
			.join(" · "),
	);

	const status = row.querySelector("[data-journal='holding-status']");

	if (!(status instanceof HTMLElement)) {
		return;
	}

	status.textContent =
		typeof holding.status === "string" ? holding.status : "unknown";
	status.className = cn(
		badgeVariants({
			variant: isOpenLot(holding) ? "success" : "info",
			size: "xs",
		}),
	);
};

const paintFindingCard = (card: HTMLElement, finding: Finding): void => {
	setText(
		card.querySelector("[data-journal='finding-component']"),
		finding.component,
	);
	setText(
		card.querySelector("[data-journal='finding-unc']"),
		`±${finding.uncertainty.toFixed(3)} unc`,
	);
	setText(
		card.querySelector("[data-journal='finding-condition']"),
		finding.condition,
	);
	setText(
		card.querySelector("[data-journal='finding-effect']"),
		finding.estimatedEffect.toFixed(4),
	);

	const fill = card.querySelector("[data-journal='finding-fill']");

	if (fill instanceof HTMLElement) {
		fill.style.width = `${Math.min(100, Math.round(Math.abs(finding.estimatedEffect) * 100))}%`;
		fill.style.background =
			finding.estimatedEffect >= 0 ? "var(--up)" : "var(--down)";
	}

	const adjustment = card.querySelector("[data-journal='finding-adjustment']");

	if (adjustment instanceof HTMLElement) {
		const text = finding.proposedAdjustment
			? `→ ${finding.proposedAdjustment}`
			: "";

		adjustment.textContent = text;
		adjustment.style.display = text === "" ? "none" : "";
	}

	const evidence = card.querySelector("[data-journal='finding-evidence']");

	if (evidence instanceof HTMLElement) {
		evidence.replaceChildren(
			...finding.evidence.map((line) => {
				const item = document.createElement("li");

				item.className = "border-(--line) border-l pl-2";
				item.textContent = line;

				return item;
			}),
		);
	}

	setText(
		card.querySelector("[data-journal='finding-validate']"),
		`validate · ${finding.requiredValidation}`,
	);
};

/*
paintJournalSurface writes lifecycle rails, holdings lots, and postmortem
findings into mounted shells so websocket cadence never re-renders JournalSurface.
*/
export const paintJournalSurface = (
	root: HTMLElement | null,
	input: {
		activeSymbol: string | null;
		findings: Finding[];
		findingKeys: string[];
		holdings: Holding[];
		holdingKeys: string[];
		lifecycleBySymbol: Record<string, string>;
		online: boolean;
		symbols: string[];
	},
): void => {
	if (root === null) {
		return;
	}

	const {
		activeSymbol,
		findings,
		findingKeys,
		holdings,
		holdingKeys,
		lifecycleBySymbol,
		online,
		symbols,
	} = input;

	setText(
		root.querySelector("[data-journal='lifecycle-meta']"),
		`${symbols.length} symbol${symbols.length === 1 ? "" : "s"}`,
	);

	const lifecycleEmpty = root.querySelector("[data-journal='lifecycle-empty']");

	if (lifecycleEmpty instanceof HTMLElement) {
		lifecycleEmpty.style.display = symbols.length === 0 ? "" : "none";
		lifecycleEmpty.textContent = online
			? "no active lifecycle"
			: "waiting for lifecycle frames";
	}

	for (const symbol of symbols) {
		paintLifecycleTrack(
			root.querySelector(`[data-lifecycle-track='${symbol}']`) as
				| HTMLElement
				| null,
			lifecycleBySymbol[symbol] ?? "observing",
		);
	}

	const filteredHoldings = (
		activeSymbol
			? holdings.filter((holding) => holding.symbol === activeSymbol)
			: holdings
	).sort((left, right) => left.symbol.localeCompare(right.symbol));

	setText(
		root.querySelector("[data-journal='holdings-meta']"),
		`${activeSymbol ?? "all symbols"} · ${filteredHoldings.length} lots`,
	);

	const holdingsEmpty = root.querySelector("[data-journal='holdings-empty']");

	if (holdingsEmpty instanceof HTMLElement) {
		holdingsEmpty.style.display = filteredHoldings.length === 0 ? "" : "none";
		holdingsEmpty.textContent = online
			? "no holdings retained"
			: "waiting for position frames";
	}

	const holdingsByKey = new Map(
		holdings.map((holding) => [
			`${holding.symbol}:${String(holding.status)}:${holding.qty}`,
			holding,
		]),
	);

	for (const key of holdingKeys) {
		const row = root.querySelector(`[data-holding='${CSS.escape(key)}']`);

		if (!(row instanceof HTMLElement)) {
			continue;
		}

		const holding = holdingsByKey.get(key);

		if (holding === undefined) {
			row.style.display = "none";
			continue;
		}

		const visible = !activeSymbol || holding.symbol === activeSymbol;

		row.style.display = visible ? "" : "none";

		if (visible) {
			paintHoldingRow(row, holding);
		}
	}

	const activeFindings = activeSymbol
		? findings.filter((finding) => finding.symbol === activeSymbol)
		: findings;

	setText(
		root.querySelector("[data-journal='findings-meta']"),
		`${activeFindings.length} finding${activeFindings.length === 1 ? "" : "s"}`,
	);

	const findingsEmpty = root.querySelector("[data-journal='findings-empty']");

	if (findingsEmpty instanceof HTMLElement) {
		findingsEmpty.style.display = activeFindings.length === 0 ? "" : "none";
		findingsEmpty.textContent = online
			? "no postmortem findings"
			: "waiting for findings frames";
	}

	const findingsByKey = new Map(
		findings.map((finding) => [
			`${finding.symbol}:${finding.component}:${finding.condition}`,
			finding,
		]),
	);

	for (const key of findingKeys) {
		const card = root.querySelector(`[data-finding='${CSS.escape(key)}']`);

		if (!(card instanceof HTMLElement)) {
			continue;
		}

		const finding = findingsByKey.get(key);

		if (finding === undefined) {
			card.style.display = "none";
			continue;
		}

		const visible = !activeSymbol || finding.symbol === activeSymbol;

		card.style.display = visible ? "" : "none";

		if (visible) {
			paintFindingCard(card, finding);
		}
	}
};
