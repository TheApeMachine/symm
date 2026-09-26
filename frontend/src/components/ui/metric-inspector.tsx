import { useMemo, useState } from "react";
import { Badge } from "./badge";
import { Button } from "./button";
import { Flex } from "./flex";
import { Input } from "./input";
import { Typography } from "./typography";

export interface MetricEvidence {
	identity: string;
	value: number;
	present: boolean;
	epoch: string | number;
	sequence: string | number;
}

export interface MetricCutRecord {
	typeId: string;
	value: {
		epoch: string | number;
		sequence: string | number;
		symbol: string;
		complete: boolean;
		metrics: MetricEvidence[];
		provenance: string;
	};
}

export interface MetricInspectorProps {
	row?: MetricCutRecord;
	title?: string;
	className?: string;
}

/* MetricInspector renders the native cut, retaining each coordinate's own causal stamp. */
export const MetricInspector = ({
	row,
	title = "Metric evidence",
	className,
}: MetricInspectorProps) => {
	const [query, setQuery] = useState("");
	const [missingOnly, setMissingOnly] = useState(false);
	const cut = row?.value;
	const metrics = cut?.metrics ?? [];
	const visible = useMemo(
		() =>
			metrics.filter(
				(metric) =>
					(!missingOnly || !metric.present) &&
					metric.identity.toLowerCase().includes(query.toLowerCase()),
			),
		[metrics, missingOnly, query],
	);
	const initialized = metrics.filter((metric) => metric.present).length;

	return (
		<Flex.Column
			className={`h-full min-h-0 overflow-hidden bg-(--surface) ${className ?? ""}`}
		>
			<Flex.Row className="shrink-0 items-center gap-4 border-b border-(--line) p-3">
				<Typography.Label>{title}</Typography.Label>
				<Badge
					label={cut?.symbol ?? "Waiting for a market cut"}
					variant="info"
				/>
				<Badge
					label={
						cut
							? `${initialized}/${metrics.length} initialized`
							: "No cut received"
					}
					variant={cut?.complete ? "success" : "warning"}
				/>
				<Typography.Mono>
					epoch {cut?.epoch ?? "—"} · sequence {cut?.sequence ?? "—"}
				</Typography.Mono>
			</Flex.Row>
			<Flex.Row className="shrink-0 items-center gap-3 border-b border-(--line) px-3 py-2">
				<Input
					aria-label="Find metric"
					placeholder="Find metric or signal family"
					value={query}
					onChange={(event) => setQuery(event.target.value)}
				/>
				<Button
					variant={missingOnly ? "solid" : "outline"}
					onClick={() => setMissingOnly(!missingOnly)}
				>
					Missing inputs
				</Button>
				<Typography.Mono>{visible.length} coordinates</Typography.Mono>
			</Flex.Row>
			<div className="min-h-0 flex-1 overflow-auto">
				<table className="w-full text-left font-mono text-xs">
					<thead className="sticky top-0 bg-(--sunken) text-(--f4)">
						<tr>
							{[
								"Metric",
								"Value",
								"State",
								"Epoch",
								"Last update sequence",
							].map((label) => (
								<th key={label} className="px-3 py-2">
									{label}
								</th>
							))}
						</tr>
					</thead>
					<tbody>
						{visible.map((metric, index) => (
							<tr
								key={`${metric.identity}:${index}`}
								className="border-t border-(--line)"
							>
								<td className="px-3 py-2 text-(--f2)">{metric.identity}</td>
								<td className="px-3 py-2 text-(--f1)">
									{metric.present ? String(metric.value) : "—"}
								</td>
								<td className="px-3 py-2">
									{metric.present ? "initialized" : "undefined"}
								</td>
								<td className="px-3 py-2">
									{metric.present ? metric.epoch : "—"}
								</td>
								<td className="px-3 py-2">
									{metric.present ? metric.sequence : "—"}
								</td>
							</tr>
						))}
					</tbody>
				</table>
			</div>
		</Flex.Column>
	);
};
