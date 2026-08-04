import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";

/*
Count reads the open-lot tally off the retained positions batch itself, which is
the only place that knows how many lots are actually held.
*/
export const Count = () => {
	return (
		<Component registerKey="positions">
			{({ ref, className }) => (
				<span
					ref={ref}
					className={cn("font-mono text-[12px] text-(--f3)", className)}
				>
					<span data-paint="length" className="text-(--f1)">
						—
					</span>{" "}
					open positions
				</span>
			)}
		</Component>
	);
};
