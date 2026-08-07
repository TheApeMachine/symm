import { DecisionChain } from "#/components/terminal/decision-chain";
import { DecisionSideRail } from "#/components/terminal/decision-side-rail";
import { LiveDecisionsEntryLine } from "#/components/terminal/decisions-entry-line";
import { cn } from "#/lib/utils";
import { Panel } from "@/components/ui/panel";
import { Component } from "../ui/component";

/*
DecisionsSurface is the candidate ladder for live decision paint.

The planner publishes decisions as a flat array, so the ladder is unrolled into
one slot per decision and every field is painted straight from the wire by
name. Nothing here transforms the payload: a row shows what the planner decided
and what it decided against.
*/
export const DecisionsSurface = () => (
	<Component registerKey="strategy" select="decisions">
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
							select a chain to scope causal + cognitive evidence
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
							slots.map((slot) => <DecisionChain key={slot} index={slot} />)
						)}
					</div>
				</div>

				<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
					<DecisionSideRail />
				</div>
			</div>
		)}
	</Component>
);
