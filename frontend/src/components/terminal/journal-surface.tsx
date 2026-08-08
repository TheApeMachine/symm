import { Component } from "#/components/ui/component";
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

Every column is one Component over the same key, so a lot appears in the rail
and in the journal on the same frame rather than on two different ones.
*/
const LIFECYCLE_TONE =
	"INITIALIZING:text-(--info) OPEN:text-(--up) CLOSED:text-(--f4) ERROR:text-(--down)";

export const JournalSurface = () => (
	<div className="grid h-full min-h-0 min-w-260 grid-cols-[minmax(280px,320px)_minmax(420px,1fr)]">
		<div className="min-h-0 overflow-auto border-(--line) border-r p-3.5">
			<Component registerKey="positions">
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
			<Component registerKey="positions">
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
												mark{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.mark"
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
												fees{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.entry_fee"
													data-paint-format=".4f"
												/>
											</span>
											<span className="text-right">
												stop{" "}
												<b
													className="font-normal text-(--f3)"
													data-paint="holding.stoploss.status"
												/>
											</span>
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
