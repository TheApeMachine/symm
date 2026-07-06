import { useSelector } from "@tanstack/react-store";
import type { ReactNode } from "react";
import { type Action, actionStore } from "#/collections/actions";
import { appStore } from "#/collections/app";

const dotColor = (candidate: Action): string => {
	if (candidate.verdict === "allow") {
		return "var(--acc)";
	}

	if (candidate.verdict === "in-play") {
		return "var(--info)";
	}

	return "var(--f4)";
};

const AllocationLegend = () => (
	<div className="mt-[11px] flex items-center gap-4 font-mono text-[9px] text-(--f3)">
		<span className="inline-flex items-center gap-[5px]">
			<span className="h-2 w-2 rounded-full bg-(--acc)" />
			admitted
		</span>
		<span className="inline-flex items-center gap-[5px]">
			<span className="h-2 w-2 rounded-full bg-(--info)" />
			in play
		</span>
		<span className="inline-flex items-center gap-[5px]">
			<span className="h-2 w-2 rounded-full bg-(--f4)" />
			blocked
		</span>
	</div>
);

export const AllocationMain = () => {
	const app = useSelector(appStore, (state) => state);
	const aStore = useSelector(actionStore, (state) => state);
	const actions = aStore.actions[app.focusSymbol]?.values() ?? [];
	const count = Object.values(aStore.actions).reduce(
		(sum, history) => sum + history.values().length,
		0,
	);
	const admitted = actions.filter((action) => action.verdict === "allow");
	const latest = actions.at(-1);

	if (count === 0) {
		return (
			<div className="flex min-h-0 flex-1 items-center justify-center font-mono text-[11px] text-(--f4)">
				waiting for backend decision frames
			</div>
		);
	}

	return (
		<div className="min-h-0 overflow-auto px-[18px] py-4">
			<div className="mb-3 flex items-center gap-3.5 font-mono text-[11px]">
				<span className="text-(--f3)">decision batch</span>
				<span className="text-(--f4)">
					tick <span className="text-(--f2)">{latest?.tick ?? "—"}</span>
				</span>
				<span className="text-(--f4)">
					admitted <span className="text-(--acc)">{admitted.length}</span>
				</span>
				<span className="ml-auto text-(--f4)">
					score, verdict, and fraction are backend fields
				</span>
			</div>

			<div className="flex items-center gap-[9px] border-(--line) border-b pb-[7px] font-mono text-[8.5px] text-(--f4) uppercase tracking-[0.06em]">
				<span className="w-[58px] shrink-0">symbol</span>
				<span className="flex-1">trader score</span>
				<span className="w-[66px] shrink-0 text-right">verdict</span>
				<span className="w-[58px] shrink-0 text-right">fraction</span>
				<span className="w-[100px] shrink-0 text-right">reason</span>
			</div>

			<div className="flex flex-col">
				{actions.map((action: Action) => (
					<div
						key={action.id}
						data-symbol={action.symbol}
						className="flex items-center gap-[9px] border-(--line) border-b py-[7px]"
					>
						<span
							className="w-[58px] shrink-0 cursor-pointer font-mono text-[11px] font-semibold"
							style={{ color: dotColor(action) }}
						>
							{action.symbol}
						</span>
						<div className="relative h-[18px] flex-1">
							<div className="absolute top-2 right-0 left-0 h-px bg-(--line)" />
							<div
								className="absolute top-[7px] left-0 h-[3px] rounded-sm bg-(--acc)"
								style={{
									width: `${action.score * 100}%`,
								}}
							/>
							<div
								className="absolute top-1 h-[9px] w-[9px] rounded-full border border-(--sunken)"
								style={{
									left: `${action.score * 100}%`,
									marginLeft: "-4.5px",
									background: dotColor(action),
								}}
							/>
						</div>
						<span className="w-[66px] shrink-0 text-right font-mono text-[10px] uppercase">
							{action.verdict}
						</span>
						<span className="w-[58px] shrink-0 text-right font-mono text-[10px] text-(--f2)">
							{(action.fraction * 100).toFixed(1)}%
						</span>
						<span className="w-[100px] truncate text-right font-mono text-[10px] text-(--f4)">
							{action.reason}
						</span>
					</div>
				))}
			</div>

			<AllocationLegend />
		</div>
	);
};

const Panel = ({ children }: { children: ReactNode }) => (
	<div className="rounded-[4px] border border-(--line) bg-(--sunken) p-3">
		{children}
	</div>
);

const Bar = ({ percent }: { percent: number }) => (
	<div className="h-2 overflow-hidden rounded bg-(--line)">
		<div className="h-full bg-(--acc)" style={{ width: `${percent}%` }} />
	</div>
);

export const AllocationSidePanel = () => {
	const app = useSelector(appStore, (state) => state);
	const aStore = useSelector(actionStore, (state) => state);
	const actions = (aStore.actions[app.focusSymbol]?.values() ?? [])
		.filter((candidate) => candidate.verdict === "allow");
	const deployed = actions.reduce(
		(sum, action) => sum + action.fraction * 100,
		0,
	);

	return (
		<div className="flex flex-col gap-3.5">
			<Panel>
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">
						Capital deployment
					</span>
					<span className="font-mono text-[11px] text-(--acc)">
						{Math.round(deployed)}%
					</span>
				</div>
				<div className="mt-1 mb-[11px] font-mono text-[9.5px] text-(--f4)">
					open position value from backend ledger
				</div>
				<Bar percent={deployed} />
				<div className="mt-[7px] flex justify-between font-mono text-[10px] text-(--f3)">
					<span>deployed {deployed}</span>
					<span>positions {actions.length}</span>
				</div>
			</Panel>

			<Panel>
				<div className="mb-0.5 font-semibold text-[12px] text-(--f1)">
					Admitted candidates
				</div>
				<div className="mb-[11px] font-mono text-[9.5px] text-(--f4)">
					current backend decision batch
				</div>
				<div className="flex flex-col gap-[9px]">
					{actions.map((action: Action) => (
						<div key={action.id} data-symbol={action.symbol}>
							<div className="mb-1 flex items-center justify-between">
								<span className="cursor-pointer font-mono text-[11px] font-semibold text-(--f1)">
									{action.symbol}
								</span>
								<span className="font-mono text-[10.5px] text-(--acc)">
									score {action.score}
								</span>
							</div>
							<div className="flex items-center gap-2">
								<div className="h-1.5 flex-1 overflow-hidden rounded bg-(--line)">
									<div
										className="h-full bg-(--acc) transition-[width] duration-500"
										style={{
											width: `${(action.fraction * 100).toFixed(1)}%`,
										}}
									/>
								</div>
								<span className="w-[38px] text-right font-mono text-[9.5px] text-(--f3)">
									{(action.fraction * 100).toFixed(1)}%
								</span>
							</div>
						</div>
					))}
				</div>
				<div className="mt-3 border-(--line) border-t pt-[11px] font-mono text-[9.5px] text-(--f4) leading-[1.6]">
					<div>· no notional is shown without a backend order quantity</div>
					<div>· deployed capital comes only from backend positions</div>
					<div>· every backend candidate remains visible</div>
				</div>
			</Panel>
		</div>
	);
};
