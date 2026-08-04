import { DecisionStage } from "#/components/terminal/decision-chain-stages";

/*
DecisionMCTSStage shows the search's real root state and result. The current
search API returns no child visit/reward table, so this deliberately stops at
the robust-child action instead of drawing a fictional tree.
*/
export const DecisionMCTSStage = () => (
	<DecisionStage title="3 · causal MCTS" meta="robust root child">
		<div className="grid grid-cols-2 gap-x-2 text-(--f4)">
			<span>
				E{" "}
				<b
					data-paint="trace.mcts.energy"
					data-paint-format=".3f"
					className="font-normal text-(--f2)"
				/>
			</span>
			<span>
				S{" "}
				<b
					data-paint="trace.mcts.surprise"
					data-paint-format=".3f"
					className="font-normal text-(--f2)"
				/>
			</span>
			<span>
				T{" "}
				<b
					data-paint="trace.mcts.treatment"
					data-paint-format=".3%"
					className="font-normal text-(--f2)"
				/>
			</span>
			<span>
				cost{" "}
				<b
					data-paint="trace.mcts.roundTripCost"
					data-paint-format=".3%"
					className="font-normal text-(--f2)"
				/>
			</span>
		</div>
		<div className="flex justify-between gap-2">
			<span className="text-(--f4)">history</span>
			<span>
				<b data-paint="trace.mcts.causalRows" className="font-normal" /> /{" "}
				<b data-paint="trace.mcts.minimumCausalRows" className="font-normal" />{" "}
				rows
			</span>
		</div>
		<div className="flex justify-between gap-2 text-(--f4)">
			<span>
				<b
					data-paint="trace.mcts.iterations"
					className="font-normal text-(--f2)"
				/>{" "}
				iterations
			</span>
			<span>
				<b
					data-paint="trace.mcts.horizonSteps"
					className="font-normal text-(--f2)"
				/>{" "}
				steps
			</span>
		</div>
		<div className="flex items-center justify-between gap-2">
			<span className="text-(--f4)">
				search{" "}
				<b
					data-paint="trace.mcts.attempted"
					data-paint-class="true:text-(--up) false:text-(--warn)"
					className="font-normal"
				/>
			</span>
			<span
				data-paint="trace.mcts.recommendedAction"
				data-paint-empty="not run"
				className="font-semibold text-(--acc) uppercase"
			>
				not run
			</span>
		</div>
		<span data-paint="trace.mcts.error" className="truncate text-(--down)" />
	</DecisionStage>
);
