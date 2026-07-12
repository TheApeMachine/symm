import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { manifoldStore } from "#/collections/manifold";
import { resonanceStore } from "#/collections/resonance";
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

const RouteComponent = () => {
	const app = useSelector(appStore, (state) => state);
	const terminal = useSelector(terminalStore, (state) => state);
	const focusSymbol = app.focusSymbol;
	const manifold = useSelector(
		manifoldStore,
		(state) => state.manifold[focusSymbol]?.values().at(-1) ?? null,
	);
	const resonance = useSelector(
		resonanceStore,
		(state) => state.resonance[focusSymbol]?.values().at(-1) ?? null,
	);

	return (
		<div className="flex h-full min-w-[1120px] flex-col">
			<Pulse />
			<div className="relative grid min-h-0 flex-1 grid-cols-[282px_minmax(360px,1fr)_332px]">
				<KernelInspector />

				<div className="min-h-0 overflow-auto border-(--line) border-r bg-(--surface)">
					<ColumnHeader
						title="Signal kernels"
						meta={`${app.kernels.length} kernels`}
					/>
					<KernelList sources={app.kernels} />
				</div>

				<div className="flex min-h-0 flex-col border-(--line) border-r bg-(--sunken)">
					<Canvas
						title="Fluid density field"
						meta="kinetic L3 · price × log-size × lifetime-CDF"
						topRight={
							<div>
								{manifold === null ? (
									<div>waiting</div>
								) : (
									<>
										<div>epoch {String(manifold.epoch)}</div>
										<div>mass {String(manifold.visibleMass)}</div>
										<div>modes {String(manifold.oscillatorCount)}</div>
									</>
								)}
							</div>
						}
						legend={<FluidLegend />}
						className="flex-[1.45]"
					>
						<TerminalFluidChart contour={terminal.fieldStyle === "Contour"} />
					</Canvas>
					<Canvas
						title={`Predictive coding · ${
							resonance === null
								? "waiting"
								: `${String(resonance.samples)} samples`
						}`}
						meta="online hierarchy · sensory reconstruction"
						footer={
							resonance === null
								? "waiting"
								: `symbol ${String(resonance.symbol)}`
						}
						topRight={
							<div className="flex gap-3 text-left">
								<span className="inline-flex items-center gap-1.5">
									<span className="inline-block h-px w-3 bg-(--f1)" />
									actual
								</span>
								<span className="inline-flex items-center gap-1.5">
									<span className="inline-block h-px w-3 bg-info" />
									prediction
								</span>
								<span className="inline-flex items-center gap-1.5">
									<span className="size-2 bg-[color-mix(in_srgb,var(--acc)_30%,transparent)]" />
									error
								</span>
							</div>
						}
						className="flex-1 border-(--line) border-t"
					>
						<TerminalPredictionChart />
					</Canvas>
				</div>

				<DashboardRail />
			</div>
		</div>
	);
};

export const Route = createFileRoute("/")({
	component: RouteComponent,
});
