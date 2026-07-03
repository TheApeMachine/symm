import { appStore } from "#/collections/app";
import { decisionStore } from "#/collections/decisions";
import { diagnosticsStore } from "#/collections/diagnostics";
import { executionsStore } from "#/collections/executions";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { positionsStore } from "#/collections/positions";
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
import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";

export const RouteComponent = () => {
	const kernels = useSelector(appStore, (state) => state.kernels);
	const fieldStyle = useSelector(terminalStore, (state) => state.fieldStyle);
	const measurements = useSelector(
		measurementsStore,
		(state) => state.measurements,
	);
	const manifold = useSelector(manifoldStore, (state) => state.frame);
	const resonance = useSelector(resonanceStore, (state) => state.frame);
	const decision = useSelector(decisionStore, (state) => state);
	const positions = useSelector(positionsStore, (state) =>
		state.frame === null
			? []
			: (state.frame.positions as Record<string, unknown>[]),
	);
	const executions = useSelector(executionsStore, (state) => state.history);
	const diagnostics = useSelector(diagnosticsStore, (state) => state.history);

	return (
		<div className="flex h-full min-w-[1120px] flex-col">
			<Pulse />
			<div className="relative grid min-h-0 flex-1 grid-cols-[282px_minmax(360px,1fr)_332px]">
				<KernelInspector />

				<div className="min-h-0 overflow-auto border-(--line) border-r bg-(--surface)">
					<ColumnHeader title="Signal kernels" />
					<div className="divide-y divide-(--line)">
						{kernels.length === 0 ? (
							<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
								waiting for measurement artifacts
							</div>
						) : null}
						{kernels.map((kernel) => {
							const history = measurements[kernel].values();
							const measurement = history.at(-1);
							const confidence =
								measurement === undefined
									? 0
									: Number(
											(measurement.output as Record<string, unknown>)
												.confidence,
										);
							const values = history.map((item) =>
								Number(
									(item.output as Record<string, unknown>).confidence,
								),
							);
							const points =
								values.length === 1
									? `0,${(1 - values[0]).toFixed(3)} 1,${(1 - values[0]).toFixed(3)}`
									: values
											.map(
												(value, index) =>
													`${(index / (values.length - 1)).toFixed(3)},${(1 - value).toFixed(3)}`,
											)
											.join(" ");

							return (
								<button
									type="button"
									key={kernel}
									onClick={() => terminalStore.actions.inspectSource(kernel)}
									className="block w-full cursor-pointer px-4 py-3 text-left hover:bg-(--raised)"
								>
									<div className="flex items-center justify-between gap-3">
										<div className="min-w-0 flex-1">
											<div className="truncate font-serif font-semibold text-[15px] text-(--f1)">
												{kernel}
											</div>
											<div className="mt-2">
												<svg
													viewBox="0 0 1 1"
													preserveAspectRatio="none"
													className="h-5 w-full overflow-visible"
												>
													<title>{`${kernel} confidence`}</title>
													<line
														x1="0"
														y1="1"
														x2="1"
														y2="1"
														stroke="var(--line)"
														strokeWidth="1.2"
														vectorEffect="non-scaling-stroke"
													/>
													{points === "" ? null : (
														<polyline
															points={points}
															fill="none"
															stroke="var(--acc)"
															strokeWidth="1.4"
															vectorEffect="non-scaling-stroke"
														/>
													)}
												</svg>
												<div className="mt-1 h-1 overflow-hidden bg-(--line)">
													<div
														className="h-full bg-(--acc)"
														style={{
															width:
																measurement === undefined
																	? "0%"
																	: `${confidence * 100}%`,
														}}
													/>
												</div>
											</div>
										</div>
										<div className="shrink-0 text-right font-mono text-[10px]">
											<div
												className={
													measurement === undefined
														? "text-(--f4)"
														: "text-(--f2)"
												}
											>
												{measurement === undefined
													? "waiting"
													: `${(confidence * 100).toFixed(0)}%`}
											</div>
											{measurement === undefined ? null : (
												<div className="text-(--f4)">
													{String(measurement.scope)}
												</div>
											)}
										</div>
									</div>
								</button>
							);
						})}
					</div>
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

				<div className="flex min-h-0 flex-col bg-(--surface)">
					<div className="flex min-h-0 flex-[1.15] flex-col border-(--line) border-b">
						<ColumnHeader
							title="Decisions"
							meta={
								<span>
									{decision.allowed.length} allow · {decision.denied.length}{" "}
									deny
								</span>
							}
						/>
						<div className="min-h-0 flex-1 overflow-auto">
							{decision.decisions.values().length === 0 ? (
								<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
									waiting for decision artifacts
								</div>
							) : null}
							{decision.decisions
								.values()
								.slice(-12)
								.reverse()
								.map((decision) => {
									return (
										<div
											key={decision.uuid as string}
											className="border-(--line) border-b px-4 py-3 font-mono text-[11px]"
										>
											<div className="flex items-center justify-between gap-3">
												<span className="text-(--f1)">
													{String(decision.scope)}
												</span>
												<span className="text-(--acc)">
													{String(decision.verdict)}
												</span>
											</div>
											<div className="mt-1 flex items-center justify-between gap-3 text-(--f4)">
												<span>{String(decision.source)}</span>
												<span>score {String(decision.score)}</span>
											</div>
										</div>
									);
								})}
						</div>
					</div>
					<div className="flex min-h-0 flex-1 flex-col border-(--line) border-b">
						<ColumnHeader
							title="Open positions"
							meta={<span>{positions.length} open</span>}
						/>
						<div className="min-h-0 flex-1 overflow-auto">
							{positions.length === 0 ? (
								<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
									waiting for position artifacts
								</div>
							) : null}
							{positions.slice(-8).map((position) => (
								<div
									key={position.symbol as string}
									className="border-(--line) border-b px-4 py-3 font-mono text-[11px]"
								>
									<div className="flex items-center justify-between gap-3">
										<span className="text-(--f1)">
											{String(position.symbol)}
										</span>
										<span className="text-(--f3)">
											{String(position.status)}
										</span>
									</div>
									<div className="mt-1 flex items-center justify-between gap-3 text-(--f4)">
										<span>qty {String(position.quantity)}</span>
										<span>pnl {String(position.unrealizedPnl)}</span>
									</div>
								</div>
							))}
						</div>
					</div>
					<div className="flex min-h-0 flex-1 flex-col">
						<ColumnHeader title="Audit trail" />
						<div className="min-h-0 flex-1 overflow-auto">
							{diagnostics.length === 0 && executions.length === 0 ? (
								<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
									waiting for diagnostics or execution artifacts
								</div>
							) : null}
							{[...diagnostics, ...executions]
								.slice(-12)
								.reverse()
								.map((entry) => (
									<div
										key={entry.uuid as string}
										className="border-(--line) border-b px-4 py-3 font-mono text-[11px]"
									>
										<div className="flex items-center justify-between gap-3">
											<span className="text-(--f1)">{String(entry.role)}</span>
											<span className="text-(--f4)">{String(entry.scope)}</span>
										</div>
										<div className="mt-1 text-(--f4)">
											{entry.role === "diagnostic"
												? String(entry.reason)
												: String(entry.order_status)}
										</div>
									</div>
								))}
						</div>
					</div>
				</div>
			</div>
		</div>
	);
};

export const Route = createFileRoute("/")({
	component: RouteComponent,
});
