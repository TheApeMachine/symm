import { type MouseEvent, useEffect, useRef } from "react";
import {
	CapitalStage,
	EvidenceStage,
	ForecastStage,
} from "#/components/terminal/decision-chain-stages";
import { DecisionMCTSStage } from "#/components/terminal/decision-mcts-stage";
import {
	consumeDecisionsPendingFocus,
	setDecisionsScopeSymbol,
} from "#/components/terminal/decision-side";
import { Typography } from "#/components/ui/typography";

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

const selectDecision = (event: MouseEvent<HTMLButtonElement>): void => {
	const symbol = event.currentTarget.querySelector<HTMLElement>(
		"[data-decision-chain='symbol']",
	)?.textContent;

	if (!symbol) {
		return;
	}

	selectRow(event.currentTarget, symbol);
};

/*
DecisionChain presents the real backend decision path. MCTS is intentionally
shown as a root-action result: the current search API does not expose child
visits or rewards, so the UI does not invent a tree it never received.
*/
export const DecisionChain = ({ index }: { index: number }) => {
	const rowRef = useRef<HTMLButtonElement>(null);

	/*
		A dashboard row navigating in leaves a one-shot pending symbol behind.
		The symbol span is DOM-painted straight off the wire, so the observer
		watches its own text: the first time it settles on the pending symbol,
		this row claims the focus, expands, and scrolls itself into view.
	*/
	useEffect(() => {
		const row = rowRef.current;
		const symbolElement = row?.querySelector<HTMLElement>(
			"[data-decision-chain='symbol']",
		);

		if (!row || !symbolElement) {
			return;
		}

		const tryClaim = () => {
			const symbol = symbolElement.textContent;

			if (!symbol || !consumeDecisionsPendingFocus(symbol)) {
				return;
			}

			selectRow(row, symbol);
			row.scrollIntoView({ block: "center", behavior: "smooth" });
		};

		tryClaim();
		const observer = new MutationObserver(tryClaim);
		observer.observe(symbolElement, { childList: true, characterData: true });

		return () => observer.disconnect();
	}, []);

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
						edge=
						<b
							data-paint="expectedReturn"
							data-paint-format=".2bp"
							className="font-normal text-(--acc)"
						/>
						bp
					</span>
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

			<div className="hidden border-(--line) border-b px-3 py-2 font-mono text-[9px] text-(--f4) group-data-[selected=true]:block">
				<span className="text-(--f3)">verdict: </span>
				<span data-paint="cause" data-paint-empty="pending" className="text-(--f2)" />
				<span> · </span>
				<span data-paint="reason" data-paint-empty="planner admitted this candidate" />
			</div>

			<div className="hidden grid-cols-4 gap-1.5 p-2 font-mono text-[8.5px] group-data-[selected=true]:grid">
				<ForecastStage />
				<EvidenceStage />
				<DecisionMCTSStage />
				<CapitalStage />
			</div>

			<div className="hidden items-center gap-4 border-(--line) border-t px-3 py-1.5 font-mono text-[8.5px] text-(--f4) group-data-[selected=true]:flex">
				<span>selected root</span>
				<span
					data-paint="trace.mcts.recommendedAction"
					className="max-w-80 truncate text-(--acc)"
				/>
				<span className="ml-auto">
					round{" "}
					<b data-paint="arbitrationRound" className="font-normal text-(--f2)" />
				</span>
			</div>
		</button>
	);
};
