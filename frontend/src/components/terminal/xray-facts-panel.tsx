import { cn } from "#/lib/utils";
import { Component } from "#/components/ui/component";

/*
XrayFactsPanel is the static facts shell. DRAW paints via the paintXrayFacts*
exports.
*/
export const XrayFactsPanel = () => (
	<Component registerKey="">
		{({ ref, className }) => (
			<div ref={ref} className={cn("flex h-full flex-col", className)}>
	<div className="mt-2 flex flex-col gap-2.5 border-(--line) border-t px-3.5 py-3 font-mono text-[12px]">
		<div className="flex justify-between gap-3">
			<span className="text-(--f3)">regime class</span>
			<span className="text-right text-(--acc)" />
		</div>
		<div className="flex justify-between gap-3">
			<span className="text-(--f3)">coherence</span>
			<span className="text-right" />
		</div>
		<div className="flex justify-between gap-3">
			<span className="text-(--f3)">free energy</span>
			<span className="text-right text-(--f1)" />
		</div>
		<div className="flex justify-between gap-3">
			<span className="text-(--f3)">surprise</span>
			<span className="text-right text-(--f1)" />
		</div>
		<div className="flex justify-between gap-3">
			<span className="text-(--f3)">flow events</span>
			<span className="text-right text-(--f1)" />
		</div>
		<div className="flex justify-between gap-3">
			<span className="text-(--f3)">branching η</span>
			<span className="text-right" />
		</div>
	</div>
			</div>
		)}
	</Component>
);
