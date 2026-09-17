import { DEFAULT_KERNELS } from "#/collections/app";
import { SignalDetail } from "#/components/kernel/detail";
import { CrossSectionPanel } from "#/components/terminal/cross-section-panel";
import { HealthPanel } from "#/components/terminal/health";
import { KernelList } from "#/components/terminal/kernel-list";
import { orderedKernelSources } from "#/components/terminal/kernel-meta";
import { RadarPanel } from "#/components/terminal/regime-radar";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Rail } from "#/components/ui/rail";

/*
signalsSurfaceSources merges configured kernels with backend sources
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
	const kernels = DEFAULT_KERNELS;

	return (
		<Grid
			cols={3}
			responsive={false}
			className="h-full min-w-270 grid-cols-[230px_minmax(420px,1fr)_320px]"
		>
			{/*
				Unlike the dashboard's fixed kernel set, this list merges in sources
				discovered from the backend, so its length is not known up front and
				the column stays a scrolling one — the rows share the height when
				they fit and scroll when they genuinely cannot.
			*/}
			<Rail position="left" surface="surface">
				<Rail.Header title="Kernels" />
				<KernelList sources={kernels} compact />
			</Rail>

			<Flex.Column fullHeight className="min-h-0 overflow-auto bg-(--bg)">
				<SignalDetail />
			</Flex.Column>

			<Rail position="right" surface="surface">
				<Rail.Body padding="m">
					<HealthPanel />
					<CrossSectionPanel />
					<RadarPanel />
				</Rail.Body>
			</Rail>
		</Grid>
	);
};
