import { Component } from "#/components/ui/component";

export const LiveResonanceFooter = () => (
	<Component registerKey="resonance">
		{({ ref, className }) => (
			<span ref={ref} className={className}>
				symbol <span data-paint="symbol" />
			</span>
		)}
	</Component>
);
