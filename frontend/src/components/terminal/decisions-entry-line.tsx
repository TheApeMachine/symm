import { useDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { Component } from "#/components/ui/component";
import { Panel } from "@/components/ui/panel";

/*
LiveDecisionsEntryLine states the entry gate the selected candidate is being
judged against.

The causal batch carries one row per symbol, so the panel pins its row with
data-scope/data-filter on the candidate the ladder has selected. Every figure is
painted straight off that row: entry_baseline is the gate, strength is what the
symbol currently reads against it, and confidence is how much of that reading
the model will stand behind. Nothing is recomputed here — a browser that derives
the gate is a browser that can disagree with the engine about it.
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
								causal admission line
							</span>
							<span className="text-[9px] text-(--f4)">
								classifier evidence, not a price or probability
							</span>
						</div>
						<div className="grid grid-cols-3 gap-2 text-[10px]">
							<div className="rounded-xs border border-(--line) bg-(--surface) px-2 py-1">
								<div className="text-(--f4)">entry baseline</div>
								<div
									data-paint="entry_baseline"
									data-paint-format=".6f"
									className="mt-0.5 text-[12px] font-semibold text-(--acc)"
								/>
							</div>
							<div className="rounded-xs border border-(--line) bg-(--surface) px-2 py-1">
								<div className="text-(--f4)">evidence score</div>
								<div
									data-paint="strength"
									data-paint-format=".6f"
									className="mt-0.5 text-[12px] text-(--f1)"
								/>
							</div>
							<div className="rounded-xs border border-(--line) bg-(--surface) px-2 py-1">
								<div className="text-(--f4)">evidence share</div>
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
