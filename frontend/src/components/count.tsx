import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";

export const Count = () => {
	return (
		<Component register={(paint) => registerPainter("tick", paint)}>
			{({ ref, className }) => (
				<span
					ref={ref}
					className={cn("font-mono text-[12px] text-(--f3)", className)}
				>
					<span data-paint="open" /> open positions
				</span>
			)}
		</Component>
	);
};
