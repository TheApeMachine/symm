import { useMemo, useState } from "react";
import { Panel } from "#/components/ui/panel";
import { Flex } from "#/components/ui/flex";
import { getLastFrame, registerPainter } from "#/providers/ws-stores";
import type { JSONSerializable } from "#/components/ui/paint";
import { cn } from "@/lib/utils";

/*
RegulatorPredictiveCoding renders the global regulator's live state using the
same predictive-coding visual language as the dashboard: verdict tiles, scalar
diagnostics with paint bindings, and control nodes with health tones.
*/

type RegulatorFrame = {
	status?: string;
	surprise?: number;
	energy?: number;
	predictedReturn?: number;
	predictionScale?: number;
	predictedActive?: number;
	activityScale?: number;
	samples?: number;
	markSamples?: number;
	intervalMarks?: number;
	lastMarkSymbol?: string;
	lastMarkAt?: string;
	lastMarkReturn?: number;
	lastMarkDrawdown?: number;
	lastMarkFloorDistance?: number;
	lastMarkSurgeArmed?: boolean;
	summary?: string;
	subsystems?: Array<{
		name: string;
		label: string;
		health: string;
		direction: string;
		valueText: string;
		explanation: string;
		value: number;
	}>;
	sparkline?: number[];
};

const DEFAULT_FRAME: RegulatorFrame = {
	status: "observing",
	surprise: 0,
	energy: 0,
	samples: 0,
	markSamples: 0,
	sparkline: [],
	subsystems: [],
};

const HEALTH_TONE: Record<string, { dot: string; text: string; border: string }> = {
	healthy: {
		dot: "bg-(--up)",
		text: "text-(--up)",
		border: "border-(--up)/30",
	},
	adapting: {
		dot: "bg-(--warn)",
		text: "text-(--warn)",
		border: "border-(--warn)/30",
	},
	observing: {
		dot: "bg-(--acc)",
		text: "text-(--acc)",
		border: "border-(--acc)/30",
	},
	strained: {
		dot: "bg-(--down)",
		text: "text-(--down)",
		border: "border-(--down)/30",
	},
};

const STATUS_LABEL: Record<string, string> = {
	healthy: "Predictive / Optimizing",
	adapting: "Identifying / Exploring",
	observing: "Observing / Resolving",
	strained: "Adverse Return Forecast",
};

const VerdictTile = ({
	title,
	children,
}: {
	title: string;
	children?: React.ReactNode;
}) => {
	const tone = HEALTH_TONE.observing;

	return (
		<div className={cn("flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2")}>
			<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
				{title}
			</div>
			<div className="flex items-baseline gap-2">
				<span className={cn("size-1.5 shrink-0 self-center rounded-full", tone.dot)} />
				<span className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)">
					{title}
				</span>
			</div>
			{children}
		</div>
	);
};

const ScalarMetric = ({
	label,
	bind,
	format = ".4f",
	tone = "text-(--f1)",
}: {
	label: string;
	bind: string;
	format?: string;
	tone?: string;
}) => (
	<div className="bg-[#0a0907] px-2 py-1.5">
		<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
			{label}
		</div>
		<div
			data-paint={bind}
			data-paint-format={format}
			className={cn("mt-0.5 font-mono text-[11px] tabular-nums", tone)}
		>
			—
		</div>
	</div>
);

const ControlNode = ({
	subsystem,
}: {
	subsystem: NonNullable<RegulatorFrame["subsystems"]>[number];
}) => {
	const tone = HEALTH_TONE[subsystem.health] ?? HEALTH_TONE.observing;
	const isChanged = !["configured", "resolving", "validated"].includes(
		subsystem.direction,
	);

	return (
		<div
			className={cn(
				"flex flex-col gap-2 rounded-md border bg-(--surface) p-3 transition-colors hover:border-(--line2)",
				tone.border,
			)}
		>
			<div className="flex items-center justify-between">
				<span className="font-semibold font-mono text-[11px] text-(--f3) uppercase tracking-wider">
					{subsystem.label}
				</span>
				<span
					className={cn(
						"px-2 py-0.5 rounded font-mono text-[10px] font-medium border uppercase",
						tone.text,
						tone.border,
					)}
				>
					{subsystem.health}
				</span>
			</div>

			<div className="flex items-baseline gap-2">
				<span className="text-2xl font-bold font-mono text-(--f1)">
					{subsystem.valueText}
				</span>
				<span
					className={cn(
						"font-mono text-[11px] font-semibold",
						isChanged ? "text-(--warn)" : "text-(--f4)",
					)}
				>
					● {subsystem.direction}
				</span>
			</div>

			<p className="font-mono text-[11px] text-(--f4) leading-snug">
				{subsystem.explanation}
			</p>
		</div>
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
			<title>Recent predictive-coding reconstruction error</title>
			<path
				d={pathD}
				fill="none"
				stroke="var(--acc)"
				strokeWidth="2"
				strokeLinecap="round"
			/>
		</svg>
	);
};

const RegulatorBridge = ({
	onFrame,
}: {
	onFrame: (frame: RegulatorFrame) => void;
}) => {
	useMemo(() => {
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

export const RegulatorPredictiveCoding = () => {
	const [frame, setFrame] = useState<RegulatorFrame>(DEFAULT_FRAME);

	const status = frame.status ?? DEFAULT_FRAME.status ?? "observing";
	const sparkline = frame.sparkline ?? DEFAULT_FRAME.sparkline ?? [];
	const subsystems: NonNullable<RegulatorFrame["subsystems"]> =
		frame.subsystems ?? DEFAULT_FRAME.subsystems ?? [];

	return (
		<div className="flex h-full min-w-275 flex-col overflow-auto bg-(--bg) p-5 gap-5">
			<RegulatorBridge onFrame={setFrame} />

			{/* Verdict row */}
			<Panel className="grid grid-cols-3 gap-px border border-(--line) bg-(--line)">
				<VerdictTile title="residual model">
					<span
						data-paint="surprise"
						data-paint-format=".4f"
						className="mt-0.5 truncate font-mono text-[11px] text-(--warning)"
					>
						—
					</span>
				</VerdictTile>
				<VerdictTile title="direction skill">
					<span
						data-paint="energy"
						data-paint-format=".3f"
						className="mt-0.5 truncate font-mono text-[11px] text-(--info)"
					>
						—
					</span>
				</VerdictTile>
				<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
					<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						entry gate
					</div>
					<div className="flex items-baseline gap-2">
						<span
							className={cn(
								"size-1.5 shrink-0 self-center rounded-full",
								status === "healthy" ? "bg-(--up)" : "bg-(--warn)",
							)}
						/>
						<span className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)">
							{STATUS_LABEL[status] ?? status}
						</span>
					</div>
				</div>
			</Panel>

			{/* Scalar diagnostics */}
			<Panel className="grid grid-cols-5 gap-px border border-(--line) bg-(--line)">
				<ScalarMetric label="predicted return" bind="predictedReturn" format="+.3f" tone="text-(--f1)" />
				<ScalarMetric label="prediction scale" bind="predictionScale" format=".4f" tone="text-(--f1)" />
				<ScalarMetric label="next-interval activity" bind="predictedActive" format=".3f" tone="text-(--f1)" />
				<ScalarMetric label="activity scale" bind="activityScale" format="±.3f" tone="text-(--f1)" />
				<ScalarMetric label="resolved outcomes" bind="samples" format=".0f" tone="text-(--acc)" />
				<ScalarMetric label="position marks" bind="markSamples" format=".0f" tone="text-(--acc)" />
				<ScalarMetric label="interval marks" bind="intervalMarks" format=".0f" tone="text-(--f2)" />
				<ScalarMetric label="last move" bind="lastMarkReturn" format="+.4f" tone="text-(--up)" />
				<ScalarMetric label="peak drawdown" bind="lastMarkDrawdown" format="+.4f" tone="text-(--down)" />
				<ScalarMetric label="floor distance" bind="lastMarkFloorDistance" format="+.3f" tone="text-(--warn)" />
			</Panel>

			{/* Mark-level context */}
			<Panel className="p-4 border border-(--line) bg-(--surface) rounded-md font-mono text-[11px]">
				<Flex.Row justify="between" align="center" className="gap-4">
					<Flex.Column gap={1}>
						<span className="text-[10px] text-(--f4) uppercase tracking-wider">
							Mark-level regulator context
						</span>
						<span className="text-(--f3)">
							Every executable position mark conditions the next complete
							account-level control update.
						</span>
					</Flex.Column>
					<Flex.Row gap={5} className="shrink-0 text-(--f4)">
						<span>
							last symbol{" "}
							<strong className="text-(--f1)" data-paint="lastMarkSymbol">
								—
							</strong>
						</span>
						<span>
							observed <strong className="text-(--f1)" data-paint="lastMarkAt">—</strong>
						</span>
						<span>
							surge{" "}
							<strong
								className={cn(
									"text-(--f1)",
									frame.lastMarkSurgeArmed && "text-(--warn)",
								)}
								data-paint="lastMarkSurgeArmed"
							>
								—
							</strong>
						</span>
					</Flex.Row>
				</Flex.Row>
			</Panel>

			{/* Control nodes */}
			<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
				{subsystems.map((sub) => (
					<ControlNode key={sub.name} subsystem={sub} />
				))}
			</div>

			{/* Reconstruction error trend */}
			{sparkline.length > 1 ? (
				<Panel className="p-4 border border-(--line) bg-(--surface) rounded-md">
					<Flex.Row align="center" gap={2}>
						<span className="text-[10px] uppercase tracking-wider text-(--f4)">
							Recent Surprisal Trend
						</span>
						<SparklineSVG points={sparkline} />
					</Flex.Row>
				</Panel>
			) : null}

			{/* Interpretation legend */}
			<Panel className="p-4 border border-(--line) bg-(--surface)/50 rounded-md font-mono text-[11px] text-(--f4) flex flex-col gap-1.5">
				<span className="font-semibold text-(--f3) uppercase tracking-wider text-[10px]">
					How to Interpret the Regulator
				</span>
				<div className="grid grid-cols-3 gap-3 pt-1 text-[10.5px]">
					<div>
						<strong className="text-(--up)">Green (Predictive)</strong>: Prior
						parameter/outcome pairs beat the zero-return baseline and bounded
						posterior search is selecting controls.
					</div>
					<div>
						<strong className="text-(--warn)">Amber (Identifying)</strong>: The
						return model is applying one shrinking coordinate intervention and
						waiting for its subsequent equity outcome.
					</div>
					<div>
						<strong className="text-(--down)">Red (Adverse Forecast)</strong>:
						The selected control vector has a negative posterior mean for next
						account return.
					</div>
				</div>
			</Panel>
		</div>
	);
};
