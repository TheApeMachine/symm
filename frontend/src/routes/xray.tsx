import { createFileRoute } from "@tanstack/react-router";
import {
	XrayFactsPanel,
	XrayHawkesPanel,
	XrayHierarchyPanel,
	XrayLatentPanel,
	XrayManifoldPanel,
} from "#/components/terminal/xray-panels";

/*
Xray composes store-painted panels. Predictive coding reads resonance.layers
only; manifold / ρ stay on their own side panel and never feed the hierarchy.
*/
const RouteComponent = () => (
	<div className="flex h-full min-w-[1100px] flex-col">
		<div className="grid min-h-0 flex-1 grid-cols-[minmax(520px,1fr)_352px]">
			<div className="flex min-h-0 flex-col overflow-auto border-(--line) border-r">
				<XrayHierarchyPanel />
				<XrayHawkesPanel />
			</div>

			<div className="flex min-h-0 flex-col overflow-auto bg-(--surface)">
				<div className="shrink-0 px-3.5 pt-3 pb-1.5">
					<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Latent manifold
					</div>
					<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
						universe embedding · clustered by regime · focus pulses
					</div>
				</div>
				<div className="relative mx-2 h-[300px] shrink-0">
					<XrayLatentPanel />
					<div className="pointer-events-none absolute bottom-1.5 left-2.5 font-mono text-[8.5px] text-(--f4)">
						latent-1 →
					</div>
					<div className="pointer-events-none absolute top-2.5 left-1.5 font-mono text-[8.5px] text-(--f4) [writing-mode:vertical-rl]">
						latent-2 →
					</div>
				</div>

				<XrayFactsPanel />
				<XrayManifoldPanel />
			</div>
		</div>
	</div>
);

export const Route = createFileRoute("/xray")({
	component: RouteComponent,
});
