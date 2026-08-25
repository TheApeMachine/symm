import { Flex } from "../ui/flex";

/*
TerminalSignalHeatmap is the static canvas shell. The measurements store drives
the canvas through the stream-canvas registration; this shell only mounts it.
*/
export const TerminalSignalHeatmap = () => (
	<Flex.Column className="min-h-0 overflow-auto">
		<canvas className="block size-full" />
	</Flex.Column>
);
