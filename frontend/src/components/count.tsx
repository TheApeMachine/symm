import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";

export const Count = () => {
	return (
		<Component registerKey="tick">
			{({ ref, className }) => (
				<span
					ref={ref}
					data-index="0"
					className={cn("font-mono text-[12px] text-(--f3)", className)}
				>
					<span data-paint="open" /> open positions
				</span>
			)}
		</Component>
	);
};
