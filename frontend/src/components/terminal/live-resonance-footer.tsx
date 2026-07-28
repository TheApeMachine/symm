import { Component } from "#/components/ui/component";
import { registerPainter } from "#/providers/ws-stores";

export const LiveResonanceFooter = () => (
	<Component register={(paint) => registerPainter("resonance", paint)}>
		{({ ref, className }) => (
			<span ref={ref} className={className}>
				symbol <span data-paint="symbol" />
			</span>
		)}
	</Component>
);
