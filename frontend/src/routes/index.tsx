import { createFileRoute } from "@tanstack/react-router";
import { DEFAULT_KERNELS } from "#/collections/app";
import { AuditTrail } from "#/components/dashboard/audit";
import { Decisions } from "#/components/dashboard/decisions";
import { FluidLegend } from "#/components/dashboard/fluid";
import { Positions } from "#/components/dashboard/positions";
import { KernelInspector } from "#/components/kernel/inspector";
import { Pulse } from "#/components/pulse";
import {
	TerminalFluidChart,
	TerminalPhaseDialChart,
	TerminalPredictionChart,
} from "#/components/terminal/charts";
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
					<Section
						fit="content"
						className="min-h-0 overflow-auto border-(--line) border-r"
					>
						<Section.Header
							sticky
							title="Signal kernels"
							meta={`${kernels.length} kernels`}
						/>
						<KernelList sources={kernels} />
					</Section>

					<Flex.Column className="min-h-0 border-(--line) border-r bg-(--sunken)">
						<Flex className="min-h-0 flex-1">
							<Canvas
								title="Fluid density field"
								meta="navier-stokes · vol-rank x delta · whale carriers"
								legend={<FluidLegend />}
								className="aspect-square h-full flex-none"
							>
								<TerminalFluidChart />
							</Canvas>
							<Canvas
								title="Phase dial"
								meta="ω-fingerprint · signed corpus response · DMT basins"
								topRight={
									<div className="flex flex-col gap-0.5">
										<span className="inline-flex items-center justify-end gap-1.5">
											<span className="inline-block size-1.5 bg-(--acc)" />
											alignment ray
										</span>
										<span className="inline-flex items-center justify-end gap-1.5">
											<span className="inline-block size-1.5 bg-info" />
											wave modes
										</span>
										<span className="inline-flex items-center justify-end gap-1.5">
											<span className="inline-block h-px w-3 bg-(--line2)" />ρ =
											0 ring
										</span>
									</div>
								}
								className="min-w-0 flex-1 border-(--line) border-l"
							>
								<TerminalPhaseDialChart />
							</Canvas>
						</Flex>
						<Canvas
							title={
								<>
									Predictive coding · <LiveResonanceTitle />
								</>
							}
							meta="settled latent state · dynamic horizon · adaptive learning pace"
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
							className="flex-1 border-(--line) border-t"
						>
							<TerminalPredictionChart />
						</Canvas>
					</Flex.Column>

					<Flex.Column className="min-h-0 overflow-hidden bg-(--surface)">
						<div className="min-h-0 flex-[1.15] border-(--line) border-b">
							<Decisions />
						</div>
						<Section className="flex-none border-(--line) border-b">
							<Section.Header title="Open positions" />
							<Positions />
						</Section>
						<div className="min-h-0 flex-1 overflow-auto">
							<AuditTrail />
						</div>
					</Flex.Column>
				</Grid>
			</Flex>
		</Flex.Column>
	);
};

export const Route = createFileRoute("/")({
	component: RouteComponent,
});
