import { createFileRoute } from "@tanstack/react-router";
import { ColumnHeader } from "#/components/dashboard/header";
import { KernelInspector } from "#/components/kernel/inspector";
import { Pulse } from "#/components/pulse";
import { ThesisModal } from "#/components/terminal/thesis-modal";
import { Canvas } from "#/components/ui/canvas";
import { Component } from "#/components/ui/component";
import { Grid } from "#/components/ui/grid";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";
import { Flex } from "@/components/ui/flex";

const RouteComponent = () => {
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
					<div className="min-h-0 overflow-auto border-(--line) border-r bg-(--surface)">
						<ColumnHeader title="Signal kernels" meta={"10 kernels"} />
						<List>
							<List.Item>
								<Typography.Span>Kernel 1</Typography.Span>
							</List.Item>
						</List>
					</div>

					<Flex.Column className="min-h-0 border-(--line) border-r bg-(--sunken)">
						<Canvas
							title="Pilot-wave field"
							meta="shared |ψ|² / ρ · focused particles · X relative log price · Z empirical order-age rank · max over Y log size"
							topRight={<LiveManifoldMeta />}
							legend={<FluidLegend />}
							className="flex-[1.45]"
						>
							<TerminalFluidChart />
						</Canvas>
						<Canvas
							title={
								<>
									Predictive coding · <LiveResonanceTitle />
								</>
							}
							meta="hierarchy layers · adjacent top-down links · calibrated return head"
							footer={<LiveResonanceFooter />}
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

					<Flex.Column className="min-h-0 bg-(--surface)">
						<Flex.Column className="min-h-0 flex-[1.15] border-(--line) border-b">
							<Component
								register={(paint) => registerPainter("positions", paint)}
							>
								{({ ref, className }) => (
									<ColumnHeader
										ref={ref}
										title="Decisions"
										className={className}
										meta={
											<span>
												<span data-paint="holdings" data-transform="count" />{" "}
												active ·{" "}
												<span data-paint="holdings" data-transform="count" />{" "}
												passive
											</span>
										}
									/>
								)}
							</Component>
							{[
								{ scope: "decisions", header: "Decisions" },
								{ scope: "open_positions", header: "Open Positions" },
								{ scope: "audit_trail", header: "Audit Trail" },
							].map((item) => (
								<Component
									key={`${item.scope}`}
									register={(paint) => select(item.scope, paint)}
								>
									{({ ref, className }) => (
										<Flex.Column
											ref={ref}
											className={cn(
												"min-h-0 flex-1 border-(--line) border-b",
												className,
											)}
										>
											<ColumnHeader
												title={item.header}
												meta={
													<>
														<span data-paint="" />{" "}
														<Typography.Span>open</Typography.Span>
													</>
												}
											/>
											<div className="min-h-0 flex-1 overflow-auto p-1.5">
												<div
													data-paint=""
													className="px-4 py-5 font-mono text-[11px] text-(--f4)"
												>
													waiting for holdings frames
												</div>
											</div>
										</Flex.Column>
									)}
								</Component>
							))}
						</Flex.Column>
					</Flex.Column>
				</Grid>
			</Flex>
		</Flex.Column>
	);
};

export const Route = createFileRoute("/signals")({
	component: RouteComponent,
});
