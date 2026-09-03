import { type MouseEvent, useRef } from "react";
import { strategyStore } from "#/collections/app";
import {
	EvidenceStage,
	ExecutionStage,
	StructuralStage,
} from "#/components/terminal/decision-chain-stages";
import { DecisionMCTSStage } from "#/components/terminal/decision-mcts-stage";
import { setDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { Typography } from "#/components/ui/typography";
import { Decision } from "#/providers/telemetry/telemetry/decision";

const selectRow = (row: HTMLElement, symbol: string): void => {
	setDecisionsScopeSymbol(symbol);

	for (const other of document.querySelectorAll<HTMLElement>(
		"[data-decision-chain='row']",
	)) {
		const selected = other === row;
		other.dataset.selected = String(selected);
		other.setAttribute("aria-expanded", String(selected));
	}
};

const decObj = new Decision();

/*
FRAME_SCAN must match the surface's retention window so a row's pinned frame
index resolves to the same frame the surface indexed it from.
*/
const FRAME_SCAN = 50;

const decisionToThesis = (d: Decision) => ({
	id: d.id() ?? "",
	action: d.action() ?? "",
	symbol: d.symbol() ?? "",
	direction: Number(d.direction()),
	thesisScore: d.thesisScore(),
	thesisConfidence: d.thesisConfidence(),
	thesisSupport: d.thesisSupport(),
	thesisContradiction: d.thesisContradiction(),
	thesisConditions: d.thesisConditions(),
	predictiveStatus: "",
	taskSkill: d.taskSkill(),
	forecastHorizon: Number(d.forecastHorizon()),
	graphScore: d.graphScore(),
	confidence: d.confidence(),
	reason: d.reason() ?? "",
	cause: d.reason() ?? "",
});

export const DecisionChain = ({
	frame,
	index,
}: {
	frame: number;
	index: number;
}) => {
	const rowRef = useRef<HTMLButtonElement>(null);

	strategyStore.subscribe((state) => {
		// The row is pinned to the frame its symbol was last decided in, so a
		// later frame about a different symbol cannot repaint it.
		const frames = state.getLastN(FRAME_SCAN);
		const source = frames[frame];
		if (!source || index >= source.decisionsLength()) return;

		const current = source.decisions(index, decObj);
		if (!current) return;

		const row = rowRef.current;
		if (!row) return;

		// Only repaint from a frame that actually carries this row's symbol.
		// Decision indices are per-frame positions, not stable identities, so
		// a frame that omits this symbol would otherwise paint a different
		// symbol's numbers into this row.
		const painted =
			row.querySelector<HTMLElement>('[data-df="symbol"]')?.textContent ?? "";
		const incoming = current.symbol() ?? "";

		if (painted !== "" && incoming !== painted) {
			return;
		}

		const set = (q: string, value: string) => {
			const el = row.querySelector<HTMLElement>(`[data-df="${q}"]`);
			if (el) el.textContent = value;
		};

		set("symbol", incoming);
		set("reason", current.reason() ?? "");
		set("thesisScore", current.thesisScore().toFixed(4));
		set("thesisConfidence", `${(current.confidence() * 100).toFixed(1)}%`);
		set("graphScore", current.graphScore().toFixed(5));
		set("action", current.action() ?? "—");
		set("cause", current.reason() ?? "pending");
	});

	// Resolve against the frame this row is pinned to, not the newest one.
	// Reading getLast here made every row render the most recent frame's
	// decision, so a board of symbols all showed the same one.
	const source = strategyStore.state.getLastN(FRAME_SCAN)[frame];
	const decision =
		source && index < source.decisionsLength()
			? source.decisions(index, new Decision())
			: null;

	if (!decision) {
		return null;
	}

	const selectDecision = (event: MouseEvent<HTMLButtonElement>): void => {
		selectRow(event.currentTarget, decision.symbol() ?? "");
	};

	const thesisDec = decisionToThesis(decision);

	// Seed the paint targets from the frame already in hand. The subscription
	// above repaints them on every later frame, but until one arrives the row
	// would otherwise render as an empty outline.
	const seed = {
		symbol: decision.symbol() ?? "",
		reason: decision.reason() ?? "",
		thesisScore: decision.thesisScore().toFixed(4),
		thesisConfidence: `${(decision.confidence() * 100).toFixed(1)}%`,
		graphScore: decision.graphScore().toFixed(5),
		action: decision.action() ?? "—",
	};

	return (
		<button
			ref={rowRef}
			type="button"
			data-index={index}
			data-decision-chain="row"
			data-selected="false"
			aria-expanded="false"
			onClick={selectDecision}
			className="group w-full cursor-pointer overflow-hidden rounded border border-(--line) bg-(--surface) text-left font-[inherit] transition-colors data-[selected=true]:border-[color-mix(in_srgb,var(--acc)_45%,transparent)]"
		>
			<div className="flex items-start justify-between gap-3 border-(--line) border-b px-3 py-2">
				<div className="min-w-0">
					<Typography.Span
						data-df="symbol"
						data-decision-chain="symbol"
						className="font-mono font-semibold text-[13px] text-(--f1)"
					>
						{seed.symbol}
					</Typography.Span>
					<Typography.Span
						data-df="reason"
						className="mt-0.5 block truncate font-mono text-[9px] text-(--f4)"
					>
						{seed.reason}
					</Typography.Span>
				</div>
				<div className="flex shrink-0 items-center gap-2 font-mono">
					<span className="text-[9px] text-(--f4)">
						thesis=
						<b data-df="thesisScore" className="font-normal text-(--acc)">
							{seed.thesisScore}
						</b>
					</span>
					<span className="text-[9px] text-(--f4)">
						conf=
						<b data-df="thesisConfidence" className="font-normal text-(--f2)">
							{seed.thesisConfidence}
						</b>
					</span>
					<span className="text-[9px] text-(--f4)">
						graph=
						<b data-df="graphScore" className="font-normal text-(--f2)">
							{seed.graphScore}
						</b>
					</span>
					<Typography.Span
						data-df="action"
						className="rounded-[3px] border border-(--line) px-2 py-0.75 font-semibold text-[10px] uppercase"
					>
						{seed.action}
					</Typography.Span>
				</div>
			</div>

			<div className="hidden border-(--line) border-b px-3 py-2 font-mono text-[9px] text-(--f4) group-data-[selected=true]:block">
				<span className="text-(--f3)">verdict: </span>
				<span data-df="cause" className="text-(--f2)" />
				<span> · </span>
				<span data-df="reason" />
			</div>

			<div className="hidden grid-cols-5 gap-1.5 p-2 font-mono text-[8.5px] group-data-[selected=true]:grid">
				<StructuralStage decision={thesisDec as any} />
				<EvidenceStage decision={thesisDec as any} />
				<DecisionMCTSStage decision={decision} />
				<ExecutionStage decision={thesisDec as any} />
			</div>

			<div className="hidden items-center gap-4 border-(--line) border-t px-3 py-1.5 font-mono text-[8.5px] text-(--f4) group-data-[selected=true]:flex">
				<span>selected root</span>
				<span
					data-df="recommended"
					className="max-w-80 truncate text-(--acc)"
				/>
				<span className="ml-auto">
					round <b data-df="round" className="font-normal text-(--f2)" />
				</span>
			</div>
		</button>
	);
};
