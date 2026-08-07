import { Component } from "#/components/ui/component";

/*
AllocationMain is the sizing ladder.

Every column is a number the allocator already committed to: the utility it
scored the candidate at, the haircut flow pressure took off it, the notional it
sized, and the class it sized under. The ladder used to derive its own
cross-section — median, MAD, an entry line — from whatever symbols happened to
be on screen, which meant the browser could disagree with the engine about what
was allocated. It now reports the decision instead of re-deciding it.
*/
export const AllocationMain = () => (
	<Component registerKey="strategy" select="decisions">
		{({ ref, slots }) => (
			<div ref={ref} className="min-h-0 overflow-auto px-4.5 py-4">
				<div className="mb-3 flex items-center gap-3.5 font-mono text-[11px]">
					<span className="text-(--f3)">cross-section</span>
					<span className="text-(--f4)">
						candidates <span className="text-(--f2)">{slots.length}</span>
					</span>
					<span className="ml-auto text-(--f4)">
						sized by the allocator · haircut is flow pressure
					</span>
				</div>

				<div className="flex items-center gap-2.25 border-(--line) border-b pb-1.75 font-mono text-[8.5px] text-(--f4) uppercase tracking-[0.06em]">
					<span className="w-14.5 shrink-0">symbol</span>
					<span className="flex-1">utility {"->"} confidence</span>
					<span className="w-13 shrink-0 text-right">haircut</span>
					<span className="w-10.5 shrink-0 text-right">class</span>
					<span className="w-18.5 shrink-0 text-right">notional</span>
				</div>

				<div className="flex flex-col">
					{slots.length === 0 ? (
						<div className="py-24 text-center font-mono text-[11px] text-(--f4)">
							waiting for backend decision frames
						</div>
					) : (
						slots.map((slot) => (
							<div
								key={slot}
								data-index={slot}
								className="flex items-center gap-2.25 border-(--line) border-b py-1.75 font-mono text-[10px]"
							>
								<span
									data-paint="symbol"
									className="w-14.5 shrink-0 truncate font-semibold text-(--f1)"
								/>

								{/*
									The bar is the allocator's own confidence in the candidate,
									and the action beside it says what that confidence bought —
									so a symbol that was scored but not taken never reads as
									allocated.
								*/}
								<div className="flex flex-1 items-center gap-2">
									<div className="h-1.25 flex-1 overflow-hidden rounded-[3px] bg-(--line)">
										<div
											data-set="confidence"
											data-target="style.--share"
											className="h-full bg-(--acc)"
											style={{
												width: "clamp(0%, calc(var(--share, 0) * 100%), 100%)",
											}}
										/>
									</div>
									<span
										data-paint="utility"
										data-paint-format=".5f"
										className="w-16 shrink-0 text-right text-(--f2)"
									/>
									<span
										data-paint="action"
										data-paint-class="enter:text-(--up) exit:text-(--down) hold:text-(--warn) nothing:text-(--f4)"
										className="w-12 shrink-0 text-right text-[9px] uppercase"
									/>
								</div>

								<span
									data-paint="allocation_haircut"
									data-paint-format=".1%"
									className="w-13 shrink-0 text-right text-(--down)"
								/>
								<span
									data-paint="allocationClass"
									data-paint-empty="—"
									className="w-10.5 shrink-0 truncate text-right text-(--f3)"
								/>
								<span
									data-paint="proposedNotional"
									data-paint-format=".2f"
									className="w-18.5 shrink-0 text-right text-(--acc)"
								/>
							</div>
						))
					)}
				</div>

				<div className="mt-2.75 flex items-center gap-4 font-mono text-[9px] text-(--f3)">
					{[
						["var(--acc)", "notional sized"],
						["var(--down)", "flow haircut"],
						["var(--f4)", "scored only"],
					].map(([color, label]) => (
						<span key={label} className="inline-flex items-center gap-1.25">
							<span
								className="h-2 w-2 rounded-full"
								style={{ background: color }}
							/>
							{label}
						</span>
					))}
				</div>
			</div>
		)}
	</Component>
);
