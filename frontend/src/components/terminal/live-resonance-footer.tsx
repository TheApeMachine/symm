import { Component } from "#/components/ui/component";
import { RESONANCE_FOCUS } from "#/providers/ws-stores";
/*
The resonance batch carries every settled carrier, not just the focused one, so
this reads the focused-carrier stream rather than a position in that batch.
*/

export const LiveResonanceFooter = () => (
	<Component registerKey={RESONANCE_FOCUS}>
		{({ ref, className }) => (
			<span ref={ref} className={className}>
				α{" "}
				<span data-paint="alpha" data-paint-format=".4f">
					—
				</span>
				{" · surprise "}
				<span data-paint="surprise" data-paint-format=".2f">
					—
				</span>
			</span>
		)}
	</Component>
);
