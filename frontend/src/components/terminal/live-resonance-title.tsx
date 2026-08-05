import { Component } from "#/components/ui/component";

export const LiveResonanceTitle = () => (
	<Component registerKey="resonance" select="0">
		{({ ref, className }) => (
			<span ref={ref} className={className}>
				K
				<span data-paint="forecast.supportedHorizon" data-paint-format=".0f">
					—
				</span>
				{" · confidence "}
				<span data-paint="forecast.confidence" data-paint-format=".0%">
					—
				</span>
			</span>
		)}
	</Component>
);
