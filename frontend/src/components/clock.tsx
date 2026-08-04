import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { cn } from "#/lib/utils";

/*
Clock shows the engine's own wall clock, stamped on each strategy arbitration.

It is deliberately not the browser's clock: this terminal drives replayed runs
as readily as live ones, and the only time that explains what the surfaces are
showing is the time the engine thinks it is.
*/
export const Clock = () => (
	<Component registerKey="strategy">
		{({ ref, className }) => (
			<Flex.Column ref={ref} className={cn(className)}>
				<Flex>
					<span data-paint="at" data-paint-format="time" data-paint-suffix=" UTC">
						—
					</span>
				</Flex>
				<Flex className="text-(--f4)">
					<span data-paint="at" data-paint-format="date">
						engine clock
					</span>
				</Flex>
			</Flex.Column>
		)}
	</Component>
);
