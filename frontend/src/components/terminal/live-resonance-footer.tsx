import { Component } from "#/components/ui/component";

export const LiveResonanceFooter = () => (
	<Component registerKey="resonance">
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
