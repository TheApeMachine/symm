import { Component } from "#/components/ui/component";

export const LiveResonanceTitle = () => (
	<Component registerKey="resonance">
		{({ ref, className }) => (
			<span ref={ref} className={className}>
				<span data-paint="samples" data-paint-format=".0f" />
				{" samples"}
			</span>
		)}
	</Component>
);
