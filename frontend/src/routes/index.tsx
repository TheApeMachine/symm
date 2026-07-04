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
import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";

export const RouteComponent = () => {
	const kernels = useSelector(appStore, (state) => state.kernels);
	const fieldStyle = useSelector(terminalStore, (state) => state.fieldStyle);
	const manifold = useSelector(manifoldStore, (state) => state.frame);
	const resonance = useSelector(resonanceStore, (state) => state.frame);

	return (
		<div className="flex h-full min-w-[1120px] flex-col">
			<Pulse />
			<div className="relative grid min-h-0 flex-1 grid-cols-[282px_minmax(360px,1fr)_332px]">
				<KernelInspector />

				<div className="min-h-0 overflow-auto border-(--line) border-r bg-(--surface)">
					<ColumnHeader
						title="Signal kernels"
						meta={`${kernels.length} kernels`}
					/>
					<KernelList origins={kernels} />
				</div>

				<div className="flex min-h-0 flex-col border-(--line) border-r bg-(--sunken)">
					<Canvas
						title="Fluid density field"
						meta="navier–stokes · vol-rank × Δ · whale carriers"
						topRight={
							<div>
								{manifold === null ? (
									<div>waiting</div>
								) : (
									<>
										<div>
											{String((manifold.grid as Record<string, unknown>).x)}×
											{String((manifold.grid as Record<string, unknown>).y)}×
											{String((manifold.grid as Record<string, unknown>).z)}
										</div>
										<div>
											outliers {(manifold.carriers as unknown[]).length}
										</div>
										<div>peak {String(manifold.peak)}</div>
									</>
								)}
							</div>
						}
						legend={<FluidLegend />}
						className="flex-[1.45]"
					>
						<TerminalFluidChart contour={fieldStyle === "Contour"} />
					</Canvas>
					<Canvas
						title={`Predictive coding · ${
							resonance === null ? "waiting" : String(resonance.type)
						}`}
						meta="hierarchical error · 8-step horizon"
						footer={
							resonance === null
								? "waiting"
								: resonance.type === "resonance_universe"
									? `snapshots ${(resonance.snapshots as unknown[]).length}`
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
