import { DecisionSideRail } from "#/components/terminal/decision-side-rail";
import { LiveDecisionsEntryLine } from "#/components/terminal/decisions-entry-line";
import { StrategyDecisionRows } from "#/components/terminal/strategy-decisions";
import { cn } from "#/lib/utils";
import { Panel } from "@/components/ui/panel";
import { Component } from "../ui/component";
import { Typography } from "../ui/typography";

/*
DecisionsSurface is the candidate ladder for live decision paint.

The planner publishes decisions as a flat array, so the ladder is unrolled into
one slot per decision and every field is painted straight from the wire by
name. Nothing here transforms the payload: a row shows what the planner decided
and what it decided against.
*/
export const DecisionsSurface = () => (
	<Component registerKey="decisions">
		{({ ref, className, slots }) => (
			<div
				ref={ref}
				className={cn(
					"grid h-full min-h-0 min-w-260 grid-cols-[minmax(640px,1fr)_332px]",
					className,
				)}
			>
				<div className="min-h-0 overflow-auto px-5 py-4.5">
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
						{slots.length === 0 ? (
							<Panel
								variant="surface"
								size="bare"
								data-decision="waiting"
								className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
							>
								waiting for backend decision frames
							</Panel>
						) : (
							slots.map((slot) => (
								<Panel
									key={slot}
									variant="surface"
									size="bare"
									data-index={slot}
									className="overflow-hidden rounded border border-(--line)"
								>
									<div className="grid grid-cols-[78px_1fr_132px_92px] items-center gap-3 px-3 py-2.25">
										<div>
											<Typography.Span
												data-paint="symbol"
												className="font-mono font-semibold text-[13px] text-(--f1)"
											/>
											<Typography.Span
												data-paint="allocationClass"
												className="block font-mono text-[9px] text-(--f4)"
											/>
										</div>

										<div className="flex flex-col gap-1">
											<div className="flex items-center gap-1.75">
												<span className="w-16 font-mono text-[9px] text-(--f4)">
													return
												</span>
												<Typography.Span
													data-paint="expectedReturn"
													data-paint-format=".4f"
													className="flex-1 text-right font-mono text-[9px] text-(--f3)"
												/>
											</div>

											<div className="flex items-center gap-1.75">
												<span className="w-16 font-mono text-[9px] text-(--f4)">
													fees
												</span>
												<Typography.Span
													data-paint="expectedFees"
													data-paint-format=".4f"
													className="flex-1 text-right font-mono text-[9px] text-(--f3)"
												/>
											</div>

											<div className="flex items-center gap-1.75">
												<span className="w-16 font-mono text-[9px] text-(--f4)">
													spread
												</span>
												<Typography.Span
													data-paint="expectedSpread"
													data-paint-format=".4f"
													className="flex-1 text-right font-mono text-[9px] text-(--f3)"
												/>
											</div>
										</div>

										<div>
											<div className="mb-0.75 flex items-center justify-between font-mono text-[9.5px] text-(--f4)">
												<span>utility</span>
												<Typography.Span
													data-paint="utility"
													data-paint-format=".3f"
													className="text-(--f1)"
												/>
											</div>

											<div className="flex items-center justify-between font-mono text-[9px] text-(--f4)">
												<span>confidence</span>
												<Typography.Span
													data-paint="confidence"
													data-paint-format=".1%"
												/>
											</div>

											<div className="flex items-center justify-between font-mono text-[9px] text-(--f4)">
												<span>uncertainty</span>
												<Typography.Span
													data-paint="uncertainty"
													data-paint-format=".3f"
												/>
											</div>
										</div>

										<div className="text-right">
											<Typography.Span
												data-paint="action"
												data-paint-class="enter:bg-(--pos)/15,text-(--pos)"
												className="inline-block rounded-xs px-2.25 py-0.75 font-semibold text-[10px] uppercase"
											/>
											<Typography.Span
												data-paint="cause"
												className="mt-0.75 block font-mono text-[9px] text-(--f4)"
											/>
										</div>
									</div>
								</Panel>
							))
						)}
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
