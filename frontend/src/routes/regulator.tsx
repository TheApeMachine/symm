import { createFileRoute } from "@tanstack/react-router";
import { useLayoutEffect, useState } from "react";
import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";
import type { JSONSerializable } from "#/components/ui/paint";
import { getLastFrame, registerPainter } from "#/providers/ws-stores";

type SubsystemStatus = {
	name: string;
	label: string;
	health: "healthy" | "adapting" | "strained" | "observing";
	direction: string;
	valueText: string;
	explanation: string;
	value: number;
};

type RegulatorFrame = {
	status?: "healthy" | "adapting" | "strained" | "observing";
	surprise?: number;
	energy?: number;
	predictedReturn?: number;
	predictionScale?: number;
	samples?: number;
	summary?: string;
	subsystems?: SubsystemStatus[];
	sparkline?: number[];
};

const DEFAULT_FRAME: RegulatorFrame = {
	status: "observing",
	surprise: 0.0,
	energy: 0.0,
	summary: "Waiting for a changed account valuation to resolve the first applied control vector.",
	subsystems: [],
	sparkline: [],
};

const HealthPulse = ({ status }: { status: "healthy" | "adapting" | "strained" | "observing" }) => {
	const colors = {
		observing: {
			bg: "bg-(--acc)",
			text: "text-(--acc)",
			border: "border-(--acc)",
			ring: "shadow-[0_0_15px_rgba(59,130,246,0.35)]",
			label: "Observing / Resolving",
		},
		healthy: {
			bg: "bg-(--up)",
			text: "text-(--up)",
			border: "border-(--up)",
			ring: "shadow-[0_0_15px_rgba(34,197,94,0.35)]",
			label: "Predictive / Optimizing",
		},
		adapting: {
			bg: "bg-(--warn)",
			text: "text-(--warn)",
			border: "border-(--warn)",
			ring: "shadow-[0_0_15px_rgba(234,179,8,0.35)]",
			label: "Identifying / Exploring",
		},
		strained: {
			bg: "bg-(--down)",
			text: "text-(--down)",
			border: "border-(--down)",
			ring: "shadow-[0_0_15px_rgba(239,68,68,0.35)]",
			label: "Adverse Return Forecast",
		},
	};

	const config = colors[status] ?? colors.observing;

	return (
		<Flex.Row align="center" gap={3} className="shrink-0">
			<div className={`relative h-4 w-4 rounded-full ${config.bg} ${config.ring} animate-pulse`} />
			<span className={`font-mono text-[13px] font-semibold tracking-wider uppercase ${config.text}`}>
				{config.label}
			</span>
		</Flex.Row>
	);
};

const SparklineSVG = ({ points }: { points: number[] }) => {
	if (points.length < 2) {
		return null;
	}

	const max = Math.max(...points, 0.1);
	const min = Math.min(...points, 0);
	const range = max - min || 1;

	const width = 300;
	const height = 40;

	const coords = points.map((val, idx) => {
		const x = (idx / (points.length - 1)) * width;
		const y = height - ((val - min) / range) * (height - 8) - 4;
		return `${x.toFixed(1)},${y.toFixed(1)}`;
	});

	const pathD = `M ${coords.join(" L ")}`;

	return (
		<svg width={width} height={height} className="overflow-visible">
			<path d={pathD} fill="none" stroke="var(--acc)" strokeWidth="2" strokeLinecap="round" />
		</svg>
	);
};

const RegulatorBridge = ({ onFrame }: { onFrame: (frame: RegulatorFrame) => void }) => {
	useLayoutEffect(() => {
		const paint = (updates: JSONSerializable) => {
			if (updates && typeof updates === "object" && !Array.isArray(updates)) {
				onFrame(updates as RegulatorFrame);
			}
		};

		const unregister = registerPainter("regulator", paint);
		const seed = getLastFrame("regulator");

		if (seed && typeof seed === "object" && !Array.isArray(seed)) {
			onFrame(seed as RegulatorFrame);
		}

		return unregister;
	}, [onFrame]);

	return null;
};

const RouteComponent = () => {
	const [frame, setFrame] = useState<RegulatorFrame>(DEFAULT_FRAME);

	const status = frame.status ?? "healthy";
	const surprise = frame.surprise ?? 0.04;
	const energy = frame.energy ?? 0.01;
	const predictedReturn = frame.predictedReturn ?? 0;
	const predictionScale = frame.predictionScale ?? 0;
	const samples = frame.samples ?? 0;
	const summary = frame.summary ?? "System operating in calm, optimal equilibrium.";
	const subsystems = frame.subsystems ?? DEFAULT_FRAME.subsystems ?? [];
	const sparkline = frame.sparkline ?? DEFAULT_FRAME.sparkline ?? [];

	return (
		<div className="flex h-full min-w-275 flex-col overflow-auto bg-(--bg) p-5 gap-5">
			<RegulatorBridge onFrame={setFrame} />

			{/* Vital Health Banner */}
			<Panel className="p-5 border border-(--line2) bg-(--surface) rounded-md flex flex-col gap-3">
				<Flex.Row justify="between" align="center" className="w-full">
					<Flex.Column gap={1}>
						<span className="font-mono text-[10px] text-(--f4) uppercase tracking-widest">
							Global Active Inference Regulator
						</span>
						<h1 className="text-xl font-bold tracking-tight text-(--f1)">
							System Vital Influence Dashboard
						</h1>
					</Flex.Column>

					<HealthPulse status={status} />
				</Flex.Row>

				<p className="font-mono text-[12px] text-(--f3) leading-relaxed">
					{summary}
				</p>

				<Flex.Row justify="between" align="center" className="pt-2 border-t border-(--line) text-[11px] font-mono text-(--f4)">
					<Flex.Row gap={4}>
						<span>Reconstruction Error: <strong className="text-(--f1)">{surprise.toFixed(4)}</strong></span>
						<span>Variational Energy: <strong className="text-(--f1)">{energy.toFixed(3)}</strong></span>
						<span>Next Equity Return: <strong className="text-(--f1)">{(predictedReturn * 100).toFixed(3)}%</strong></span>
						<span>Posterior Scale: <strong className="text-(--f1)">{predictionScale.toFixed(4)}</strong></span>
						<span>Resolved Outcomes: <strong className="text-(--f1)">{samples}</strong></span>
					</Flex.Row>
					{sparkline.length > 1 ? (
						<Flex.Row align="center" gap={2}>
							<span className="text-[10px] uppercase tracking-wider text-(--f4)">Recent Surprisal Trend</span>
							<SparklineSVG points={sparkline} />
						</Flex.Row>
					) : null}
				</Flex.Row>
			</Panel>

			{/* Subsystem Influence Grid */}
			<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
				<Component registerKey="regulator">
					{({ ref }) => (
						<div ref={ref} className="contents">
							{subsystems.map((sub) => {
								const isExpanding = sub.direction === "expanding" || sub.direction === "restricted";
								const healthColor =
									sub.health === "healthy"
										? "text-(--up) border-(--up)/30"
										: sub.health === "adapting"
											? "text-(--warn) border-(--warn)/30"
											: sub.health === "observing"
												? "text-(--acc) border-(--acc)/30"
												: "text-(--down) border-(--down)/30";

								return (
									<Panel key={sub.name} className="p-4 border border-(--line) bg-(--surface) rounded-md flex flex-col gap-2 justify-between hover:border-(--line2) transition-colors">
										<Flex.Row justify="between" align="center">
											<span className="font-semibold font-mono text-[11px] text-(--f3) uppercase tracking-wider">
												{sub.label}
											</span>
											<span className={`px-2 py-0.5 rounded font-mono text-[10px] font-medium border uppercase ${healthColor}`}>
												{sub.health}
											</span>
										</Flex.Row>

										<Flex.Row align="baseline" gap={2} className="my-1">
											<span className="text-2xl font-bold font-mono text-(--f1)">
												{sub.valueText}
											</span>
											<span className={`font-mono text-[11px] font-semibold ${isExpanding ? "text-(--warn)" : "text-(--up)"}`}>
												{isExpanding ? "▲ " + sub.direction : "● " + sub.direction}
											</span>
										</Flex.Row>

										<p className="font-mono text-[11px] text-(--f4) leading-snug">
											{sub.explanation}
										</p>
									</Panel>
								);
							})}
						</div>
					)}
				</Component>
			</div>

			{/* Educational Footer Legend */}
			<Panel className="p-4 border border-(--line) bg-(--surface)/50 rounded-md font-mono text-[11px] text-(--f4) flex flex-col gap-1.5">
				<span className="font-semibold text-(--f3) uppercase tracking-wider text-[10px]">
					How to Interpret Regulator Influence
				</span>
				<div className="grid grid-cols-3 gap-3 pt-1 text-[10.5px]">
					<div>
						<strong className="text-(--up)">Green (Predictive)</strong>: Prior parameter/outcome pairs beat the zero-return baseline and bounded posterior search is selecting controls.
					</div>
					<div>
						<strong className="text-(--warn)">Amber (Identifying)</strong>: The return model is applying one shrinking coordinate intervention and waiting for its subsequent equity outcome.
					</div>
					<div>
						<strong className="text-(--down)">Red (Adverse Forecast)</strong>: The best evaluated control neighborhood still has a negative posterior mean for next account return.
					</div>
				</div>
			</Panel>
		</div>
	);
};

export const Route = createFileRoute("/regulator")({
	component: RouteComponent,
});
