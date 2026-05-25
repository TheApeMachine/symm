import {
	useSymmConnected,
	useSymmDecisionTrace,
	useSymmEnginePulse,
	useSymmEntryLine,
	useSymmEvaluations,
	useSymmScanProgress,
} from "#/lib/symm/use-symm-ui";
import { SidebarSection } from "./sidebar";
import {
	whyLabel,
	type DecisionTraceEvent,
	type EvaluationRow,
} from "#/lib/symm/events";
import { EmptyHint } from "./hint";
import { VerdictBadge } from "#/components/verdict";

export const DecisionsPanel = () => {
	const connected = useSymmConnected();
	const decisionTrace = useSymmDecisionTrace();
	const pulse = useSymmEnginePulse();
	const scan = useSymmScanProgress();
	const evaluations = useSymmEvaluations();
	const entryLine = useSymmEntryLine();

	const hasEvaluations = evaluations.length > 0;
	const quotesReady = scan.quotesReady || pulse?.ticker_ready || 0;
	const symbolsTotal = scan.symbolsTotal ?? pulse?.symbols_total;
	const fluidSampled = scan.fluidSampled || pulse?.fluid_sampled || 0;

	return (
		<SidebarSection title="Decisions" fill className="min-w-0">
			{pulse ? (
				<EnginePulseStrip
					pulse={pulse}
					connected={connected}
					quotesReady={quotesReady}
					symbolsTotal={symbolsTotal}
					fluidSampled={fluidSampled}
				/>
			) : connected ? (
				<div className="border-b border-(--dash-border) px-3 py-1.5 text-[10px] tabular-nums text-(--dash-muted)">
					Scanning — quotes {quotesReady}/{symbolsTotal ?? "?"} · fluid{" "}
					{fluidSampled}
				</div>
			) : null}
			{entryLine.line > 0 ? (
				<EntryLineStrip
					line={entryLine.line}
					median={entryLine.median}
					mad={entryLine.mad}
				/>
			) : null}
			{hasEvaluations ? (
				<EvaluationTable evaluations={evaluations} />
			) : decisionTrace?.decisions?.length ? (
				<DecisionTable decisions={decisionTrace.decisions} />
			) : pulse?.signals?.length ? (
				<SignalTable signals={pulse.signals} />
			) : (
				<EmptyHint
					connected={connected}
					message={
						connected
							? pulse
								? `Scanning — quotes ${quotesReady}/${symbolsTotal ?? "?"} · fluid ${fluidSampled}`
								: "Waiting for first engine pulse…"
							: undefined
					}
				/>
			)}
		</SidebarSection>
	);
};

const EntryLineStrip = ({
	line,
	median,
	mad,
}: {
	line: number;
	median: number;
	mad: number;
}) => (
	<div className="border-b border-(--dash-border) px-3 py-1.5 text-[10px] tabular-nums text-(--dash-muted)">
		line {line.toFixed(3)} · median {median.toFixed(3)} · mad {mad.toFixed(3)}
	</div>
);

const EnginePulseStrip = ({
	pulse,
	connected,
	quotesReady,
	symbolsTotal,
	fluidSampled,
}: {
	pulse: NonNullable<ReturnType<typeof useSymmEnginePulse>>;
	connected: boolean;
	quotesReady: number;
	symbolsTotal?: number;
	fluidSampled: number;
}) => {
	if (!connected) {
		return null;
	}

	return (
		<div className="border-b border-(--dash-border) px-3 py-1.5 text-[10px] tabular-nums text-(--dash-muted)">
			<span className="font-medium text-(--dash-text)">#{pulse.seq}</span>{" "}
			{pulse.phase} · meas {pulse.measurements} · cand {pulse.candidates} · open{" "}
			{pulse.open}
				<>
					{" "}
					· quotes {quotesReady}/{symbolsTotal ?? "?"} · fluid {fluidSampled}
					{(pulse.fluid_warming ?? 0) > 0 ? `+${pulse.fluid_warming} warm` : ""}
				</>
				{forecastRejectSummary(pulse.forecast_rejects)}
		</div>
	);
};

const forecastRejectSummary = (
	rejections: Record<string, number> | undefined,
) => {
	const entries = Object.entries(rejections ?? {}).sort(
		(left, right) => right[1] - left[1],
	);

	if (entries.length === 0) {
		return "";
	}

	const [key, count] = entries[0];
	const [source, reason] = key.split(":");

	return ` · reject ${source} ${whyLabel(reason)} ×${count}`;
};

const formatPct = (value: number | undefined) => {
	if (typeof value !== "number" || !Number.isFinite(value)) {
		return "—";
	}

	return `${(value * 100).toFixed(2)}%`;
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

const EvaluationTable = ({ evaluations }: { evaluations: EvaluationRow[] }) => (
	<div className="overflow-auto">
		<table className="w-full text-left text-xs">
			<thead className="sticky top-0 bg-(--dash-panel) text-(--dash-muted)">
				<tr>
					<th className="px-3 py-1.5 font-medium">Symbol</th>
					<th className="px-2 py-1.5 text-right font-medium">Combined</th>
					<th className="hidden px-2 py-1.5 text-right font-medium xl:table-cell">
						Edge
					</th>
					<th className="hidden px-2 py-1.5 font-medium md:table-cell">
						Signals
					</th>
					<th className="px-2 py-1.5 font-medium">Verdict</th>
					<th className="hidden px-2 py-1.5 font-medium lg:table-cell">Why</th>
				</tr>
			</thead>
			<tbody>
				{evaluations.map((row) => (
					<tr key={row.symbol} className="border-t border-(--dash-border)">
						<td className="px-3 py-1.5 font-medium">{row.symbol}</td>
						<td className="px-2 py-1.5 text-right tabular-nums">
							{row.combined.toFixed(3)}
							{row.support > 1 ? (
								<span className="ml-1 text-(--dash-muted)">×{row.support}</span>
							) : null}
						</td>
						<td className="hidden px-2 py-1.5 text-right tabular-nums text-(--dash-muted) xl:table-cell">
							{formatPct(row.expected_return)}
							<span className="ml-1">/ {formatPct(row.required_edge)}</span>
						</td>
						<td className="hidden max-w-48 truncate px-2 py-1.5 text-(--dash-muted) md:table-cell">
							{(row.signals ?? [])
								.map(
									(reading) =>
										`${reading.source} ${reading.confidence.toFixed(2)}`,
								)
								.join(" · ")}
						</td>
						<td className="px-2 py-1.5">
							<VerdictBadge allow={row.allow} inPlay={true} />
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
