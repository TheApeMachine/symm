import { DecisionSideRail } from "#/components/terminal/decision-side-rail";
import { LiveDecisionsEntryLine } from "#/components/terminal/decisions-entry-line";
import { StrategyDecisionRows } from "#/components/terminal/strategy-decisions";
import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";
import { Panel } from "@/components/ui/panel";
import { Component } from "../ui/component";
import { Typography } from "../ui/typography";

/*
DecisionsSurface is the static candidate-ladder shell. DRAW paints stats via
paintDecisions* exports without React reconciliation each tick.
*/
export const DecisionsSurface = () => (
	<Component register={(paint) => registerPainter("decisions", paint)}>
		{({ ref, className }) => (
			<div
				ref={ref}
				className={cn(
					"grid h-full min-h-0 min-w-260 grid-cols-[minmax(640px,1fr)_332px]",
					className,
				)}
			>
				<div className="min-h-0 overflow-auto px-5 py-4.5">
					<div className="mb-4.5 grid grid-cols-4 gap-2.5">
						<div
							className="rounded border border-(--line) bg-(--surface) px-3 py-2.5"
						>
							<div className="text-[9.5px] text-(--f4) uppercase tracking-widest">
								<Typography.Span data-paint="title" />

							</div>
							<div
								className={cn(
									"mt-0.5 font-mono font-semibold text-[26px] leading-[1.1]",
								)}
							>
								0
							</div>
							<Typography.Span data-paint="subtitle" />
						</div>
					</div>

					<LiveDecisionsEntryLine />

					<div className="mb-2 flex items-baseline justify-between">
						<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
							Candidate evaluation
						</span>
						<span className="font-mono text-[9.5px] text-(--f4)">
							click a row to inspect attribution + counterfactuals
						</span>
					</div>

					<div
						className="flex flex-col gap-1.75"
						data-decision-host="candidates"
					>
						<Panel
							variant="surface"
							size="bare"
							data-decision="waiting"
							className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
						>
							waiting for backend decision frames
						</Panel>
					</div>
				</div>

				<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
					<DecisionSideRail />
					<StrategyDecisionRows />
				</div>
			</div>
		)}
	</Component>
);
