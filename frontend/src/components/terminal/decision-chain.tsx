import type { MouseEvent } from "react";
import {
	CapitalStage,
	EvidenceStage,
	ForecastStage,
} from "#/components/terminal/decision-chain-stages";
import { DecisionMCTSStage } from "#/components/terminal/decision-mcts-stage";
import { setDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { Typography } from "#/components/ui/typography";

const selectDecision = (event: MouseEvent<HTMLButtonElement>): void => {
	const symbol = event.currentTarget.querySelector<HTMLElement>(
		"[data-decision-chain='symbol']",
	)?.textContent;

	if (!symbol) {
		return;
	}

	setDecisionsScopeSymbol(symbol);

	for (const row of document.querySelectorAll<HTMLElement>(
		"[data-decision-chain='row']",
	)) {
		const selected = row === event.currentTarget;
		row.dataset.selected = String(selected);
		row.setAttribute("aria-expanded", String(selected));
	}
};

/*
DecisionChain presents the real backend decision path. MCTS is intentionally
shown as a root-action result: the current search API does not expose child
visits or rewards, so the UI does not invent a tree it never received.
*/
export const DecisionChain = ({ index }: { index: number }) => (
	<button
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
					data-paint="symbol"
					data-decision-chain="symbol"
					className="font-mono font-semibold text-[13px] text-(--f1)"
				/>
				<Typography.Span
					data-paint="reason"
					className="mt-0.5 block truncate font-mono text-[9px] text-(--f4)"
				/>
			</div>
			<div className="flex shrink-0 items-center gap-2 font-mono">
				<span className="text-[9px] text-(--f4)">
					u=
					<b
						data-paint="utility"
						data-paint-format=".5f"
						className="font-normal text-(--f2)"
					/>
				</span>
				<Typography.Span
					data-paint="action"
					data-paint-class="enter:text-(--up) exit:text-(--down) hold:text-(--warn) nothing:text-(--f4)"
					className="rounded-[3px] border border-(--line) px-2 py-0.75 font-semibold text-[10px] uppercase"
				/>
			</div>
		</div>

		<div className="hidden grid-cols-4 gap-1.5 p-2 font-mono text-[8.5px] group-data-[selected=true]:grid">
			<ForecastStage />
			<EvidenceStage />
			<DecisionMCTSStage />
			<CapitalStage />
		</div>

		<div className="hidden items-center gap-4 border-(--line) border-t px-3 py-1.5 font-mono text-[8.5px] text-(--f4) group-data-[selected=true]:flex">
			<span>alternatives</span>
			<span>
				enter{" "}
				<b
					data-paint="alternatives.enter"
					data-paint-format=".5f"
					className="font-normal text-(--f2)"
				/>
			</span>
			<span>
				nothing{" "}
				<b
					data-paint="alternatives.nothing"
					data-paint-format=".5f"
					className="font-normal text-(--f2)"
				/>
			</span>
			<span className="ml-auto">
				round{" "}
				<b data-paint="arbitrationRound" className="font-normal text-(--f2)" />
			</span>
		</div>
	</button>
);
