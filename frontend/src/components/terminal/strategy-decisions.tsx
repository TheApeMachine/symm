import { cn } from "#/lib/utils";
import { Panel } from "@/components/ui/panel";
import { Section } from "@/components/ui/section";
import { Component } from "../ui/component";
import { Typography } from "../ui/typography";

/*
StrategyDecisionRows is the strategy-intent list.

Decisions arrive as a flat array, so the list is unrolled into one slot per
decision and each field is painted straight from the wire by name. Rows are
rebuilt only when the number of decisions changes; every tick after that writes
text into the nodes that are already there.
*/
export const StrategyDecisionRows = () => (
	<Component registerKey="decisions">
		{({ ref, className, slots }) => (
			<div ref={ref} className={className}>
				<Section className="mt-4">
					<Section.Header
						title="Strategy intent"
						meta={`${slots.length} decisions`}
					/>
					<div className="min-h-0 flex-1 overflow-auto p-2">
						{slots.length === 0 ? (
							<Panel
								variant="surface"
								size="bare"
								data-strategy="empty"
								className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)"
							>
								waiting for strategy decision frames
							</Panel>
						) : (
							<div className="flex flex-col gap-2">
								{slots.map((slot) => (
									<div
										key={slot}
										data-index={slot}
										className={cn(
											"rounded-[3px] border border-(--line)",
											"bg-(--surface) px-3 py-2.5",
										)}
									>
										<div className="flex items-center justify-between gap-2">
											<Typography.Span
												data-paint="symbol"
												className="font-mono font-semibold text-[12px] text-(--f1)"
											/>
											<Typography.Span
												data-paint="action"
												data-paint-class="enter:text-(--pos),exit:text-(--neg)"
												className="font-semibold text-[10px] uppercase"
											/>
										</div>

										<Typography.Span
											data-paint="utility"
											data-paint-format=".4f"
											className="mt-1 block font-mono text-[10px] text-(--f3)"
										/>

										<Typography.Span
											data-paint="reason"
											className="mt-1 block font-mono text-[9.5px] text-(--f4)"
										/>

										<div className="mt-2 grid grid-cols-2 gap-1.5 font-mono text-[9.5px]">
											<div className="text-(--f4)">notional</div>
											<Typography.Span
												data-paint="proposedNotional"
												data-paint-format=".2f"
												className="text-right text-(--f2)"
											/>

											<div className="text-(--f4)">confidence</div>
											<Typography.Span
												data-paint="confidence"
												data-paint-format=".1%"
												className="text-right text-(--f2)"
											/>

											<div className="text-(--f4)">expected return</div>
											<Typography.Span
												data-paint="expectedReturn"
												data-paint-format=".4f"
												className="text-right text-(--f2)"
											/>

											<div className="text-(--f4)">class</div>
											<Typography.Span
												data-paint="allocationClass"
												className="text-right text-(--f2)"
											/>
										</div>
									</div>
								))}
							</div>
						)}
					</div>
				</Section>
			</div>
		)}
	</Component>
);
