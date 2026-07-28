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
