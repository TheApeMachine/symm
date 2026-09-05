import { useRef } from "react";
import { regulatorStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";
import { computeSparklinePath } from "#/components/ui/sparkline";
import { cn } from "#/lib/utils";
import { Subsystem } from "#/providers/telemetry/telemetry/subsystem";

const STATUS_LABEL: Record<string, string> = {
	healthy: "Predictive / Optimizing",
	adapting: "Identifying / Exploring",
	observing: "Observing / Resolving",
	strained: "Adverse Return Forecast",
};

const METRICS = [
	{ which: "predictedReturn", label: "predicted return", digits: 3 },
	{ which: "predictionScale", label: "prediction scale", digits: 3 },
	{ which: "predictedActive", label: "next-interval activity", digits: 3 },
	{ which: "activityScale", label: "activity scale", digits: 3 },
	{ which: "samples", label: "resolved outcomes", digits: 0 },
	{ which: "markSamples", label: "position marks", digits: 0 },
	{ which: "intervalMarks", label: "interval marks", digits: 0 },
	{ which: "lastMarkReturn", label: "last move", digits: 3 },
	{ which: "lastMarkDrawdown", label: "peak drawdown", digits: 3 },
	{ which: "lastMarkFloorDistance", label: "floor distance", digits: 3 },
] as const;

const num = (value: unknown, digits: number): string =>
	typeof value === "number"
		? value.toFixed(digits)
		: typeof value === "bigint"
			? String(value)
			: "—";

const ScalarMetric = ({
	label,
	which,
	tone = "text-(--f1)",
}: {
	label: string;
	which: string;
	tone?: string;
}) => (
	<div className="bg-[#0a0907] px-2 py-1.5">
		<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
			{label}
		</div>
		<div
			data-r={which}
			className={cn("mt-0.5 font-mono text-[11px] tabular-nums", tone)}
		>
			—
		</div>
	</div>
);

type QueryEntry = {
	elements: Record<string, HTMLElement | null>;
	gate: HTMLElement | null;
	sparkline: SVGPathElement | null;
	subElements: Record<
		string,
		{
			val: HTMLElement | null;
			dir: HTMLElement | null;
			hlth: HTMLElement | null;
		}
	>;
};

let queryCache: QueryEntry | null = null;
const subObj = new Subsystem();

export const RegulatorPredictiveCoding = () => {
	const root = useRef<HTMLDivElement>(null);

	regulatorStore.subscribe((state) => {
		if (!root.current) return;
		const frame = state.getLast();
		if (!frame) return;

		if (!queryCache) {
			const elements: Record<string, HTMLElement | null> = {};
			const metricKeys = [
				"surprise",
				"energy",
				"predictedReturn",
				"predictionScale",
				"predictedActive",
				"activityScale",
				"samples",
				"markSamples",
				"intervalMarks",
				"lastMarkReturn",
				"lastMarkDrawdown",
				"lastMarkFloorDistance",
				"status",
				"lastSymbol",
				"lastAt",
				"surge",
			];
			for (const k of metricKeys) {
				elements[k] = root.current.querySelector<HTMLElement>(
					`[data-r="${k}"]`,
				);
			}

			const subElements: Record<
				string,
				{
					val: HTMLElement | null;
					dir: HTMLElement | null;
					hlth: HTMLElement | null;
				}
			> = {};
			const subsContainer =
				root.current.querySelector<HTMLElement>("[data-subs]");
			if (subsContainer) {
				for (const name of [
					"risk",
					"timing",
					"alpha",
					"liquidity",
					"toxicity",
					"inventory",
				]) {
					subElements[name] = {
						val: subsContainer.querySelector<HTMLElement>(
							`[data-sub-val="${name}"]`,
						),
						dir: subsContainer.querySelector<HTMLElement>(
							`[data-sub-dir="${name}"]`,
						),
						hlth: subsContainer.querySelector<HTMLElement>(
							`[data-sub-hlth="${name}"]`,
						),
					};
				}
			}

			queryCache = {
				elements,
				gate: root.current.querySelector<HTMLElement>("[data-gate]"),
				sparkline: root.current.querySelector<SVGPathElement>(
					"[data-k=regulator-sparkline]",
				),
				subElements,
			};
		}

		const set = (q: string, value: string) => {
			const el = queryCache?.elements[q];
			if (el) el.textContent = value;
		};

		set("surprise", num(frame.surprise(), 4));
		set("energy", num(frame.energy(), 3));
		set("predictedReturn", num(frame.predictedReturn(), 3));
		set("predictionScale", num(frame.predictionScale(), 3));
		set("predictedActive", num(frame.predictedActive(), 3));
		set("activityScale", num(frame.activityScale(), 3));
		set("samples", num(frame.samples(), 0));
		set("markSamples", num(frame.markSamples(), 0));
		set("intervalMarks", num(frame.intervalMarks(), 0));
		set("lastMarkReturn", num(frame.lastMarkReturn(), 3));
		set("lastMarkDrawdown", num(frame.lastMarkDrawdown(), 3));
		set("lastMarkFloorDistance", num(frame.lastMarkFloorDistance(), 3));

		const statusStr = frame.status() ?? "observing";
		set("status", STATUS_LABEL[statusStr] ?? statusStr);
		set("lastSymbol", String(frame.lastMarkSymbol() ?? "—"));
		set("lastAt", String(frame.lastMarkAt() ?? "—"));
		set("surge", String(frame.lastMarkSurgeArmed()));

		if (queryCache.gate) {
			queryCache.gate.className = cn(
				"size-1.5 shrink-0 self-center rounded-full",
				statusStr === "healthy" ? "bg-(--up)" : "bg-(--warn)",
			);
		}

		if (queryCache.sparkline) {
			const pts: number[] = [];
			for (let i = 0; i < frame.sparklineLength(); i++) {
				const pt = frame.sparkline(i);
				if (pt !== null) pts.push(pt);
			}
			if (pts.length > 1) {
				queryCache.sparkline.setAttribute(
					"d",
					computeSparklinePath(pts, 300, 40, 4),
				);
			}
		}

		for (let i = 0; i < frame.subsystemsLength(); i++) {
			const sub = frame.subsystems(i, subObj);
			if (!sub) continue;
			const name = sub.name() ?? "";
			const subEntry = queryCache.subElements[name];
			if (!subEntry) continue;

			if (subEntry.val) subEntry.val.textContent = sub.valueText() ?? "—";
			if (subEntry.dir)
				subEntry.dir.textContent = `● ${sub.direction() ?? "—"}`;
			if (subEntry.hlth) subEntry.hlth.textContent = sub.health() ?? "—";
		}
	});

	return (
		<div
			ref={root}
			className="flex h-full min-w-275 flex-col overflow-auto bg-(--bg) p-5 gap-5"
		>
			<div className="font-mono text-[13px] font-semibold text-(--f1) uppercase tracking-[0.13em]">
				Global Predictive-Coding Regulator
			</div>
			<Panel className="grid grid-cols-3 gap-px border border-(--line) bg-(--line)">
				<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
					<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						residual model
					</div>
					<div className="flex items-baseline gap-2">
						<span className="size-1.5 shrink-0 self-center rounded-full bg-(--acc)" />
						<span
							data-r="surprise"
							className="truncate font-mono text-[13px] uppercase tracking-wide text-(--warning)"
						>
							—
						</span>
					</div>
				</div>
				<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
					<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						direction skill
					</div>
					<div className="flex items-baseline gap-2">
						<span className="size-1.5 shrink-0 self-center rounded-full bg-(--acc)" />
						<span
							data-r="energy"
							className="truncate font-mono text-[13px] uppercase tracking-wide text-(--info)"
						>
							—
						</span>
					</div>
				</div>
				<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
					<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						entry gate
					</div>
					<div className="flex items-baseline gap-2">
						<span
							data-gate
							className="size-1.5 shrink-0 self-center rounded-full bg-(--warn)"
						/>
						<span
							data-r="status"
							className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)"
						>
							Observing / Resolving
						</span>
					</div>
				</div>
			</Panel>

			<Panel className="grid grid-cols-5 gap-px border border-(--line) bg-(--line)">
				{METRICS.map((metric) => (
					<ScalarMetric
						key={metric.which}
						label={metric.label}
						which={metric.which}
					/>
				))}
			</Panel>

			<Panel className="p-4 border border-(--line) bg-(--surface) rounded-md font-mono text-[11px]">
				<Flex.Row justify="between" align="center" className="gap-4">
					<Flex.Column gap={1}>
						<span className="text-[10px] text-(--f4) uppercase tracking-wider">
							Position Marks
						</span>
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
							<strong className="text-(--f1)" data-r="lastSymbol">
								—
							</strong>
						</span>
						<span>
							observed{" "}
							<strong className="text-(--f1)" data-r="lastAt">
								—
							</strong>
						</span>
						<span>
							surge{" "}
							<strong className="text-(--f1)" data-r="surge">
								—
							</strong>
						</span>
					</Flex.Row>
				</Flex.Row>
			</Panel>

			<div
				data-subs
				className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4"
			>
				{[
					{ name: "risk", label: "Risk & Position Sizing" },
					{ name: "timing", label: "Execution Timing" },
					{ name: "alpha", label: "Alpha Signal Weight" },
					{ name: "liquidity", label: "Liquidity Capture" },
					{ name: "toxicity", label: "Orderflow Toxicity" },
					{ name: "inventory", label: "Inventory Skew" },
				].map((sub) => (
					<div
						key={sub.name}
						className="flex flex-col gap-2 rounded-md border border-(--line) bg-(--surface) p-3 transition-colors hover:border-(--line2)"
					>
						<div className="flex items-center justify-between">
							<span className="font-semibold font-mono text-[11px] text-(--f3) uppercase tracking-wider">
								{sub.label}
							</span>
							<span
								data-sub-hlth={sub.name}
								className="px-2 py-0.5 rounded font-mono text-[10px] font-medium border border-(--line) text-(--f3) uppercase"
							>
								observing
							</span>
						</div>
						<div className="flex items-baseline gap-2">
							<span
								data-sub-val={sub.name}
								className="text-2xl font-bold font-mono text-(--f1)"
							>
								—
							</span>
							<span
								data-sub-dir={sub.name}
								className="font-mono text-[11px] font-semibold text-(--f4)"
							>
								● configured
							</span>
						</div>
					</div>
				))}
			</div>

			<Panel className="p-4 border border-(--line) bg-(--surface) rounded-md">
				<Flex.Row align="center" gap={4}>
					<span className="text-[10px] uppercase tracking-wider text-(--f4)">
						Recent Surprisal Trend
					</span>
					<svg width={300} height={40} className="overflow-visible">
						<path
							data-k="regulator-sparkline"
							d=""
							fill="none"
							stroke="var(--acc)"
							strokeWidth="2"
							strokeLinecap="round"
						/>
					</svg>
				</Flex.Row>
			</Panel>

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
