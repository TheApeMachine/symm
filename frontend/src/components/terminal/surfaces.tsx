import type { TerminalSurface } from "#/collections/terminal";
import { AllocationSidePanel } from "#/components/terminal/allocation-side";
import { DecisionTreeView } from "#/components/terminal/decision";
import { HealthPanel, RadarPanel } from "#/components/terminal/health";
import { KernelList } from "#/components/terminal/rows";
import { AllocationView, SignalDetail } from "#/components/terminal/widgets";

const INSIGHT_FEATURED_SOURCES = [
	"fluid",
	"prediction",
	"hawkes",
	"causal",
	"manifold",
	"correlation",
	"pumpdump",
	"liquidity",
] as const;

const SignalSurface = () => (
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
			<HealthPanel />
			<RadarPanel />
		</div>
	</div>
);

const AllocationSurface = () => (
	<div className="grid h-full min-w-[1080px] grid-cols-[minmax(560px,1fr)_320px]">
		<AllocationView />
		<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
			<AllocationSidePanel />
		</div>
	</div>
);

const EmptySurface = ({ title }: { title: string }) => (
	<div className="flex h-full items-center justify-center font-mono text-[11px] text-(--f4)">
		{title} is route-owned
	</div>
);

export const SurfaceBody = ({ surface }: { surface: TerminalSurface }) => {
	if (surface === "decisions") {
		return <DecisionTreeView />;
	}

	if (surface === "allocation") {
		return <AllocationSurface />;
	}

	if (surface === "xray" || surface === "cortex") {
		return <EmptySurface title={surface} />;
	}

	return <SignalSurface />;
};
