/*
AllocationMain is the static cross-section ladder shell. DRAW paints rows via
paintAllocationSurface without React reconciliation each tick.
*/
export const AllocationMain = () => (
	<div className="min-h-0 overflow-auto px-[18px] py-4">
		<div className="mb-3 flex items-center gap-3.5 font-mono text-[11px]">
			<span className="text-(--f3)">cross-section</span>
			<span className="text-(--f4)">
				median <span data-alloc="median" className="text-(--f2)" />
			</span>
			<span className="text-(--f4)">
				mad <span data-alloc="mad" className="text-(--f2)" />
			</span>
			<span className="text-(--f4)">
				entry <span data-alloc="threshold" className="text-(--acc)" />
			</span>
			<span className="ml-auto text-(--f4)">live ladder frames</span>
		</div>

		<div className="flex items-center gap-[9px] border-(--line) border-b pb-[7px] font-mono text-[8.5px] text-(--f4) uppercase tracking-[0.06em]">
			<span className="w-[58px] shrink-0">symbol</span>
			<span className="flex-1">thesis score {"->"} sqrt(confirmations)</span>
			<span className="w-[52px] shrink-0 text-right">edge</span>
			<span className="w-[42px] shrink-0 text-right">share</span>
			<span className="w-[74px] shrink-0 text-right">notional</span>
		</div>

		<div className="flex flex-col" data-alloc-host="rows">
			<div
				data-alloc="waiting"
				className="py-24 text-center font-mono text-[11px] text-(--f4)"
			>
				waiting for backend decision frames
			</div>
		</div>

		<div className="mt-[11px] flex items-center gap-4 font-mono text-[9px] text-(--f3)">
			{[
				["var(--acc)", "allocated"],
				["var(--info)", "in play · below edge"],
				["var(--f4)", "scanned"],
			].map(([color, label]) => (
				<span key={label} className="inline-flex items-center gap-[5px]">
					<span
						className="h-2 w-2 rounded-full"
						style={{ background: color }}
					/>
					{label}
				</span>
			))}
			<span className="inline-flex items-center gap-[5px]">
				<span className="h-px w-[10px] bg-(--acc)" /> entry line
			</span>
		</div>
	</div>
);

/*
AllocationSidePanel is the static capital deployment shell. DRAW paints sizing
rows via paintAllocationSurface without React reconciliation each tick.
*/
export const AllocationSidePanel = () => (
	<div className="flex flex-col gap-3.5">
		<div className="rounded-[3px] border border-(--line) bg-(--surface) p-3">
			<div className="flex items-center justify-between">
				<span className="font-semibold text-[12px] text-(--f1)">
					Capital deployment
				</span>
				<span
					data-alloc="deployed-pct"
					className="font-mono text-[11px] text-(--acc)"
				/>
			</div>
			<div className="mt-1 mb-[11px] font-mono text-[9.5px] text-(--f4)">
				share of deployable free cash
			</div>
			<div className="h-2 overflow-hidden rounded-[2px] bg-(--line)">
				<div
					data-alloc="deploy-fill"
					className="h-full bg-(--acc)"
					style={{ width: "0%" }}
				/>
			</div>
			<div className="mt-[7px] flex justify-between font-mono text-[10px] text-(--f3)">
				<span data-alloc="deployed-label" />
				<span data-alloc="reserved-label" />
			</div>
		</div>

		<div className="rounded-[3px] border border-(--line) bg-(--surface) p-3">
			<div className="mb-0.5 font-semibold text-[12px] text-(--f1)">
				Position sizing
			</div>
			<div
				data-alloc="quote-label"
				className="mb-[11px] font-mono text-[9.5px] text-(--f4)"
			/>
			<div className="flex flex-col gap-[9px]" data-alloc-host="sizes">
				<div
					data-alloc="sizing-empty"
					className="border-(--line) border-t pt-[11px] font-mono text-[9.5px] text-(--f4)"
				>
					no symbols above entry edge
				</div>
			</div>
		</div>
	</div>
);
