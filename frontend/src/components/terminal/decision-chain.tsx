import { type MouseEvent, useRef } from "react";
import type { Decision } from "#/types/thesis";
import {
	EvidenceStage,
	ExecutionStage,
	StructuralStage,
} from "#/components/terminal/decision-chain-stages";
import { DecisionMCTSStage } from "#/components/terminal/decision-mcts-stage";
import {
	setDecisionsScopeSymbol,
} from "#/components/terminal/decision-side";
import { Typography } from "#/components/ui/typography";
import { strategyStore, useSubscribe } from "#/providers/ws-stores";

const selectRow = (row: HTMLElement, symbol: string): void => {
	setDecisionsScopeSymbol(symbol);

	for (const other of document.querySelectorAll<HTMLElement>("[data-decision-chain='row']")) {
		const selected = other === row;
		other.dataset.selected = String(selected);
		other.setAttribute("aria-expanded", String(selected));
	}
};

export const DecisionChain = ({ index }: { index: number }) => {
	const rowRef = useRef<HTMLButtonElement>(null);

	const decision = strategyStore.state?.decisions[index] as Decision | undefined;

	useSubscribe(strategyStore, (state) => {
		const current = state?.decisions[index];

		if (current === undefined) {
			return;
		}

		const row = rowRef.current;

		if (row === null) {
			return;
		}

		const set = (q: string, value: string) => {
			const el = row.querySelector<HTMLElement>(`[data-df="${q}"]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		set("symbol", current.symbol);
		set("reason", current.reason ?? "");
		set("thesisScore", current.thesisScore.toFixed(4));
		set("thesisConfidence", `${(current.thesisConfidence * 100).toFixed(1)}%`);
		set("graphScore", current.graphScore.toFixed(5));
		set("action", current.action);
		set("cause", current.cause ?? "pending");
		set("recommended", current.trace?.mcts?.recommendedAction ?? "");
		set("round", current.arbitrationRound === undefined ? "" : String(current.arbitrationRound));
	}, [index]);

	if (decision === undefined) {
		return null;
	}

	const selectDecision = (event: MouseEvent<HTMLButtonElement>): void => {
		selectRow(event.currentTarget, decision.symbol);
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
					<Typography.Span data-df="symbol" data-decision-chain="symbol" className="font-mono font-semibold text-[13px] text-(--f1)" />
					<Typography.Span data-df="reason" className="mt-0.5 block truncate font-mono text-[9px] text-(--f4)" />
				</div>
				<div className="flex shrink-0 items-center gap-2 font-mono">
					<span className="text-[9px] text-(--f4)">thesis=<b data-df="thesisScore" className="font-normal text-(--acc)" /></span>
					<span className="text-[9px] text-(--f4)">conf=<b data-df="thesisConfidence" className="font-normal text-(--f2)" /></span>
					<span className="text-[9px] text-(--f4)">graph=<b data-df="graphScore" className="font-normal text-(--f2)" /></span>
					<Typography.Span data-df="action" className="rounded-[3px] border border-(--line) px-2 py-0.75 font-semibold text-[10px] uppercase" />
				</div>
			</div>

			<div className="hidden border-(--line) border-b px-3 py-2 font-mono text-[9px] text-(--f4) group-data-[selected=true]:block">
				<span className="text-(--f3)">verdict: </span>
				<span data-df="cause" className="text-(--f2)" />
				<span> · </span>
				<span data-df="reason" />
			</div>

			<div className="hidden grid-cols-4 gap-1.5 p-2 font-mono text-[8.5px] group-data-[selected=true]:grid">
				<StructuralStage decision={decision} />
				<EvidenceStage decision={decision} />
				<DecisionMCTSStage decision={decision} />
				<ExecutionStage decision={decision} />
			</div>

			<div className="hidden items-center gap-4 border-(--line) border-t px-3 py-1.5 font-mono text-[8.5px] text-(--f4) group-data-[selected=true]:flex">
				<span>selected root</span>
				<span data-df="recommended" className="max-w-80 truncate text-(--acc)" />
				<span className="ml-auto">round <b data-df="round" className="font-normal text-(--f2)" /></span>
			</div>
		</button>
	);
};