import { Flex } from "./flex";
import { Typography } from "./typography";

export interface AccountPosition {
	symbol: string;
	quantity: string;
	basis: string;
	spent: string;
	proceeds: string;
	mark: string;
	pending: string;
	amount: string;
	reserved: string;
	fee: string;
	epoch: string | number;
	sequence: string | number;
}

export interface AccountPositionsProps {
	positions?: AccountPosition[];
	className?: string;
}

/* AccountPositions renders authoritative decimal strings without recalculating inventory or money. */
export const AccountPositions = ({
	positions,
	className,
}: AccountPositionsProps) => (
	<Flex.Column
		className={`min-h-0 overflow-auto bg-(--surface) p-3 ${className ?? ""}`}
	>
		<Typography.Label>Positions and pending orders</Typography.Label>
		{positions?.length === 0 && (
			<Typography.Mono className="p-3">
				No open positions or pending orders
			</Typography.Mono>
		)}
		{!positions && (
			<Typography.Mono className="p-3">Awaiting account state</Typography.Mono>
		)}
		<table className="mt-3 w-full text-left font-mono text-xs">
			<thead className="bg-(--sunken) text-(--f4)">
				<tr>
					{[
						"Market",
						"Quantity",
						"Basis · USD",
						"Executable mark",
						"Pending",
						"Reserved · USD",
						"Last reconciliation",
					].map((label) => (
						<th key={label} className="px-2 py-2">
							{label}
						</th>
					))}
				</tr>
			</thead>
			<tbody>
				{positions?.map((position) => (
					<tr key={position.symbol} className="border-b border-(--line)">
						<td className="px-2 py-2 text-(--f1)">{position.symbol}</td>
						<td className="px-2 py-2">{position.quantity}</td>
						<td className="px-2 py-2">{position.basis}</td>
						<td className="px-2 py-2">{position.mark || "—"}</td>
						<td className="px-2 py-2">{position.pending || "—"}</td>
						<td className="px-2 py-2">{position.reserved}</td>
						<td className="px-2 py-2">
							{position.epoch}/{position.sequence}
						</td>
					</tr>
				))}
			</tbody>
		</table>
	</Flex.Column>
);
