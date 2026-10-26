import { useSelector } from "@tanstack/react-store";
import { useEffect, useState } from "react";
import { atoms } from "#/collections/app";
import {
	branchesFromPolicyKeys,
	cognitionStore,
} from "#/collections/cognition";
import { cn } from "#/lib/utils";
import { Badge, Flex, Section, Typography } from "../ui";

export interface ForwardLearningVizProps {
	symbol?: string;
}

export const ForwardLearningViz = ({
	symbol: propSymbol,
}: ForwardLearningVizProps = {}) => {
	const globalFocus = useSelector(atoms.focus, (state) => state);
	const focusSymbol = propSymbol || globalFocus;

	const [openPositionsCount, setOpenPositionsCount] = useState(0);

	const trieBranches = useSelector(cognitionStore, (state) => {
		if (state.tree?.branches?.length) return state.tree.branches;
		const keys =
			state.policyKeys.length > 0 ? state.policyKeys : (state.tree?.keys ?? []);
		return branchesFromPolicyKeys(keys);
	});

	useEffect(() => {
		const unsubPos = atoms.positionCount.subscribe((count) => {
			setOpenPositionsCount(count);
		});
		setOpenPositionsCount(atoms.positionCount.get());
		return () => {
			unsubPos?.unsubscribe?.();
		};
	}, []);

	const observationTotal = trieBranches.reduce(
		(acc, path) => acc + (path.observed === false ? 0 : path.visits),
		0,
	);

	return (
		<Flex.Column className="h-full min-h-0 overflow-hidden bg-(--bg)">
			<Section.Header
				size="s"
				className="border-(--line) border-b shrink-0"
				meta={
					<Badge
						variant="disabled"
						label={
							openPositionsCount > 0
								? `${openPositionsCount} open`
								: "No open positions"
						}
					/>
				}
			>
				<Flex.Row align="center" gap={2} className="min-w-0">
					<Badge data-l="policy-memory-title" label="POLICY MEMORY" />
					<Typography.Mono size="s" tone="f2">
						{focusSymbol}
					</Typography.Mono>
				</Flex.Row>
			</Section.Header>

			<Section fit="pane" className="min-h-0 flex-1">
				<Section.Header
					size="s"
					title="Radix trie memory"
					meta={`${observationTotal.toLocaleString()} observations routed`}
				/>
				<Section.Body scroll>
					<table className="w-full text-left border-collapse text-[10px]">
						<thead>
							<tr className="text-(--f4) border-(--line) border-b">
								<th className="font-normal pb-1.5 px-2">Path Signature</th>
								<th className="font-normal pb-1.5 px-2 text-right">Depth</th>
								<th className="font-normal pb-1.5 px-2 text-right">Visits</th>
								<th className="font-normal pb-1.5 px-2 text-right">
									Policy Bias
								</th>
								<th className="font-normal pb-1.5 px-2">Confidence</th>
								<th className="font-normal pb-1.5 px-2 text-right">
									Learned Policy
								</th>
							</tr>
						</thead>
						<tbody>
							{trieBranches.map((path, index) => (
								<tr
									key={path.hash}
									className="border-(--line)/50 border-b last:border-0 hover:bg-(--raised)"
								>
									<td
										className={cn(
											"py-1.5 px-2 font-mono",
											index === 0 ? "text-(--acc)" : "text-(--f2)",
										)}
									>
										{path.hash}
										{path.observed === false ? (
											<span className="ml-1 text-(--f4)">derived</span>
										) : null}
									</td>
									<td className="py-1.5 px-2 text-right">{path.depth}</td>
									<td className="py-1.5 px-2 text-right">
										{path.observed === false ? "—" : path.visits.toLocaleString()}
									</td>
									<td
										className={cn(
											"py-1.5 px-2 text-right font-bold",
											path.observed === false
												? "text-(--f4)"
												: path.meanEdge >= 0
													? "text-(--up)"
													: "text-(--down)",
										)}
									>
										{path.observed === false
											? "unavailable"
											: `${path.meanEdge >= 0 ? "+" : ""}${(path.meanEdge * 100).toFixed(1)}%`}
									</td>
									<td className="py-1.5 px-2">
										{path.observed === false ? (
											<span className="text-(--f4)">unavailable</span>
										) : (
											<div className="flex items-center gap-2">
												<div className="w-12 h-1 bg-(--sunken) rounded overflow-hidden">
													<div
														className="h-full bg-(--info)"
														style={{ width: `${path.confidence}%` }}
													/>
												</div>
												<span>{path.confidence.toFixed(1)}%</span>
											</div>
										)}
									</td>
									<td className="py-1.5 px-2 text-right">
										<Badge
											size="xxs"
											variant={path.policy === "ENTER" ? "success" : "disabled"}
											label={path.policy}
										/>
									</td>
								</tr>
							))}
							{trieBranches.length === 0 && (
								<tr>
									<td colSpan={6} className="py-8 text-center text-(--f4)">
										No cognitive radix branches registered yet. Open the
										cognitive tree tab to load explore paths from the hub.
									</td>
								</tr>
							)}
						</tbody>
					</table>
				</Section.Body>
			</Section>
		</Flex.Column>
	);
};
