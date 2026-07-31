import { registerPainter } from "#/providers/ws-stores";
import {Component} from "#/components/ui/component";
import { cn } from "#/lib/utils";

/*
HawkesChart is the static canvas shell. KernelList paints it via paintHawkes.
*/
export const HawkesChart = () => (
	<Component register={(paint) => registerPainter("hawkes", paint)}>
		{({ ref, className }) => (
			<div ref={ref} className={cn("min-h-0 overflow-auto", className)}>
				<canvas className="absolute inset-0 h-full w-full" />
			</div>
		)}
	</Component>
);
