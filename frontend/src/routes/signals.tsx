import { createFileRoute } from "@tanstack/react-router";
import { HealthPanel, RadarPanel } from "#/components/terminal/health";
import { KernelList } from "#/components/terminal/rows";
import { SignalDetail } from "#/components/terminal/widgets";

const INSIGHT_FEATURED_SOURCES = [
	"fluid",
	"prediction",
	"hawkes",
	"resonance",
	"cognitive",
	"causal",
	"manifold",
	"regime",
] as const;

const RouteComponent = () => {
	return (
		<div className="grid h-full min-w-[1080px] grid-cols-[230px_minmax(420px,1fr)_320px]">
			<div className="min-h-0 overflow-auto border-(--line) border-r bg-(--surface)">
				<div className="sticky top-0 border-(--line) border-b bg-(--surface) px-3 py-2.5 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Kernels
				</div>
				<KernelList origins={[...INSIGHT_FEATURED_SOURCES]} compact />
			</div>
			<div className="min-h-0 overflow-auto bg-(--bg)">
				<SignalDetail />
			</div>
			<div className="min-h-0 space-y-3.5 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
				<HealthPanel origins={[...INSIGHT_FEATURED_SOURCES]} />
				<RadarPanel />
			</div>
		</div>
	);
};

export const Route = createFileRoute("/signals")({
	component: RouteComponent,
});
