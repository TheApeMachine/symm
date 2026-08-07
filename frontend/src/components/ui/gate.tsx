import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";
import { badgeVariants } from "./badge";
import type { Size } from "./types";

/*
Gate is a badge whose word is chosen by a painted boolean.

A painted node's text is whatever the frame wrote into it, so a flag bound
directly reads "true" or "false" — which is a value, not a label. Gate keeps
both words in the tree and lets the flag pick between them: the frame writes
into the badge's own dataset, and CSS shows the matching one. React never
re-renders, so this stays a direct-paint surface.

	<Gate bind="hawkes" on="live" off="standby" />

`bind` is the path inside the painted frame, exactly as data-paint takes it.
The element must sit inside a Component whose registered key carries that flag —
including a Component nested inside another, since a gate is rarely a row of the
same batch as the surface around it.

Every class below is a literal. A tone assembled from a prop at runtime is never
seen by the build's class scan and would emit no CSS at all, so the two states
are fixed rather than configurable.
*/

export type GateProps = Omit<ComponentProps<"span">, "children"> & {
	/* Path inside the painted frame; written to data-paint. */
	bind: string;
	/* Word shown when the flag reads true. */
	on?: string;
	/* Word shown when it does not. */
	off?: string;
	size?: Size;
};

export const Gate = ({
	ref,
	bind,
	on = "live",
	off = "standby",
	size = "xs",
	className,
	...props
}: GateProps) => (
	<span
		ref={ref}
		data-paint={bind}
		/*
			data-paint-prop diverts the value into the dataset instead of the text,
			which is what leaves the two words below intact for CSS to choose from.
		*/
		data-paint-prop="dataset.gate"
		className={cn(
			"group",
			badgeVariants({ variant: "disabled", size }),
			"data-[gate=true]:border-[color-mix(in_srgb,var(--success)_40%,transparent)]",
			"data-[gate=true]:bg-[color-mix(in_srgb,var(--success)_12%,transparent)]",
			"data-[gate=true]:text-(--success)",
			className,
		)}
		{...props}
	>
		<span className="group-data-[gate=true]:hidden">{off}</span>
		<span className="hidden group-data-[gate=true]:inline">{on}</span>
	</span>
);
