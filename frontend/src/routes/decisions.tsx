import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { decisionsStore } from "#/collections/decisions";

const verdictColor = (verdict: unknown): string =>
	verdict === "allow" ? "var(--up)" : "var(--f3)";

const RouteComponent = () => {
	const frame = useSelector(decisionsStore, (state) => state.frame);
	const decisions =
		(frame?.decisions as Record<string, unknown>[] | undefined) ?? [];

	return (
		<div className="flex h-full min-w-[1080px] flex-col">
			<div className="flex shrink-0 items-center gap-[22px] border-(--line) border-b bg-(--surface) px-[18px] py-3">
				<div>
					<div className="font-serif font-semibold text-[18px] text-(--f1) leading-[1.1]">
						Playbook decisions
					</div>
					<div className="mt-[3px] font-mono text-[10px] text-(--f4)">
						candidate actions · admitted vs blocked · the single decision point
					</div>
				</div>
				<span className="ml-auto font-mono text-[11px] text-(--f3)">
					{decisions.length} candidates
				</span>
			</div>

			{decisions.length === 0 ? (
				<div className="flex flex-1 items-center justify-center font-mono text-[11px] text-(--f4)">
					waiting for playbook decisions
				</div>
			) : (
				<div className="min-h-0 flex-1 overflow-auto p-3.5">
					<table className="w-full font-mono text-[11px]">
						<thead>
							<tr className="text-left text-[9px] text-(--f4) uppercase tracking-widest">
								<th className="py-2 pr-3">Symbol</th>
								<th className="py-2 pr-3">Side</th>
								<th className="py-2 pr-3">Type</th>
								<th className="py-2 pr-3 text-right">Price</th>
								<th className="py-2 pr-3 text-right">Qty</th>
								<th className="py-2 pr-3 text-right">Conf</th>
								<th className="py-2 pr-3">Verdict</th>
								<th className="py-2">Why</th>
							</tr>
						</thead>
						<tbody>
							{decisions.map((d, index) => (
								<tr
									key={`${String(d.symbol)}-${index}`}
									className="border-(--line) border-t"
								>
									<td className="py-2 pr-3 text-(--f1)">{String(d.symbol)}</td>
									<td className="py-2 pr-3 text-(--f2)">{String(d.side)}</td>
									<td className="py-2 pr-3 text-(--f3)">{String(d.type)}</td>
									<td className="py-2 pr-3 text-right text-(--f2)">
										{String(d.price)}
									</td>
									<td className="py-2 pr-3 text-right text-(--f2)">
										{String(d.quantity)}
									</td>
									<td className="py-2 pr-3 text-right text-(--f2)">
										{String(d.confidence)}
									</td>
									<td
										className="py-2 pr-3"
										style={{ color: verdictColor(d.verdict) }}
									>
										{String(d.verdict)}
									</td>
									<td className="py-2 text-(--f4)">{String(d.why)}</td>
								</tr>
							))}
						</tbody>
					</table>
				</div>
			)}
		</div>
	);
};

export const Route = createFileRoute("/decisions")({
	component: RouteComponent,
});
