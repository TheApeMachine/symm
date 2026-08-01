import { cn } from "#/lib/utils";
import { Component } from "#/components/ui/component";
import { Flex } from "../ui/flex";

/*
TerminalSignalHeatmap is the static canvas shell. The measurements painter
updates its canvas directly.
*/
export const TerminalSignalHeatmap = () => (
	<Component registerKey="measurements">
		{({ ref, className }) => (
			<Flex.Column ref={ref} className={cn("min-h-0 overflow-auto", className)}>
				<canvas className="block size-full" />
			</Flex.Column>
		)}
	</Component>
);
