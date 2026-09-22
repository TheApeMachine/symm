import type { ComponentProps } from "react";
import { Flex } from "./flex";
import { Typography } from "./typography";

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
	branches?: PolicyBranch[] | null;
};

export const PolicyBranches = ({ branches, ...props }: PolicyBranchesProps) => (
	<Flex.Column {...props}>
		<Typography.Label>Active branches · Radix trie memory</Typography.Label>
		{!branches?.length && (
			<Typography.Mono>No recorded branches</Typography.Mono>
		)}
		{!!branches?.length && (
			<table className="w-full text-left font-mono text-xs">
				<thead>
					<tr>
						{[
							"Path signature",
							"Depth",
							"Visits",
							"Mean edge",
							"Confidence",
							"Policy",
						].map((label) => (
							<th key={label} className="p-2 font-normal">
								{label}
							</th>
						))}
					</tr>
				</thead>
				<tbody>
					{branches.map((branch) => (
						<tr key={branch.id} className="border-b border-(--line)">
							<td className="p-2">{branch.signature}</td>
							<td>{branch.depth}</td>
							<td>{branch.visits.toLocaleString()}</td>
							<td>
								{branch.meanEdgeBps === undefined
									? "—"
									: `${branch.meanEdgeBps.toFixed(2)} bp`}
							</td>
							<td>
								{branch.confidence === undefined
									? "—"
									: `${(100 * branch.confidence).toFixed(1)}%`}
							</td>
							<td>{branch.policy ?? "—"}</td>
						</tr>
					))}
				</tbody>
			</table>
		)}
	</Flex.Column>
);
