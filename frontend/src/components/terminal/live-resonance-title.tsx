import { Component } from "#/components/ui/component";

export const LiveResonanceTitle = () => (
	<Component registerKey="resonance">
		{({ ref, className }) => (
			<span ref={ref} className={className}>
				K
				<span data-paint="activeHorizon" data-paint-format=".0f">
					—
				</span>
				{" · confidence "}
				<span data-paint="confidence" data-paint-format=".0%">
					—
				</span>
			</span>
		)}
	</Component>
);
