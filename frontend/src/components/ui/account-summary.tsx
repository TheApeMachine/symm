import { Badge } from "./badge";
import { Flex } from "./flex";
import { Stat } from "./stat";
import { Typography } from "./typography";

export interface AccountSummaryProps {
	cash?: string;
	pnl?: string;
	equity?: string;
	observations?: number | string;
	decisions?: number | string;
	open?: number | string;
	outcomes?: number | string;
	positives?: number | string;
	meanEdge?: number;
	edgeDefined?: boolean;
	standardError?: number;
	uncertaintyDefined?: boolean;
	phase?: string;
	className?: string;
}

/* AccountSummary displays the account node's measured paper performance and promotion state. */
export const AccountSummary = ({
	cash,
	pnl,
	equity,
	observations,
	decisions,
	open,
	outcomes,
	positives,
	meanEdge,
	edgeDefined,
	standardError,
	uncertaintyDefined,
	phase,
	className,
}: AccountSummaryProps) => (
	<Flex.Column
		className={`gap-3 border border-(--line) bg-(--surface) p-4 ${className ?? ""}`}
	>
		<Flex.Row className="items-center gap-3">
			<Typography.Label>Execution account</Typography.Label>
			<Badge label={phase ?? "Awaiting account state"} variant="info" />
		</Flex.Row>
		<div className="grid grid-cols-3 gap-3">
			<Stat label="Cash · USD" value={cash ?? "—"} />
			<Stat label="Equity · USD" value={equity ?? "—"} />
			<Stat label="Realized P&L · USD" value={pnl ?? "—"} />
			<Stat label="Market observations" value={observations ?? "—"} />
			<Stat label="Decisions" value={decisions ?? "—"} />
			<Stat label="Open positions" value={open ?? "—"} />
			<Stat label="Completed paper trades" value={outcomes ?? "—"} />
			<Stat label="Profitable paper trades" value={positives ?? "—"} />
			<Stat
				label="Measured mean edge"
				value={edgeDefined ? meanEdge : "—"}
				format="percent"
			/>
			<Stat
				label="Edge standard error"
				value={uncertaintyDefined ? standardError : "—"}
				format="percent"
			/>
		</div>
	</Flex.Column>
);
