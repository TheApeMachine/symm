import { useSelector } from "@tanstack/react-store";
import {
	decisionStore,
	latestStrategyDecisions,
} from "#/collections/decisions";
import { fixed } from "#/components/terminal/decision-format";
import { TerminalSection } from "#/components/terminal/panels";
import type { StrategyDecision } from "#/types/thesis";
import { Badge } from "@/components/ui/badge";
import { Panel } from "@/components/ui/panel";

const alternativeEntries = (decision: StrategyDecision): string[] =>
	Object.entries(decision.alternatives ?? {})
		.sort((left, right) => right[1] - left[1])
		.slice(0, 4)
		.map(([action, utility]) => `${action} ${utility.toFixed(3)}`);

/*
StrategyDecisionRows renders backend strategy decisions beside gate verdicts so
utility, alternatives, and cause remain visible without inferring them locally.
*/
export const StrategyDecisionRows = ({ symbol }: { symbol?: string }) => {
	const decisions = useSelector(decisionStore, (state) =>
		latestStrategyDecisions(state.decisions),
	);
	const rows = symbol
		? decisions.filter((decision) => decision.symbol === symbol)
		: decisions;

	return (
		<TerminalSection
			title="Strategy intent"
			meta={`${rows.length} decision${rows.length === 1 ? "" : "s"}`}
			className="mt-4"
		>
			<div className="min-h-0 flex-1 overflow-auto p-2">
				{rows.length === 0 ? (
					<Panel
						variant="surface"
						size="bare"
						className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)"
					>
						waiting for strategy decision frames
					</Panel>
				) : null}
				<div className="flex flex-col gap-2">
					{rows.map((decision) => (
						<Panel
							key={`${decision.symbol}:${decision.action}:${decision.at}`}
							variant="surface"
							size="bare"
							className="px-3 py-2.5"
						>
							<div className="flex items-center justify-between gap-2">
								<div className="font-mono font-semibold text-[12px] text-(--f1)">
									{decision.symbol}
								</div>
								<Badge
									label={decision.action}
									variant={
										decision.action === "enter"
											? "success"
											: decision.action === "exit"
												? "warning"
												: "info"
									}
									size="xs"
								/>
							</div>
							<div className="mt-1 font-mono text-[10px] text-(--f3)">
								utility {fixed(decision.utility)} · {decision.cause}
							</div>
							<div className="mt-1 font-mono text-[9.5px] text-(--f4)">
								{decision.reason}
							</div>
							<div className="mt-2 grid grid-cols-2 gap-1.5 font-mono text-[9.5px]">
								<div className="text-(--f4)">notional</div>
								<div className="text-right text-(--f2)">
									{fixed(decision.proposedNotional)}
								</div>
								<div className="text-(--f4)">confidence</div>
								<div className="text-right text-(--f2)">
									{fixed(decision.confidence)}
								</div>
								<div className="text-(--f4)">expected return</div>
								<div className="text-right text-(--f2)">
									{fixed(decision.expectedReturn)}
								</div>
								<div className="text-(--f4)">class</div>
								<div className="text-right text-(--f2)">
									{decision.allocationClass}
								</div>
							</div>
							{alternativeEntries(decision).length > 0 ? (
								<div className="mt-2 border-(--line) border-t pt-2 font-mono text-[9px] text-(--f4)">
									alt · {alternativeEntries(decision).join(" · ")}
								</div>
							) : null}
						</Panel>
					))}
				</div>
			</div>
		</TerminalSection>
	);
};
