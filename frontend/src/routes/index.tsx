import { createFileRoute } from "@tanstack/react-router";
import { DEFAULT_KERNELS } from "#/collections/app";
import { Decisions } from "#/components/dashboard/decisions";
import { Positions } from "#/components/dashboard/positions";
import { KernelInspector } from "#/components/kernel/inspector";
import { Pulse } from "#/components/pulse";
import { TerminalPredictionChart } from "#/components/terminal/charts";
import { KernelList } from "#/components/terminal/kernel-list";
import { LiveResonanceTitle } from "#/components/terminal/live-resonance-title";
import { ThesisModal } from "#/components/terminal/thesis-modal";
import { Canvas } from "#/components/ui/canvas";
import { Flex } from "@/components/ui/flex";
import { Grid } from "@/components/ui/grid";
import { Section } from "@/components/ui/section";

const RouteComponent = () => {
	const kernels = DEFAULT_KERNELS;

	return (
		<Flex.Column fullWidth className="h-full min-w-280">
			<Pulse />
			<Flex fullWidth className="relative min-h-0 flex-1">
				<KernelInspector />
				<ThesisModal />
				<Grid
					fullWidth
					responsive={false}
					className="h-full min-h-0 min-w-0 flex-1 grid-cols-[282px_minmax(360px,1fr)_332px]"
				>
					<Section fit="pane" className="min-h-0 border-(--line) border-r">
						<Section.Header
							sticky
							title="Signal kernels"
							meta={`${kernels.length} kernels`}
						/>
						<KernelList sources={kernels} />
					</Section>

					<Flex.Column className="min-h-0 border-(--line) border-r bg-(--sunken)">
						<Canvas
							title={
								<>
									Predictive coding · <LiveResonanceTitle />
								</>
							}
							meta="settled latent state · adaptive horizon · strict-prior direction head"
							topRight={
								<div className="flex gap-3 text-left">
									<span className="inline-flex items-center gap-1.5">
										<span className="inline-block h-px w-3 bg-(--acc)" />
										forward curve
									</span>
									<span className="inline-flex items-center gap-1.5">
										<span className="inline-block h-px w-3 bg-info" />
										latent state
									</span>
									<span className="inline-flex items-center gap-1.5">
										<span className="inline-block h-px w-3 bg-(--line2)" />
										zero
									</span>
								</div>
							}
							className="flex-1"
						>
							<TerminalPredictionChart />
						</Canvas>
					</Flex.Column>

					<Flex.Column className="min-h-0 overflow-hidden bg-(--surface)">
						<div className="min-h-0 flex-[1.15] border-(--line) border-b">
							<Decisions />
						</div>
						<Section className="min-h-0 flex-1 overflow-auto border-(--line) border-b">
							<Section.Header title="Open positions" />
							<Positions />
						</Section>
					</Flex.Column>
				</Grid>
			</Flex>
		</Flex.Column>
	);
};

export const Route = createFileRoute("/")({
	component: RouteComponent,
});
