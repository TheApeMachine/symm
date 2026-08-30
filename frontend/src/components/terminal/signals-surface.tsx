import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { SignalDetail } from "#/components/kernel/detail";
import { CrossSectionPanel } from "#/components/terminal/cross-section-panel";
import { HealthPanel } from "#/components/terminal/health";
import { KernelList } from "#/components/terminal/kernel-list";
import { orderedKernelSources } from "#/components/terminal/kernel-meta";
import { RadarPanel } from "#/components/terminal/regime-radar";

/*
signalsSurfaceSources merges configured kernels with discovered backend sources
into a stable ordered list for KernelList shells.
*/
export const signalsSurfaceSources = (
	kernels: string[],
	backendSources: string[],
): string[] =>
	orderedKernelSources([...new Set([...kernels, ...backendSources])]);

/*
SignalsSurface is a static Signal Insight shell. KernelList mounts from app
kernels; DRAW paints live readouts. Health and radar derive sources from each
measurements batch — no React state for DRAW discovery.
*/
export const SignalsSurface = () => {
	const kernels = useSelector(appStore, (state) => state.kernels);

	return (
		<div className="grid h-full min-w-270 grid-cols-[230px_minmax(420px,1fr)_320px]">
			{/*
				Unlike the dashboard's fixed kernel set, this list merges in sources
				discovered from the backend, so its length is not known up front and
				the column stays a scrolling one — the rows share the height when
				they fit and scroll when they genuinely cannot.
			*/}
			<div className="flex min-h-0 flex-col border-(--line) border-r bg-(--surface)">
				<div className="shrink-0 border-(--line) border-b bg-(--surface) px-3 py-2.5 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Kernels
				</div>
				<KernelList sources={kernels} compact />
			</div>
			<div className="min-h-0 overflow-auto bg-(--bg)">
				<SignalDetail />
			</div>
			<div className="min-h-0 space-y-3.5 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
				<HealthPanel />
				<CrossSectionPanel />
				<RadarPanel />
			</div>
		</div>
	);
};
