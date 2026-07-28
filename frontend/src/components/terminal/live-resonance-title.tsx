import { Component } from "#/components/ui/component";
import { registerPainter } from "#/providers/ws-stores";

export const LiveResonanceTitle = () => (
	<Component register={(paint) => registerPainter("resonance", paint)}>
		{({ ref, className }) => (
			<span ref={ref} className={className}>
				<span data-paint="samples" data-paint-format=".0f" />
				{" samples"}
			</span>
		)}
	</Component>
);
