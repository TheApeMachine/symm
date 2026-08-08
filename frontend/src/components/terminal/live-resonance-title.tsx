import { Component } from "#/components/ui/component";
import { RESONANCE_FOCUS } from "#/providers/ws-stores";
/*
The resonance batch carries every settled carrier, not just the focused one, so
this reads the focused-carrier stream rather than a position in that batch.
*/

export const LiveResonanceTitle = () => (
	<Component registerKey={RESONANCE_FOCUS}>
		{({ ref, className }) => (
			<span ref={ref} className={className}>
				K
				<span data-paint="forecast.supportedHorizon" data-paint-format=".0f">
					—
				</span>
				{" · direction p "}
				<span data-paint="forecast.confidence" data-paint-format=".1%">
					—
				</span>
			</span>
		)}
	</Component>
);
