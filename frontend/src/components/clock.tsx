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
	/*
		The strategy envelope carries no stamp of its own; the decisions it
		arbitrated do. Reading the first decision's instant is what makes this the
		engine's clock rather than the browser's.
	*/
	<Component registerKey="strategy" select="decisions">
		{({ ref, className }) => (
			<Flex.Column ref={ref} data-index="0" className={cn(className)}>
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
