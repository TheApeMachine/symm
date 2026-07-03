export const FluidLegend = () => (
	<div className="pointer-events-none absolute bottom-2.5 left-3 flex gap-3.5 font-mono text-[9px] text-(--f3)">
		<span className="inline-flex items-center gap-1.5">
			<span className="size-2 rounded-full bg-(--acc) shadow-[0_0_7px_var(--acc)]" />
			whale carrier
		</span>
		<span className="inline-flex items-center gap-1.5">
			<span className="size-2 rounded-full bg-info" />
			laminar
		</span>
		<span className="inline-flex items-center gap-1.5">
			<span className="size-2 rounded-full bg-(--down)" />
			turbulent
		</span>
	</div>
);
