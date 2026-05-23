import { useSymmConnected, useSymmDecisionTrace } from "#/lib/symm/use-symm-ui";
import { SidebarSection } from "./sidebar";
import { whyLabel, type DecisionTraceEvent } from "#/lib/symm/events";
import { EmptyHint } from "./hint";
import { VerdictBadge } from "#/components/verdict";

export const DecisionsPanel = () => {
	const connected = useSymmConnected();
	const decisionTrace = useSymmDecisionTrace();

	return (
		<SidebarSection title="Decisions" fill className="min-w-0">
			{decisionTrace?.decisions?.length ? (
				<DecisionTable decisions={decisionTrace.decisions} />
			) : (
				<EmptyHint
					connected={connected}
					message={
						connected
							? "Rescore every ~1s — decision trace will appear here"
							: undefined
					}
				/>
			)}
		</SidebarSection>
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
						<tr key={row.symbol} className="border-t border-(--dash-border)">
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
