import { Component } from "#/components/ui/component";
import { registerPainter } from "#/providers/ws-stores";

/*
XrayHawkesPanel streams focused buy-side conditional intensity directly onto its
canvas. Component owns the canvas history and paints each new measurement once.
*/
export const XrayHawkesPanel = () => (
	<Component
		select="$"
		register={(paint) => registerPainter("measurements", paint)}
	>
		{({ ref }) => (
			<div
				ref={ref}
				className="flex min-h-[210px] flex-1 flex-col border-(--line) border-t"
			>
				<div className="flex items-start justify-between gap-3 px-[18px] pt-3 pb-2">
					<div>
						<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
							Hawkes self-exciting intensity
						</div>
						<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
							λ(t) = μ + Σ α·e^(-β(t-tᵢ)) · order-flow arrivals
						</div>
					</div>
					<div className="font-mono text-[10px] text-(--f4)">
						streamed conditional intensity
					</div>
				</div>
				<div className="relative min-h-0 flex-1">
					<canvas
						data-stream-filter="source=hawkes"
						data-stream-id="at"
						data-stream-value="metrics.conditional_intensity:buy.raw"
						className="absolute inset-0 block size-full"
					/>
				</div>
			</div>
		)}
	</Component>
);
