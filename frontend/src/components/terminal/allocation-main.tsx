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
