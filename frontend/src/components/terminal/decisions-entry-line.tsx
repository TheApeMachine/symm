import { useDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { Component } from "#/components/ui/component";
import { Panel } from "@/components/ui/panel";

/*
LiveDecisionsEntryLine shows the selected candidate's causal evidence
classification. These are classifier diagnostics, not the planner's entry
gate: entry_baseline is the runner-up evidence share, strength is the strongest
standardized evidence channel, and confidence is the winning evidence share.
*/
export const LiveDecisionsEntryLine = () => {
	const scope = useDecisionsScopeSymbol();

	if (scope === undefined) {
		return null;
	}

	return (
		<Component registerKey="causal">
			{({ ref }) => (
				<div ref={ref} data-scope="symbol" data-filter={scope}>
					<Panel className="mb-3.5 px-3 py-2 font-mono">
						<div className="mb-1.5 flex items-center justify-between gap-3">
							<span className="text-[10px] font-semibold text-(--f3) uppercase tracking-[0.13em]">
								causal evidence classification
							</span>
							<span className="text-[9px] text-(--f4)">
								standardized channels and classifier shares
							</span>
						</div>
						<div className="grid grid-cols-3 gap-2 text-[10px]">
							<div className="rounded-xs border border-(--line) bg-(--surface) px-2 py-1">
								<div className="text-(--f4)">runner-up evidence share</div>
								<div
									data-paint="entry_baseline"
									data-paint-format=".6f"
									className="mt-0.5 text-[12px] font-semibold text-(--acc)"
								/>
							</div>
							<div className="rounded-xs border border-(--line) bg-(--surface) px-2 py-1">
								<div className="text-(--f4)">
									strongest standardized channel
								</div>
								<div
									data-paint="strength"
									data-paint-format=".6f"
									className="mt-0.5 text-[12px] text-(--f1)"
								/>
							</div>
							<div className="rounded-xs border border-(--line) bg-(--surface) px-2 py-1">
								<div className="text-(--f4)">winning evidence share</div>
								<div
									data-paint="confidence"
									data-paint-format=".1%"
									className="mt-0.5 text-[12px] text-(--info)"
								/>
							</div>
						</div>
					</Panel>
				</div>
			)}
		</Component>
	);
};
