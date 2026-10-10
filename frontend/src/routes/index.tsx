import { createFileRoute } from "@tanstack/react-router";
import { DEFAULT_KERNELS } from "#/collections/app";
import { Decisions } from "#/components/dashboard/decisions";
import { Positions } from "#/components/dashboard/positions";
import { KernelInspector } from "#/components/kernel/inspector";
import { Pulse } from "#/components/pulse";
import { TerminalPredictionChart } from "#/components/terminal/charts";
import { LiveResonanceTitle } from "#/components/terminal/live-resonance-title";
import {
	Canvas,
	Flex,
	Grid,
	Metric,
	Section,
	Typography,
} from "#/components/ui";

const RouteComponent = () => {
	return (
		<Flex.Column fullWidth className="h-full min-w-280">
			<Pulse />
			<Flex fullWidth className="relative min-h-0 flex-1">
				<KernelInspector />
				<Grid
					fullWidth
					responsive={false}
					className="h-full min-h-0 min-w-0 flex-1 grid-cols-[282px_minmax(360px,1fr)_332px]"
				>
					<Section fit="pane" className="min-h-0 border-(--line) border-r">
						<Section.Header
							sticky
							title="Signal kernels"
							meta={`${DEFAULT_KERNELS.length} kernels`}
						/>
						{DEFAULT_KERNELS.map((kernel) => (
							<Metric key={kernel} name={kernel} />
						))}
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
								<Flex.Row gap={3} className="text-left">
									<Flex.Row align="center" gap={1}>
										<Flex className="h-px w-3 bg-(--acc)" />
										<Typography.Span>forward curve</Typography.Span>
									</Flex.Row>
									<Flex.Row align="center" gap={1}>
										<Flex className="h-px w-3 bg-info" />
										<Typography.Span>latent state</Typography.Span>
									</Flex.Row>
									<Flex.Row align="center" gap={1}>
										<Flex className="h-px w-3 bg-(--line2)" />
										<Typography.Span>zero</Typography.Span>
									</Flex.Row>
								</Flex.Row>
							}
							className="flex-1"
						>
							<TerminalPredictionChart />
						</Canvas>
					</Flex.Column>

					<Flex.Column className="min-h-0 overflow-hidden bg-(--surface)">
						<Flex.Column className="min-h-0 flex-[1.15] border-(--line) border-b">
							<Decisions />
						</Flex.Column>
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
