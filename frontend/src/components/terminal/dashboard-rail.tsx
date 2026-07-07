import { useSelector } from "@tanstack/react-store";
import { type Action, actionStore } from "#/collections/actions";
import { type Execution, executionsStore } from "#/collections/executions";
import { type Position, positionsStore } from "#/collections/positions";
import { ColumnHeader } from "#/components/dashboard/header";
import { fixed } from "#/components/terminal/decision-format";
import { cn } from "#/lib/utils";

const DecisionRows = ({ decisions }: { decisions: Action[] }) => (
	<div className="min-h-0 flex-1 overflow-auto">
		<div className="grid grid-cols-[78px_58px_minmax(84px,1fr)_58px] gap-2 border-(--line) border-b px-3 py-2 font-mono text-[9px] text-(--f4) uppercase">
			<span>Symbol</span>
			<span className="text-right">Comb</span>
			<span className="text-right">Fraction</span>
			<span className="text-right">Verdict</span>
		</div>
		{decisions.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				waiting for decision frames
			</div>
		) : null}
		{decisions.map((decision) => {
			return (
				<div
					key={decision.id}
					data-symbol={decision.symbol}
					className="grid grid-cols-[78px_58px_minmax(84px,1fr)_58px] gap-2 border-(--line) border-b px-3 py-2 font-mono text-[11px]"
				>
					<div className="min-w-0">
						<div className="truncate font-semibold text-(--f1)">
							{decision.symbol}
						</div>
						<div className="truncate text-[9px] text-(--f4)">trader</div>
					</div>
					<span className="text-right text-(--f2)">
						{fixed(Number(decision.score))}
					</span>
					<span className="truncate text-right text-(--f2)">
						{(decision.fraction * 100).toFixed(2)}%
					</span>
					<span className="text-right">
						<span
							className={cn(
								"rounded-[2px] px-1.5 py-0.5 font-semibold text-[9px] uppercase",
								decision.verdict === "allow"
									? "bg-[color-mix(in_srgb,var(--up)_16%,transparent)] text-(--up)"
									: "bg-[color-mix(in_srgb,var(--down)_16%,transparent)] text-(--down)",
							)}
						>
							{decision.verdict}
						</span>
					</span>
				</div>
			);
		})}
	</div>
);

const PositionRows = ({
	positions,
	quote,
	observed,
}: {
	positions: Position[];
	quote: string;
	observed: boolean;
}) => (
	<div className="min-h-0 flex-1 overflow-auto">
		{positions.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				{observed ? "no open positions" : "waiting for position frames"}
			</div>
		) : null}
		{positions.slice(-8).map((position) => {
			const pnlTone =
				position.pnl > 0 || position.return_pct > 0
					? "text-(--up)"
					: position.pnl < 0 || position.return_pct < 0
						? "text-(--down)"
						: "text-(--f3)";

			return (
				<div
					key={`${position.symbol}:${position.entry_price}:${position.qty}`}
					className="border-(--line) border-b px-3 py-2.5 font-mono text-[11px]"
				>
					<div className="flex items-start justify-between gap-3">
						<span className="font-semibold text-(--f1)">{position.symbol}</span>
						<span className={cn("text-right font-semibold", pnlTone)}>
							P/L {position.pnl.toFixed(4)} {quote}
						</span>
					</div>
					<div className="mt-1 flex items-center justify-between gap-3 text-[10px] text-(--f4)">
						<span>
							entry {fixed(position.entry_price)} / mark {fixed(position.mark)}
						</span>
						<span className={pnlTone}>
							{(position.return_pct * 100).toFixed(2)}%
						</span>
					</div>
				</div>
			);
		})}
	</div>
);

const AuditRows = ({ executions }: { executions: Execution[] }) => (
	<div className="min-h-0 flex-1 overflow-auto">
		{executions.length === 0 ? (
			<div className="px-4 py-5 font-mono text-[11px] text-(--f4)">
				waiting for execution frames
			</div>
		) : null}
		{executions.map((execution) => (
			<div
				key={execution.exec_id}
				className="border-(--line) border-b px-3 py-2.5 font-mono text-[11px]"
			>
				<div className="flex items-center justify-between gap-3">
					<span className="font-semibold text-(--f2)">
						{execution.order_status}
					</span>
					<span className="text-[9px] text-(--f4)">#{execution.exec_id}</span>
				</div>
				<div className="mt-1 truncate text-[10px] text-(--f4)">
					{execution.side} / {execution.symbol}
				</div>
			</div>
		))}
	</div>
);

export const DashboardRail = () => {
	const actions = useSelector(actionStore, (state) =>
		Object.values(state.actions).flatMap((history) => history.values()),
	);
	const allowed = actions.filter((action) => action.verdict === "allow");
	const denied = actions.filter((action) => action.verdict !== "allow");
	const positionsState = useSelector(positionsStore, (state) => state);
	const executions = useSelector(executionsStore, (state) =>
		state.executions.flat(),
	);
	const positions = positionsState.positions;
	const quote = positions[0]?.symbol.split("/")[1] ?? "USD";
	const net = positions.reduce((sum, position) => sum + position.pnl, 0);

	return (
		<div className="flex min-h-0 flex-col bg-(--surface)">
			<div className="flex min-h-0 flex-[1.15] flex-col border-(--line) border-b">
				<ColumnHeader
					title="Decisions"
					meta={
						<span>
							{allowed.length} allow · {denied.length} deny
						</span>
					}
				/>
				<DecisionRows decisions={actions.reverse()} />
			</div>
			<div className="flex min-h-0 flex-1 flex-col border-(--line) border-b">
				<ColumnHeader
					title="Open positions"
					meta={
						<span>
							{positions.length === 0
								? ""
								: `net ${net.toFixed(4)} ${quote} · `}
							{positions.length} open
						</span>
					}
				/>
				<PositionRows
					positions={positions}
					quote={quote}
					observed={positionsState.observed}
				/>
			</div>
			<div className="flex min-h-0 flex-1 flex-col">
				<ColumnHeader title="Audit trail" />
				<AuditRows executions={executions.reverse()} />
			</div>
		</div>
	);
};
