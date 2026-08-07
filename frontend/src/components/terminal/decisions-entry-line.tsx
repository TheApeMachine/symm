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
					<Panel className="mb-3.5 flex items-center gap-3.5 px-3 py-2 font-mono text-[11.5px]">
						<span className="text-(--f3)">entry line</span>
						<span
							data-paint="entry_baseline"
							data-paint-format=".6f"
							className="font-semibold text-(--acc)"
						/>
						<span className="text-(--f4)">·</span>
						<span className="text-(--f3)">
							strength{" "}
							<span
								data-paint="strength"
								data-paint-format=".6f"
							/>
						</span>
						<span className="text-(--f4)">·</span>
						<span className="text-(--f3)">
							confidence{" "}
							<span
								data-paint="confidence"
								data-paint-format=".4f"
							/>
						</span>
						<span className="ml-auto text-(--f4)">
							support gate ≥ 2 · strategy utility wins
						</span>
					</Panel>
				</div>
			)}
		</Component>
	);
};
