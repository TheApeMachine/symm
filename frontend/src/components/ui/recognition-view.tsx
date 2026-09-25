import { Badge } from "#/components/ui/badge";
import { basis, percent } from "#/components/ui/learning-format";
import { Stat } from "#/components/ui/stat";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";

export interface ActivityRow {
	id: string;
	time: string;
	actionStr: string;
	edgeStr: string;
	pnl?: number;
}

export interface RecognitionMetrics {
	decisions?: number | string;
	steps?: number | string;
	evaluated?: number | string;
	accuracy?: number;
	unsupported?: number | string;
	resolved?: number | string;
	edge?: number;
	action?: number | string;
	predictedAction?: number | string;
	predictedConfidence?: number;
	truthAction?: number | string;
	input_count?: number | string;
	invalid_inputs?: number | string;
}
export interface RecognitionViewProps {
	className?: string;
	metricMap?: RecognitionMetrics | null;
	recentActivity?: ActivityRow[] | null;
}

export const RecognitionView = ({
	metricMap: suppliedMetrics,
	recentActivity: suppliedActivity,
	className,
}: RecognitionViewProps) => {
	const metricMap = suppliedMetrics ?? {};
	const recentActivity = suppliedActivity ?? [];
	const decisions = metricMap.decisions;
	const steps = metricMap.steps;
	const evaluated = metricMap.evaluated;
	const accuracy = metricMap.accuracy;
	const unsupported = metricMap.unsupported;
	const resolved = metricMap.resolved;
	const edge = metricMap.edge;
	const actionVal = metricMap.action;

	const actionLabels: Record<string, string> = {
		"0": "WAIT",
		"1": "ENTER",
		"2": "EXIT",
		"3": "RETREAT",
	};

	const formatAction = (val?: number | string) => {
		if (val === undefined || val === null || val === "" || val === "—") return "—";
		return actionLabels[String(val)] ?? String(val);
	};

	const predictedVal = metricMap.predictedAction;
	const confidenceVal = metricMap.predictedConfidence;
	const truthVal = metricMap.truthAction ?? (predictedVal === undefined ? actionVal : undefined);

	const predictedLabel = formatAction(predictedVal ?? (truthVal === undefined ? actionVal : undefined));
	const truthLabel = formatAction(truthVal);

	return (
		<div
			className={cn(
				"flex h-full w-full min-h-0 flex-col overflow-hidden bg-(--bg) font-mono text-xs text-(--f2)",
				className,
			)}
		>
			{/* Top: Core Metrics Ribbon - Flush with border-b */}
			<div className="flex shrink-0 flex-col border-b border-(--line) bg-(--surface) p-4">
				<div className="mb-3 flex items-center justify-between">
					<div className="flex items-center gap-2">
						<Typography.Label size="s" tone="f4" weight="normal">
							CAUSAL PRECURSOR RECOGNITION
						</Typography.Label>
						<Badge variant="info" label="MARKET OBSERVER" size="s" />
					</div>
					<Typography.Mono size="s" tone="f4">
						{steps === undefined ? "—" : steps.toLocaleString()} frames observed
					</Typography.Mono>
				</div>

				<div className="grid grid-cols-2 gap-4 md:grid-cols-6 pt-1">
					<Stat
						label="Precursor Observations"
						value={decisions === undefined ? "—" : decisions.toLocaleString()}
					/>
					<Stat
						label="Evaluations"
						value={evaluated === undefined ? "—" : evaluated.toLocaleString()}
					/>
					<Stat
						label="Accuracy"
						value={accuracy === undefined ? "—" : percent(accuracy)}
					/>
					<Stat
						label="Quoted Edge"
						value={edge !== undefined ? basis(edge) : "—"}
					/>
					<Stat
						label="Unsupported Context"
						value={
							unsupported === undefined ? "—" : unsupported.toLocaleString()
						}
					/>
					<Stat
						label="Resolved Outcomes"
						value={resolved === undefined ? "—" : resolved.toLocaleString()}
					/>
				</div>
			</div>

			{/* Middle Split: Causal Policy Candidates & Activity Stream - Flush with hairline borders */}
			<div className="flex flex-1 min-h-0 overflow-hidden">
				{/* Left: Policy & Impulse Context */}
				<div className="flex flex-1 min-w-0 flex-col border-r border-(--line) bg-(--surface) overflow-hidden">
					<div className="flex h-8 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4 text-[11px] text-(--f4)">
						<span className="font-bold text-(--f2)">
							AUTONOMOUS POLICY DECISION
						</span>
						<span>Causal Precursor Association</span>
					</div>

					<div className="flex flex-1 flex-col gap-4 p-4 overflow-hidden">
						<div className="flex flex-col gap-3">
							<div>
								<Typography.Label size="s" tone="f4" weight="normal">
									PREDICTED POLICY ACTION
								</Typography.Label>
								<div className="mt-1 flex items-center gap-3">
									<Badge
										size="m"
										variant={
											predictedLabel === "ENTER"
												? "success"
												: predictedLabel === "EXIT"
													? "error"
													: predictedLabel === "WAIT"
														? "neutral"
														: "warning"
										}
										label={`PREDICTED: ${predictedLabel}`}
										dot
									/>
									<span className="text-(--f3) text-xs">
										conf: {confidenceVal !== undefined ? percent(confidenceVal) : "—"}
									</span>
									<span className="font-bold text-(--f1) text-xs ml-auto">
										{edge !== undefined ? basis(edge) : "—"} edge
									</span>
								</div>
							</div>

							<div className="border-t border-(--line)/40 pt-2">
								<Typography.Label size="s" tone="f4" weight="normal">
									SUPERVISED TRUTH
								</Typography.Label>
								<div className="mt-1 flex items-center gap-3">
									<Badge
										size="s"
										variant={
											truthLabel === "ENTER"
												? "success"
												: truthLabel === "EXIT"
													? "error"
													: "neutral"
										}
										label={`TRUTH: ${truthLabel}`}
									/>
									<Typography.Mono size="s" tone="f4" className="text-[10px]">
										Training truth supplied by the excursion grader
									</Typography.Mono>
								</div>
							</div>
						</div>

						<div className="border-t border-(--line) pt-4">
							<Typography.Label size="s" tone="f4" weight="normal">
								CONTEXT EVIDENCE
							</Typography.Label>
							<div className="mt-2 grid grid-cols-2 gap-3 text-[11px]">
								<div className="rounded border border-(--line) bg-(--sunken) p-3">
									<div className="text-[10px] text-(--f4)">Producer Inputs</div>
									<div className="mt-1 text-base font-bold text-(--f1)">
										{metricMap.input_count === undefined
											? "—"
											: metricMap.input_count.toLocaleString()}
									</div>
								</div>
								<div className="rounded border border-(--line) bg-(--sunken) p-3">
									<div className="text-[10px] text-(--f4)">
										Invalid Contexts
									</div>
									<div className="mt-1 text-base font-bold text-(--acc)">
										{metricMap.invalid_inputs === undefined
											? "—"
											: metricMap.invalid_inputs.toLocaleString()}
									</div>
								</div>
							</div>
						</div>
					</div>
				</div>

				{/* Right: Live Telemetry Activity Stream */}
				<div className="flex w-96 shrink-0 flex-col bg-(--surface) overflow-hidden">
					<div className="flex h-8 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4 text-[11px] text-(--f4)">
						<span className="font-bold text-(--f2)">
							RECENT ACTIVITY STREAM
						</span>
						<span>Recorded moments</span>
					</div>

					<div className="flex-1 overflow-hidden p-2">
						<div className="flex flex-col divide-y divide-(--line)/40 text-[11px]">
							{recentActivity.length === 0 && (
								<Typography.Mono>No recorded activity</Typography.Mono>
							)}
							{recentActivity.map((act) => (
								<div
									key={act.id}
									className="flex items-center justify-between py-2 px-3 hover:bg-(--raised)"
								>
									<span className="w-18 shrink-0 text-(--f4)">{act.time}</span>
									<span className="flex-1 truncate text-(--f2) px-2">
										{act.actionStr}
									</span>
									<span
										className={`shrink-0 font-bold ${
											act.edgeStr.startsWith("+")
												? "text-(--up)"
												: act.edgeStr.startsWith("-")
													? "text-(--down)"
													: "text-(--acc)"
										}`}
									>
										{act.edgeStr}
									</span>
								</div>
							))}
						</div>
					</div>

					<div className="border-t border-(--line) px-4 py-2 text-[9.5px] text-(--f4)">
						One row per recorded moment: timestamp, action policy call, and tape
						benefit.
					</div>
				</div>
			</div>
		</div>
	);
};
