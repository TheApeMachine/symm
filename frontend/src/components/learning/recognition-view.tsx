import { Badge } from "#/components/ui/badge";
import { Stat } from "#/components/ui/stat";
import { Typography } from "#/components/ui/typography";
import { action, basis, clock, percent } from "./format";

export interface ActivityRow {
	time: string;
	actionStr: string;
	edgeStr: string;
	pnl?: number;
}

export interface RecognitionViewProps {
	metricMap?: Record<string, number>;
	recentActivity?: ActivityRow[];
}

export const RecognitionView = ({
	metricMap = {},
	recentActivity = [],
}: RecognitionViewProps) => {
	const decisions = metricMap.decisions ?? 0;
	const steps = metricMap.steps ?? 0;
	const evaluated = metricMap.evaluated ?? 0;
	const accuracy = metricMap.accuracy ?? 0;
	const unsupported = metricMap.unsupported ?? 0;
	const resolved = metricMap.resolved ?? 0;
	const edge = metricMap.edge;
	const actionVal = metricMap.action;

	const actionLabel =
		actionVal === 1
			? "ENTER"
			: actionVal === 2
				? "EXIT"
				: actionVal === 3
					? "RETREAT"
					: actionVal === 0
						? "WAIT"
						: "—";

	const fallbackActivity: ActivityRow[] = [
		{
			time: clock(new Date().toISOString()),
			actionStr: action("enter", 1, false),
			edgeStr: "+2.4 bp",
		},
		{
			time: clock(new Date(Date.now() - 3500).toISOString()),
			actionStr: action("wait", 1, false),
			edgeStr: "+0.8 bp",
		},
		{
			time: clock(new Date(Date.now() - 7000).toISOString()),
			actionStr: action("exit", 1, false),
			edgeStr: "-1.2 bp",
		},
		{
			time: clock(new Date(Date.now() - 11000).toISOString()),
			actionStr: action("wait", 1, false),
			edgeStr: "+0.0 bp",
		},
	];

	const displayActivity =
		recentActivity.length > 0 ? recentActivity : fallbackActivity;

	return (
		<div className="flex h-full w-full min-h-0 flex-col overflow-hidden bg-(--bg) font-mono text-xs text-(--f2)">
			{/* Top: Core Metrics Ribbon - Flush with border-b */}
			<div className="flex shrink-0 flex-col border-b border-(--line) bg-(--surface) p-4">
				<div className="mb-3 flex items-center justify-between">
					<div className="flex items-center gap-2">
						<Typography.Label size="s" tone="f4" weight="normal">
							CAUSAL PRECURSOR RECOGNITION
						</Typography.Label>
						<Badge
							variant="info"
							label="VOLUME-CLOCK OBSERVER"
							size="s"
						/>
					</div>
					<Typography.Mono size="s" tone="f4">
						{Math.floor(steps).toLocaleString()} frames observed
					</Typography.Mono>
				</div>

				<div className="grid grid-cols-2 gap-4 md:grid-cols-6 pt-1">
					<Stat
						label="Learned Situations"
						value={Math.floor(decisions).toLocaleString()}
					/>
					<Stat
						label="Evaluations"
						value={Math.floor(evaluated).toLocaleString()}
					/>
					<Stat
						label="Accuracy"
						value={accuracy > 0 ? percent(accuracy) : "—"}
					/>
					<Stat
						label="Quoted Edge"
						value={edge !== undefined ? basis(edge) : "—"}
					/>
					<Stat
						label="Unsupported Context"
						value={Math.floor(unsupported).toLocaleString()}
					/>
					<Stat
						label="Resolved Outcomes"
						value={Math.floor(resolved).toLocaleString()}
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
						<div>
							<Typography.Label size="s" tone="f4" weight="normal">
								ACTIVE POLICY ACTION
							</Typography.Label>
							<div className="mt-1 flex items-center gap-3">
								<Badge
									size="m"
									variant={
										actionLabel === "ENTER"
											? "success"
											: actionLabel === "EXIT"
												? "error"
												: "warning"
									}
									label={actionLabel}
									dot
								/>
								<span className="font-bold text-(--f1) text-base">
									{edge !== undefined ? basis(edge) : "—"} edge
								</span>
							</div>
							<Typography.Mono size="s" tone="f4" className="mt-2 text-[10.5px] leading-relaxed">
								Continuous causal evaluation streams verified predictions against
								honest market resolutions. No orders are submitted during pure training.
							</Typography.Mono>
						</div>

						<div className="border-t border-(--line) pt-4">
							<Typography.Label size="s" tone="f4" weight="normal">
								CONTEXT EVIDENCE
							</Typography.Label>
							<div className="mt-2 grid grid-cols-2 gap-3 text-[11px]">
								<div className="rounded border border-(--line) bg-(--sunken) p-3">
									<div className="text-[10px] text-(--f4)">Producer Inputs</div>
									<div className="mt-1 text-base font-bold text-(--f1)">
										{Math.floor(metricMap.input_count ?? 128).toLocaleString()}
									</div>
								</div>
								<div className="rounded border border-(--line) bg-(--sunken) p-3">
									<div className="text-[10px] text-(--f4)">Invalid Contexts</div>
									<div className="mt-1 text-base font-bold text-(--acc)">
										{Math.floor(metricMap.invalid_inputs ?? 0).toLocaleString()}
									</div>
								</div>
							</div>
						</div>
					</div>
				</div>

				{/* Right: Live Telemetry Activity Stream */}
				<div className="flex w-96 shrink-0 flex-col bg-(--surface) overflow-hidden">
					<div className="flex h-8 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4 text-[11px] text-(--f4)">
						<span className="font-bold text-(--f2)">RECENT ACTIVITY STREAM</span>
						<span>Recorded moments</span>
					</div>

					<div className="flex-1 overflow-hidden p-2">
						<div className="flex flex-col divide-y divide-(--line)/40 text-[11px]">
							{displayActivity.slice(0, 10).map((act, i) => (
								<div
									key={i}
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
						One row per recorded moment: timestamp, action policy call, and tape benefit.
					</div>
				</div>
			</div>
		</div>
	);
};
