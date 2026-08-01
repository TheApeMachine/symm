import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";

/*
HawkesChart is the static canvas shell. The hawkes painter updates its canvas.
*/
export const HawkesChart = () => (
	<Component registerKey="hawkes">
		{({ ref, className }) => (
			<div ref={ref} className={cn("relative min-h-0 overflow-auto", className)}>
				<canvas className="absolute inset-0 h-full w-full" />
			</div>
		)}
	</Component>
);
