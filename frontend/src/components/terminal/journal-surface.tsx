import { Component } from "#/components/ui/component";
import { JOURNAL } from "#/providers/ws-stores";
import { Panel } from "@/components/ui/panel";
import { Section } from "@/components/ui/section";

/*
JournalSurface is the record of what the desk actually did.

It reads the positions batch, which is the only account of a lot the engine
publishes: its lifecycle status, the prices it was opened and marked at, the
regulator that governs its exit, and what it has cost or made. The surface
previously expected three further wire keys — lifecycle, findings and journal —
that no part of the backend sends, which is why all three columns sat on
"waiting for frames" for the whole run. What it cannot show, it no longer
claims to be waiting for.

The lifecycle rail reads current positions. The journal reads the bounded,
run-local terminal-position history derived from those same backend frames.
*/
const LIFECYCLE_TONE =
	"initializing:text-(--info) pending:text-(--warning) new:text-(--info) open:text-(--up) partial:text-(--warning) partial_filled:text-(--warning) filled:text-(--up) error:text-(--down)";

const EXIT_TONE =
	"hard_floor:text-(--down) protected_floor:text-(--up) trailing_floor:text-(--up) profit_stagnation:text-(--acc) pump_momentum_exhausted:text-(--acc) horizon_expired:text-(--warning) continuation_ev_negative:text-(--warning) execution_regime_invalidated:text-(--warning)";

/*
The visit domain spans every rendered branch, so each bar states its share of
the exploration the search actually spent, relative to the least- and
most-visited root.
*/
const BRANCH_DOMAIN = Array.from(
	{ length: 6 },
	(_, index) => `decision.trace.mcts.branches.${index}.visits`,
).join(",");

/*
BranchRow paints one root branch of the causal search the decision actually
ran: the action tried, its share of all simulations, and the mean reward
those simulations returned. Branches the search never spawned stay blank —
every bound element is a leaf, so an absent path paints nothing instead of
wiping its row.
*/
const BranchRow = ({ index }: { index: number }) => (
	<div className="flex items-center gap-2">
		<span
			data-paint={`decision.trace.mcts.branches.${index}.action`}
			className="w-40 shrink-0 truncate text-[9px] text-(--f3)"
		/>
		<div className="h-1.5 min-w-0 flex-1 rounded-xs bg-(--sunken)">
			<div
				data-set={`decision.trace.mcts.branches.${index}.visits`}
				data-set-scale="domain-percent"
				data-set-domain={BRANCH_DOMAIN}
				data-target="style.width"
				className="h-full rounded-xs bg-(--acc)"
			/>
		</div>
		<b
			data-paint={`decision.trace.mcts.branches.${index}.meanReward`}
			data-paint-format=".4f"
			className="w-12 shrink-0 text-right font-normal text-(--f2)"
		/>
	</div>
);

export const JournalSurface = () => (
	<div className="grid h-full min-h-0 min-w-260 grid-cols-[minmax(280px,320px)_minmax(420px,1fr)]">
		<div className="min-h-0 overflow-auto border-(--line) border-r p-3.5">
			<Component registerKey="positions" repeat>
				{({ ref, slots }) => (
					<div ref={ref}>
						<Section>
							<Section.Header
								title="Lifecycle rail"
								meta={`${slots.length} lots`}
							/>
							<div className="flex flex-col gap-2 p-2">
								{slots.length === 0 ? (
									<Panel
										variant="surface"
										size="bare"
										className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
									>
										no lots held
									</Panel>
								) : (
									slots.map((slot) => (
										<Panel
											key={slot}
											variant="surface"
											size="bare"
											data-index={slot}
											className="flex items-center justify-between gap-2 px-2.5 py-2 font-mono text-[11px]"
										>
											<span
												data-paint="holding.symbol"
												className="truncate text-(--f1)"
											/>
											<span
												data-paint="status"
												data-paint-class={LIFECYCLE_TONE}
												className="shrink-0 text-[9px] uppercase tracking-wide"
											/>
										</Panel>
									))
								)}
							</div>
						</Section>
					</div>
				)}
			</Component>
		</div>

		<div className="min-h-0 overflow-auto px-4 py-4.5">
			<Component registerKey={JOURNAL} repeat>
				{({ ref, slots }) => (
					<div ref={ref}>
						<Section.Header
							size="bare"
							rule={false}
							title="Journal"
							meta={`${slots.length} entries`}
						/>
						<div className="mt-2 flex flex-col gap-2">
							{slots.length === 0 ? (
								<Panel
									variant="surface"
									size="bare"
									className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
								>
									nothing traded yet this run
								</Panel>
							) : (
								slots.map((slot) => (
									<Panel
										key={slot}
										variant="surface"
										size="bare"
										data-index={slot}
										className="px-3 py-2.5 font-mono text-[11px]"
									>
										<div
											data-set="holding.pnl"
											data-set-scale="sign-color"
											data-target="style.--pnl"
											className="flex items-center justify-between gap-2"
										>
											<span
												data-paint="holding.symbol"
												className="truncate font-semibold text-(--f1)"
											/>
											<span
												data-paint="holding.pnl"
												data-paint-format=".4f"
												data-paint-suffix=" USD"
												className="shrink-0 font-semibold text-(--pnl)"
											/>
										</div>

										<div className="mt-1 grid grid-cols-2 gap-x-3 gap-y-0.5 text-[9.5px] text-(--f4)">
											<span>
												opened{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.entry_at"
													data-paint-format="time"
												/>
											</span>
											<span className="text-right">
												closed{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.exit_at"
													data-paint-format="time"
												/>
											</span>
											<span>
												entry{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.entry_price"
													data-paint-format=".6f"
												/>
											</span>
											<span className="text-right">
												exit{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.exit_price"
													data-paint-format=".6f"
												/>
											</span>
											<span>
												qty{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.qty"
													data-paint-format=".6f"
												/>
											</span>
											<span className="text-right">
												return{" "}
												<b
													className="font-normal text-(--pnl)"
													data-paint="holding.return_pct"
													data-paint-format=".2f"
												/>
												%
											</span>
											<span>
												entry fee{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.entry_fee"
													data-paint-format=".4f"
												/>
											</span>
											<span className="text-right">
												exit fee{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.exit_fee"
													data-paint-format=".4f"
												/>
											</span>
										</div>

										<div className="mt-1.5 flex items-center justify-between gap-2 border-t border-(--line) pt-1.5 text-[9.5px]">
											<span className="text-(--f4)">
												exit{" "}
												<b
													data-paint="holding.stoploss.trigger_reason"
													data-paint-class={EXIT_TONE}
													className="font-semibold"
												/>
											</span>
											<span className="text-(--f4)">
												cause{" "}
												<b
													data-paint="decision.cause"
													className="font-normal text-(--f3)"
												/>
											</span>
										</div>

										<div className="mt-1 grid grid-cols-4 gap-x-3 text-[9px] text-(--f4)">
											<span>
												floor{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.stoploss.floor"
													data-paint-format=".6f"
												/>
											</span>
											<span>
												peak{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.stoploss.peak"
													data-paint-format=".6f"
												/>
											</span>
											<span>
												profit{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.stoploss.profit_line"
													data-paint-format=".6f"
												/>
											</span>
											<span
												data-set="holding.stoploss.locked"
												data-set-scale="bool-color"
												data-target="style.color"
											>
												<b
													data-paint="holding.stoploss.locked"
													className="font-normal"
												/>
											</span>
										</div>

										<div className="mt-1.5 border-t border-(--line) pt-1.5">
											<div className="flex items-center justify-between text-[9px] text-(--f4)">
												<span>
													thesis{" "}
													<b
														className="font-normal text-(--f2)"
														data-paint="decision.thesisScore"
														data-paint-format=".4f"
													/>
												</span>
												<span>
													graph{" "}
													<b
														className="font-normal text-(--f2)"
														data-paint="decision.graphScore"
														data-paint-format=".4f"
													/>{" "}
													/ gate{" "}
													<b
														className="font-normal text-(--f3)"
														data-paint="decision.admissionGraphThreshold"
														data-paint-format=".4f"
													/>
												</span>
												<span>
													<b
														className="font-normal text-(--f3)"
														data-paint="decision.trace.mcts.iterations"
													/>{" "}
													sims →{" "}
													<b
														data-paint="decision.trace.mcts.recommendedAction"
														className="font-semibold text-(--acc)"
													/>
												</span>
											</div>
											<div className="mt-1 flex flex-col gap-0.5">
												{[0, 1, 2, 3, 4, 5].map((index) => (
													<BranchRow key={index} index={index} />
												))}
											</div>
										</div>
									</Panel>
								))
							)}
						</div>
					</div>
				)}
			</Component>
		</div>
	</div>
);
