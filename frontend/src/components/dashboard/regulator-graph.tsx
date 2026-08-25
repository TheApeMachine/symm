import { Panel } from "#/components/ui/panel";
import { Flex } from "#/components/ui/flex";
import { cn } from "@/lib/utils";
import { regulatorStore, useSubscribe } from "#/providers/ws-stores";

type RegulatorSubsystem = {
	name: string;
	label: string;
	health: string;
	direction: string;
	valueText: string;
	explanation: string;
	value: number;
};

type RegulatorFrame = {
	status?: string;
	subsystems?: RegulatorSubsystem[];
	sparkline?: number[];
	lastMarkSurgeArmed?: boolean;
	[x: string]: unknown;
};

const HEALTH_TONE: Record<string, { dot: string; text: string; border: string }> = {
	healthy: { dot: "bg-(--up)", text: "text-(--up)", border: "border-(--up)/30" },
	adapting: { dot: "bg-(--warn)", text: "text-(--warn)", border: "border-(--warn)/30" },
	observing: { dot: "bg-(--acc)", text: "text-(--acc)", border: "border-(--acc)/30" },
	strained: { dot: "bg-(--down)", text: "text-(--down)", border: "border-(--down)/30" },
};

const STATUS_LABEL: Record<string, string> = {
	healthy: "Predictive / Optimizing",
	adapting: "Identifying / Exploring",
	observing: "Observing / Resolving",
	strained: "Adverse Return Forecast",
};

const VerdictTile = ({ title, children }: { title: string; children?: React.ReactNode }) => {
	const tone = HEALTH_TONE.observing;

	return (
		<div className={cn("flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2")}>
			<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">{title}</div>
			<div className="flex items-baseline gap-2">
				<span className={cn("size-1.5 shrink-0 self-center rounded-full", tone.dot)} />
				<span className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)">{title}</span>
			</div>
			{children}
		</div>
	);
};

const ScalarMetric = ({ label, which, tone = "text-(--f1)" }: { label: string; which: string; tone?: string }) => (
	<div className="bg-[#0a0907] px-2 py-1.5">
		<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">{label}</div>
		<div data-metric={which} className={cn("mt-0.5 font-mono text-[11px] tabular-nums", tone)}>—</div>
	</div>
);

const ControlNode = ({ subsystem }: { subsystem: RegulatorSubsystem }) => {
	const tone = HEALTH_TONE[subsystem.health] ?? HEALTH_TONE.observing;
	const isChanged = !["configured", "resolving", "validated"].includes(subsystem.direction);

	return (
		<div className={cn("flex flex-col gap-2 rounded-md border bg-(--surface) p-3 transition-colors hover:border-(--line2)", tone.border)}>
			<div className="flex items-center justify-between">
				<span className="font-semibold font-mono text-[11px] text-(--f3) uppercase tracking-wider">{subsystem.label}</span>
				<span className={cn("px-2 py-0.5 rounded font-mono text-[10px] font-medium border uppercase", tone.text, tone.border)}>{subsystem.health}</span>
			</div>
			<div className="flex items-baseline gap-2">
				<span className="text-2xl font-bold font-mono text-(--f1)">{subsystem.valueText}</span>
				<span className={cn("font-mono text-[11px] font-semibold", isChanged ? "text-(--warn)" : "text-(--f4)")}>● {subsystem.direction}</span>
			</div>
			<p className="font-mono text-[11px] text-(--f4) leading-snug">{subsystem.explanation}</p>
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
			<path d={pathD} fill="none" stroke="var(--acc)" strokeWidth="2" strokeLinecap="round" />
		</svg>
	);
};

const METRICS = [
	{ which: "predictedReturn", label: "predicted return" },
	{ which: "predictionScale", label: "prediction scale" },
	{ which: "predictedActive", label: "next-interval activity" },
	{ which: "activityScale", label: "activity scale" },
	{ which: "samples", label: "resolved outcomes" },
	{ which: "markSamples", label: "position marks" },
	{ which: "intervalMarks", label: "interval marks" },
	{ which: "lastMarkReturn", label: "last move" },
	{ which: "lastMarkDrawdown", label: "peak drawdown" },
	{ which: "lastMarkFloorDistance", label: "floor distance" },
] as const;

const num = (value: unknown, digits: number): string =>
	typeof value === "number" ? value.toFixed(digits) : "—";

export const RegulatorPredictiveCoding = () => {
	const root = useSubscribe(regulatorStore, (state) => {
		const frame = (state ?? {}) as RegulatorFrame;

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-r="${q}"]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		set("surprise", num(frame.surprise, 4));
		set("energy", num(frame.energy, 3));

		for (const metric of METRICS) {
			set(metric.which, num(frame[metric.which], metric.which === "samples" || metric.which === "markSamples" || metric.which === "intervalMarks" ? 0 : 3));
		}

		set("status", STATUS_LABEL[frame.status ?? "observing"] ?? frame.status ?? "Observing / Resolving");
		set("lastSymbol", String(frame.lastMarkSymbol ?? "—"));
		set("lastAt", String(frame.lastMarkAt ?? "—"));
		set("surge", String(frame.lastMarkSurgeArmed ?? false));

		const gate = root.current?.querySelector<HTMLElement>("[data-gate]");
		if (gate instanceof HTMLElement) {
			gate.className = cn("size-1.5 shrink-0 self-center rounded-full", frame.status === "healthy" ? "bg-(--up)" : "bg-(--warn)");
		}
	});

	const subsystems = ((regulatorStore.state ?? {}) as RegulatorFrame).subsystems ?? [];
	const sparkline = ((regulatorStore.state ?? {}) as RegulatorFrame).sparkline ?? [];
	return (
		<div ref={root} className="flex h-full min-w-275 flex-col overflow-auto bg-(--bg) p-5 gap-5">
			<div className="font-mono text-[13px] font-semibold text-(--f1) uppercase tracking-[0.13em]">
				Global Predictive-Coding Regulator
			</div>
			<Panel className="grid grid-cols-3 gap-px border border-(--line) bg-(--line)">
				<VerdictTile title="residual model">
					<span data-r="surprise" className="mt-0.5 truncate font-mono text-[11px] text-(--warning)">—</span>
				</VerdictTile>
				<VerdictTile title="direction skill">
					<span data-r="energy" className="mt-0.5 truncate font-mono text-[11px] text-(--info)">—</span>
				</VerdictTile>
				<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
					<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">entry gate</div>
					<div className="flex items-baseline gap-2">
						<span data-gate className="size-1.5 shrink-0 self-center rounded-full bg-(--warn)" />
						<span data-r="status" className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)">Observing / Resolving</span>
					</div>
				</div>
			</Panel>

			<Panel className="grid grid-cols-5 gap-px border border-(--line) bg-(--line)">
				{METRICS.map((metric) => (
					<ScalarMetric key={metric.which} label={metric.label} which={metric.which} />
				))}
			</Panel>

			<Panel className="p-4 border border-(--line) bg-(--surface) rounded-md font-mono text-[11px]">
				<Flex.Row justify="between" align="center" className="gap-4">
					<Flex.Column gap={1}>
						<span className="text-[10px] text-(--f4) uppercase tracking-wider">Position Marks</span>
						<span className="text-[10px] text-(--f4) uppercase tracking-wider">Mark-level regulator context</span>
						<span className="text-(--f3)">Every executable position mark conditions the next complete account-level control update.</span>
					</Flex.Column>
					<Flex.Row gap={5} className="shrink-0 text-(--f4)">
						<span>last symbol <strong className="text-(--f1)" data-r="lastSymbol">—</strong></span>
						<span>observed <strong className="text-(--f1)" data-r="lastAt">—</strong></span>
						<span>surge <strong className="text-(--f1)" data-r="surge">—</strong></span>
					</Flex.Row>
				</Flex.Row>
			</Panel>

			<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
				{subsystems.map((sub) => (
					<ControlNode key={sub.name} subsystem={sub} />
				))}
			</div>

			{sparkline.length > 1 ? (
				<Panel className="p-4 border border-(--line) bg-(--surface) rounded-md">
					<Flex.Row align="center" gap={2}>
						<span className="text-[10px] uppercase tracking-wider text-(--f4)">Recent Surprisal Trend</span>
						<SparklineSVG points={sparkline} />
					</Flex.Row>
				</Panel>
			) : null}

			<Panel className="p-4 border border-(--line) bg-(--surface)/50 rounded-md font-mono text-[11px] text-(--f4) flex flex-col gap-1.5">
				<span className="font-semibold text-(--f3) uppercase tracking-wider text-[10px]">How to Interpret the Regulator</span>
				<div className="grid grid-cols-3 gap-3 pt-1 text-[10.5px]">
					<div><strong className="text-(--up)">Green (Predictive)</strong>: Prior parameter/outcome pairs beat the zero-return baseline and bounded posterior search is selecting controls.</div>
					<div><strong className="text-(--warn)">Amber (Identifying)</strong>: The return model is applying one shrinking coordinate intervention and waiting for its subsequent equity outcome.</div>
					<div><strong className="text-(--down)">Red (Adverse Forecast)</strong>: The selected control vector has a negative posterior mean for next account return.</div>
				</div>
			</Panel>
		</div>
	);
};