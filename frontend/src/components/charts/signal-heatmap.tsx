import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";
import { Flex } from "../ui/flex";

/*
TerminalSignalHeatmap is the static canvas shell. DRAW paints via
paintTerminalSignalHeatmap.
*/
export const TerminalSignalHeatmap = () => (
	<Component
		register={(paint) => registerPainter("measurements", paint)}
		select="rows"
	>
		{({ ref, className }) => (
			<Flex.Column ref={ref} className={cn("min-h-0 overflow-auto", className)}>
				<canvas className="block size-full" />
			</Flex.Column>
		)}
	</Component>
);
