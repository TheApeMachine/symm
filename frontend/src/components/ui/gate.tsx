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
	/*
		How long the badge keeps saying "on" after the flag drops, in ms.

		Flags worth badging are usually latches that clear on the producer's own
		cycle, so painting one raw makes the badge strobe at that cycle instead of
		reporting a state. The default is sized from the observed gap between
		raises rather than picked: symm's readiness stamps arrive a median 0.6s
		apart with a p90 near 3s and a tail to 19s, so a shorter window blinks on
		the tail and this one only falls to "off" when the producer has genuinely
		stopped. Set 0 to read the flag instantaneously.
	*/
	hold?: number;
	size?: Size;
};

export const Gate = ({
	ref,
	bind,
	on = "live",
	off = "standby",
	hold = 20000,
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
		data-paint-hold={hold === 0 ? undefined : String(hold)}
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
