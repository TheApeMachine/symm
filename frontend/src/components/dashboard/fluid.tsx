export const FluidLegend = () => (
	<div className="pointer-events-none absolute bottom-2.5 left-3 flex gap-3.5 font-mono text-[9px] text-(--f3)">
		<span className="inline-flex items-center gap-1.5">
			<span className="size-2 rounded-full bg-(--acc) shadow-[0_0_7px_var(--acc)]" />
			whale carrier
		</span>
		<span className="inline-flex items-center gap-1.5">
			<span className="inline-block h-px w-3 bg-[rgba(255,228,180,0.75)]" />
			guidance flow
		</span>
		<span className="inline-flex items-center gap-1.5">
			<span className="size-2 rounded-full bg-info/70" />
			|psi|^2 veil
		</span>
	</div>
);
