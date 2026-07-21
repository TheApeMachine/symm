import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { Canvas } from "#/components/dashboard/canvas";
import { FluidLegend } from "#/components/dashboard/fluid";
import { ColumnHeader } from "#/components/dashboard/header";
import { Pulse } from "#/components/dashboard/pulse";
import { KernelInspector } from "#/components/kernel/inspector";
import {
	TerminalFluidChart,
	TerminalPredictionChart,
} from "#/components/terminal/charts";
import { DashboardRail } from "#/components/terminal/dashboard-rail";
import { KernelList } from "#/components/terminal/kernel-list";
import {
	LiveManifoldMeta,
	LiveResonanceFooter,
	LiveResonanceTitle,
} from "#/components/terminal/live-chart-meta";
import { ThesisModal } from "#/components/terminal/thesis-modal";
import { Flex } from "@/components/ui/flex";
import { Grid } from "@/components/ui/grid";

const RouteComponent = () => {
	const kernels = useSelector(appStore, (state) => state.kernels);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const fieldStyle = useSelector(terminalStore, (state) => state.fieldStyle);
	const fieldLayer = useSelector(terminalStore, (state) => state.fieldLayer);

	return (
		<Flex.Column fullWidth className="h-full min-w-[1120px]">
			<Pulse />
			<Flex fullWidth className="relative min-h-0 flex-1">
				<KernelInspector />
				<ThesisModal />
				<Grid
					fullWidth
					responsive={false}
					className="h-full min-h-0 min-w-0 flex-1 grid-cols-[282px_minmax(360px,1fr)_332px]"
				>
					<div className="min-h-0 overflow-auto border-(--line) border-r bg-(--surface)">
						<ColumnHeader
							title="Signal kernels"
							meta={`${kernels.length} kernels`}
						/>
						<KernelList sources={kernels} />
					</div>

					<Flex.Column className="min-h-0 border-(--line) border-r bg-(--sunken)">
						<Canvas
							title="Pilot-wave field"
							meta="shared |ψ|² / ρ · focused particles · X relative log price · Z empirical order-age rank · max over Y log size"
							topRight={<LiveManifoldMeta focusSymbol={focusSymbol} />}
							legend={<FluidLegend layer={fieldLayer} />}
							className="flex-[1.45]"
						>
							<TerminalFluidChart
								contour={fieldStyle === "Contour"}
								layer={fieldLayer}
							/>
						</Canvas>
						<Canvas
							title={
								<>
									Predictive coding ·{" "}
									<LiveResonanceTitle focusSymbol={focusSymbol} />
								</>
							}
							meta="hierarchy layers · adjacent top-down links · calibrated return head"
							footer={<LiveResonanceFooter focusSymbol={focusSymbol} />}
							topRight={
								<div className="flex gap-3 text-left">
									<span className="inline-flex items-center gap-1.5">
										<span className="inline-block h-px w-3 bg-(--acc)" />
										layer ε
									</span>
									<span className="inline-flex items-center gap-1.5">
										<span className="inline-block h-px w-3 bg-info" />
										state / prediction
									</span>
									<span className="inline-flex items-center gap-1.5">
										<span className="size-2 bg-[color-mix(in_srgb,var(--up)_30%,transparent)]" />
										return ±σ
									</span>
								</div>
							}
							className="flex-1 border-(--line) border-t"
						>
							<TerminalPredictionChart />
						</Canvas>
					</Flex.Column>

					<DashboardRail />
				</Grid>
			</Flex>
		</Flex.Column>
	);
};

export const Route = createFileRoute("/")({
	component: RouteComponent,
});
