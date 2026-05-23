import {
	useSymmConnected,
	useSymmDecisionTrace,
	useSymmEnginePulse,
	useSymmScoreboard,
} from "#/lib/symm/use-symm-ui";
import { SidebarSection } from "./sidebar";
import { whyLabel, type DecisionTraceEvent } from "#/lib/symm/events";
import { EmptyHint } from "./hint";
import { VerdictBadge } from "#/components/verdict";

export const DecisionsPanel = () => {
	const connected = useSymmConnected();
	const decisionTrace = useSymmDecisionTrace();
	const scoreboard = useSymmScoreboard();
	const pulse = useSymmEnginePulse();

	const decisions = decisionTrace?.decisions ?? [];
	const targets = scoreboard?.targets ?? [];
	const signals = pulse?.signals ?? [];

	return (
		<SidebarSection title="Decisions" fill className="min-w-0">
			{pulse ? <EnginePulseStrip pulse={pulse} connected={connected} /> : null}
			{decisions.length > 0 ? (
				<DecisionTable decisions={decisions} />
			) : targets.length > 0 ? (
				<ScoreboardTable targets={targets} />
			) : signals.length > 0 ? (
				<SignalTable signals={signals} />
			) : (
				<EmptyHint
					connected={connected}
					message={
						connected
							? pulse
								? `Scanning — quotes ${pulse.ticker_ready ?? 0}/${pulse.symbols_total ?? "?"} · fluid warming ${pulse.fluid_warming ?? 0}`
								: "Waiting for first engine pulse…"
							: undefined
					}
				/>
			)}
		</SidebarSection>
	);
};

const EnginePulseStrip = ({
	pulse,
	connected,
}: {
	pulse: NonNullable<ReturnType<typeof useSymmEnginePulse>>;
	connected: boolean;
}) => {
	if (!connected) {
		return null;
	}

	return (
		<div className="border-b border-(--dash-border) px-3 py-1.5 text-[10px] tabular-nums text-(--dash-muted)">
			<span className="font-medium text-(--dash-text)">#{pulse.seq}</span>{" "}
			{pulse.phase} · meas {pulse.measurements} · cand {pulse.candidates} ·
			open {pulse.open}
			{pulse.fluid_sampled !== undefined ? (
				<>
					{" "}
					· fluid {pulse.fluid_sampled}
					{(pulse.fluid_warming ?? 0) > 0 ? `+${pulse.fluid_warming} warm` : ""}
				</>
			) : null}
		</div>
	);
};

interface Props {
	decisions: DecisionTraceEvent["decisions"];
}

export const DecisionTable = ({ decisions }: Props) => {
	return (
		<div className="overflow-auto">
			<table className="w-full text-left text-xs">
				<thead className="sticky top-0 bg-(--dash-panel) text-(--dash-muted)">
					<tr>
						<th className="px-3 py-1.5 font-medium">Symbol</th>
						<th className="px-2 py-1.5 text-right font-medium">Score</th>
						<th className="px-2 py-1.5 font-medium">Verdict</th>
						<th className="hidden px-2 py-1.5 font-medium lg:table-cell">
							Why
						</th>
					</tr>
				</thead>
				<tbody>
					{decisions.map((row) => (
						<tr
							key={`${row.symbol}:${row.source ?? ""}:${row.score}`}
							className="border-t border-(--dash-border)"
						>
							<td className="px-3 py-1.5 font-medium">{row.symbol}</td>
							<td className="px-2 py-1.5 text-right tabular-nums">
								{row.score.toFixed(3)}
							</td>
							<td className="px-2 py-1.5">
								<VerdictBadge allow={row.allow} inPlay={row.in_play} />
							</td>
							<td className="hidden max-w-36 truncate px-2 py-1.5 text-(--dash-muted) lg:table-cell">
								{whyLabel(row.why)}
							</td>
						</tr>
					))}
				</tbody>
			</table>
		</div>
	);
};

type ScoreboardTarget = {
	symbol: string;
	regime: string;
	reason: string;
	score: number;
	source?: string;
};

const ScoreboardTable = ({ targets }: { targets: ScoreboardTarget[] }) => (
	<div className="overflow-auto">
		<table className="w-full text-left text-xs">
			<thead className="sticky top-0 bg-(--dash-panel) text-(--dash-muted)">
				<tr>
					<th className="px-3 py-1.5 font-medium">Symbol</th>
					<th className="px-2 py-1.5 text-right font-medium">Score</th>
					<th className="px-2 py-1.5 font-medium">Regime</th>
				</tr>
			</thead>
			<tbody>
				{targets.map((row) => (
					<tr key={row.symbol} className="border-t border-(--dash-border)">
						<td className="px-3 py-1.5 font-medium">{row.symbol}</td>
						<td className="px-2 py-1.5 text-right tabular-nums">
							{row.score.toFixed(3)}
						</td>
						<td className="px-2 py-1.5 text-(--dash-muted)">{row.regime}</td>
					</tr>
				))}
			</tbody>
		</table>
	</div>
);

const SignalTable = ({
	signals,
}: {
	signals: Array<{
		symbol: string;
		source: string;
		score: number;
		regime: string;
		reason: string;
	}>;
}) => (
	<div className="overflow-auto">
		<table className="w-full text-left text-xs">
			<thead className="sticky top-0 bg-(--dash-panel) text-(--dash-muted)">
				<tr>
					<th className="px-3 py-1.5 font-medium">Symbol</th>
					<th className="px-2 py-1.5 font-medium">Source</th>
					<th className="px-2 py-1.5 text-right font-medium">Score</th>
				</tr>
			</thead>
			<tbody>
				{signals.map((row) => (
					<tr
						key={`${row.symbol}:${row.source}`}
						className="border-t border-(--dash-border)"
					>
						<td className="px-3 py-1.5 font-medium">{row.symbol}</td>
						<td className="px-2 py-1.5 text-(--dash-muted)">{row.source}</td>
						<td className="px-2 py-1.5 text-right tabular-nums">
							{row.score.toFixed(3)}
						</td>
					</tr>
				))}
			</tbody>
		</table>
	</div>
);
