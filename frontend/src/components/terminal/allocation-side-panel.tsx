import { Component } from "#/components/ui/component";

/*
AllocationSidePanel states what the account has to deploy and what the
allocator is holding back.

Both readings come from the frames that own them — the desk's equity for the
capital, the allocator's own decisions for the sizing — so nothing here is a
browser estimate of either.
*/
export const AllocationSidePanel = () => (
	<div className="flex flex-col gap-3.5">
		<Component registerKey="equity">
			{({ ref }) => (
				<div
					ref={ref}
					className="rounded-[3px] border border-(--line) bg-(--surface) p-3"
				>
					<div className="flex items-center justify-between">
						<span className="font-semibold text-[12px] text-(--f1)">
							Capital deployment
						</span>
						<span
							data-paint="unrealized"
							data-paint-format=".2f"
							className="font-mono text-[11px] text-(--acc)"
						/>
					</div>
					<div className="mt-1 mb-2.75 font-mono text-[9.5px] text-(--f4)">
						cash against equity if every lot were closed now
					</div>
					{/*
						The bar is cash as a share of equity, placed on the domain the two
						readings already define. Committed basis is whatever is not cash,
						so the empty part of the track is the deployed part.
					*/}
					<div className="h-2 overflow-hidden rounded-xs bg-(--line)">
						<div
							data-set="cash"
							data-set-scale="domain-percent"
							data-set-domain="cash,equity"
							data-target="style.width"
							className="h-full bg-(--acc)"
							style={{ width: "0%" }}
						/>
					</div>
					<div className="mt-1.75 flex justify-between font-mono text-[10px] text-(--f3)">
						<span>
							cash <span data-paint="cash" data-paint-format=".2f" />
						</span>
						<span>
							equity <span data-paint="equity" data-paint-format=".2f" />
						</span>
					</div>
				</div>
			)}
		</Component>

		<Component registerKey="strategy" select="decisions">
			{({ ref, slots }) => (
				<div
					ref={ref}
					className="rounded-[3px] border border-(--line) bg-(--surface) p-3"
				>
					<div className="mb-0.5 font-semibold text-[12px] text-(--f1)">
						Position sizing
					</div>
					<div className="mb-2.75 font-mono text-[9.5px] text-(--f4)">
						notional the allocator sized, against the slots it had
					</div>
					<div className="flex flex-col gap-2.25">
						{slots.length === 0 ? (
							<div className="border-(--line) border-t pt-2.75 font-mono text-[9.5px] text-(--f4)">
								no sized candidates this round
							</div>
						) : (
							slots.map((slot) => (
								<div
									key={slot}
									data-index={slot}
									className="flex items-center justify-between gap-2 font-mono text-[10px]"
								>
									<span
										data-paint="symbol"
										className="min-w-0 flex-1 truncate text-(--f1)"
									/>
									<span
										data-paint="proposedQuantity"
										data-paint-format=".6f"
										className="shrink-0 text-(--f3)"
									/>
									<span
										data-paint="proposedNotional"
										data-paint-format=".2f"
										className="w-18 shrink-0 text-right text-(--acc)"
									/>
								</div>
							))
						)}
					</div>
					<div className="mt-2.75 flex justify-between border-(--line) border-t pt-2 font-mono text-[9.5px] text-(--f4)">
						<span>slots</span>
						<span data-index="0">
							<span data-paint="openPositions" className="text-(--f2)" /> /{" "}
							<span data-paint="slotCapacity" className="text-(--f2)" />
						</span>
					</div>
				</div>
			)}
		</Component>
	</div>
);
