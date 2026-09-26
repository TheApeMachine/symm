import type { ReactNode } from "react";
import { Flex } from "./flex";
import { Typography } from "./typography";

export interface StageRowProps {
	name: string;
	group: string;
	barrier: number;
	completed?: string;
	epoch?: string;
	sequence?: string;
}

/* StageRow renders one native consumer independently, including consumers without any completed output. */
export const StageRow = ({
	name,
	group,
	barrier,
	completed,
	epoch,
	sequence,
}: StageRowProps) => (
	<div
		role="row"
		className="grid grid-cols-[5rem_12rem_minmax(15rem,1fr)_12rem_14rem_10rem] border-b border-(--line) font-mono text-xs"
	>
		{[
			barrier,
			group,
			name,
			completed ?? "—",
			epoch ?? "—",
			sequence ?? "—",
		].map((value, index) => (
			<div role="cell" key={index} className="px-3 py-2">
				{value}
			</div>
		))}
	</div>
);

export interface StageDiagnosticsProps {
	readiness?: {
		phase: string;
		ready: boolean;
		contributing: number;
		total: number;
		missing: string[];
		input_count: number;
	};
	children?: ReactNode;
	className?: string;
}

/* StageDiagnostics receives readiness from Gather; its authored child nodes receive individual native completions. */
export const StageDiagnostics = ({
	children,
	readiness,
	className,
}: StageDiagnosticsProps) => (
	<Flex.Column
		className={`h-full min-h-0 overflow-auto bg-(--surface) p-4 ${className ?? ""}`}
	>
		<Typography.Label>Native LMAX stages</Typography.Label>
		<Typography.Mono className="my-3">
			{readiness?.phase ?? "Awaiting readiness"} ·{" "}
			{readiness?.contributing ?? "—"}/{readiness?.total ?? "—"} complete signal
			families · {readiness?.input_count ?? "—"} required metrics
		</Typography.Mono>
		{readiness && !readiness.ready && (
			<Typography.Mono className="mb-3 text-(--warn)">
				Incomplete families: {readiness.missing.join(", ")}
			</Typography.Mono>
		)}
		<div role="table">
			<div
				role="row"
				className="grid grid-cols-[5rem_12rem_minmax(15rem,1fr)_12rem_14rem_10rem] bg-(--sunken) font-mono text-xs text-(--f4)"
			>
				{[
					"Barrier",
					"Group",
					"Consumer",
					"Successful observations",
					"Epoch",
					"Sequence",
				].map((label) => (
					<div role="columnheader" key={label} className="px-3 py-2">
						{label}
					</div>
				))}
			</div>
			{children}
		</div>
	</Flex.Column>
);
