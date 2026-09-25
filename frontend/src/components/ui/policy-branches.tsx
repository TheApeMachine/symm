import type { ComponentProps } from "react";
import { cn } from "#/lib/utils";
import { Flex } from "./flex";

export interface PolicyBranch {
	id: string;
	signature: string;
	depth: number;
	visits: number | string;
	/** Measured basis points; no conversion from an unspecified return unit. */
	meanEdgeBps?: number;
	/** Measured fraction in [0,1]. Absence is distinct from zero. */
	confidence?: number;
	policy?: string;
}
export type PolicyBranchesProps = Omit<
	ComponentProps<typeof Flex.Column>,
	"children"
> & {
	title?: string;
	branches?: PolicyBranch[] | null;
};

const policyTone = (policy?: string) => {
	if (policy === "ENTER") return "border-(--up)/30 bg-(--up)/10 text-(--up)";
	if (policy === "EXIT")
		return "border-(--down)/30 bg-(--down)/10 text-(--down)";
	return "border-(--f4)/30 bg-(--f4)/10 text-(--f3)";
};

/*
PolicyBranches lists the trie's recorded paths: each path signature, how deep
it runs, how often it was taken, and the action most of those visits carried.
*/
export const PolicyBranches = ({
	title = "Supervised Precursor Examples (Active Branches)",
	branches,
	className,
	...props
}: PolicyBranchesProps) => {
	const routed = (branches ?? []).reduce(
		(sum, branch) => sum + (Number(branch.visits) || 0),
		0,
	);

	return (
		<Flex.Column
			className={cn(
				"min-h-0 min-w-0 rounded border border-(--line) bg-(--surface) font-mono text-[11px] text-(--f3)",
				className,
			)}
			{...props}
		>
			<Flex.Row className="h-8 shrink-0 items-center justify-between border-(--line) border-b bg-(--sunken) px-4">
				<span className="text-[10px] uppercase tracking-widest">{title}</span>
				<span className="text-[10px] text-(--f4)">
					{routed.toLocaleString()} observations routed
				</span>
			</Flex.Row>
			<div className="min-h-0 flex-1 overflow-auto p-2">
				{!branches?.length && (
					<div className="p-2 text-(--f4)">No recorded branches</div>
				)}
				{!!branches?.length && (
					<table className="w-full border-collapse text-left">
						<thead>
							<tr className="border-(--line) border-b text-(--f4)">
								<th className="px-2 pb-2 font-normal">Path signature</th>
								<th className="px-2 pb-2 text-right font-normal">Depth</th>
								<th className="px-2 pb-2 text-right font-normal">Visits</th>
								<th className="px-2 pb-2 text-right font-normal">Mean edge</th>
								<th className="px-2 pb-2 font-normal">Confidence</th>
								<th className="px-2 pb-2 text-right font-normal">
									Learned policy
								</th>
							</tr>
						</thead>
						<tbody>
							{branches.map((branch, index) => (
								<tr
									key={branch.id}
									className="border-(--line)/50 border-b last:border-0 hover:bg-(--raised)"
								>
									<td
										className={cn(
											"max-w-80 truncate px-2 py-1.5",
											index === 0 ? "text-(--acc)" : "text-(--f2)",
										)}
										title={branch.signature}
									>
										{branch.signature}
									</td>
									<td className="px-2 py-1.5 text-right">{branch.depth}</td>
									<td className="px-2 py-1.5 text-right">
										{typeof branch.visits === "string"
											? branch.visits
											: branch.visits.toLocaleString()}
									</td>
									<td
										className={cn(
											"px-2 py-1.5 text-right",
											branch.meanEdgeBps === undefined
												? "text-(--f4)"
												: branch.meanEdgeBps > 0
													? "text-(--up)"
													: "text-(--down)",
										)}
									>
										{branch.meanEdgeBps === undefined
											? "—"
											: `${branch.meanEdgeBps > 0 ? "+" : ""}${branch.meanEdgeBps.toFixed(2)} bp`}
									</td>
									<td className="px-2 py-1.5">
										{branch.confidence === undefined ? (
											<span className="text-(--f4)">—</span>
										) : (
											<div className="flex items-center gap-2">
												<div className="h-1 w-12 overflow-hidden rounded-full bg-(--raised)">
													<div
														className="h-full bg-(--info)"
														style={{ width: `${branch.confidence * 100}%` }}
													/>
												</div>
												<span>{(branch.confidence * 100).toFixed(1)}%</span>
											</div>
										)}
									</td>
									<td className="px-2 py-1.5 text-right">
										<span
											className={cn(
												"rounded border px-1.5 py-0.5 text-[9px]",
												policyTone(branch.policy),
											)}
										>
											{branch.policy ?? "—"}
										</span>
									</td>
								</tr>
							))}
						</tbody>
					</table>
				)}
			</div>
		</Flex.Column>
	);
};
