import { useEffect, useState } from "react";
import { hubBaseUrl } from "#/lib/hub";
import { Alert } from "./alert";
import { Button } from "./button";
import { Flex } from "./flex";
import { RecordTable } from "./record-table";
import { Typography } from "./typography";

interface CaptureRun {
	id: string;
	startedAt: string;
	firstSequence: string | number;
	lastSequence: string | number;
	cuts: string | number;
	completeCuts: string | number;
}
export interface CaptureHistoryProps {
	className?: string;
}

/* CaptureHistory selects a persisted epoch and queries its actual excursion fragments. */
export const CaptureHistory = ({ className }: CaptureHistoryProps) => {
	const [runs, setRuns] = useState<CaptureRun[]>();
	const [selected, setSelected] = useState<string>();
	const [error, setError] = useState<string>();
	const [revision, setRevision] = useState(0);
	useEffect(() => {
		const controller = new AbortController();
		setError(undefined);
		fetch(`${hubBaseUrl()}/hindsight/runs`, { signal: controller.signal })
			.then(async (response) => {
				if (!response.ok)
					throw new Error(
						`Capture history failed (${response.status}): ${await response.text()}`,
					);
				const data: unknown = await response.json();
				if (
					!Array.isArray(data) ||
					data.some((row) => !row || typeof row.id !== "string")
				)
					throw new Error("Capture history returned invalid epochs");
				if (!controller.signal.aborted) setRuns(data);
			})
			.catch((failure: unknown) => {
				if (!controller.signal.aborted) setError(String(failure));
			});
		return () => controller.abort();
	}, [revision]);
	const epoch = selected ?? runs?.[0]?.id;
	return (
		<Flex.Column className={`h-full min-h-0 ${className ?? ""}`}>
			<Flex.Row className="items-center gap-3 p-3">
				<Typography.Label>Persisted epochs</Typography.Label>
				<Button
					variant="outline"
					onClick={() => setRevision((value) => value + 1)}
				>
					Refresh epochs
				</Button>
			</Flex.Row>
			{error && <Alert variant="error">{error}</Alert>}
			<Flex.Column className="max-h-64 overflow-auto px-3">
				{runs?.map((run) => (
					<Button
						key={run.id}
						variant={run.id === epoch ? "solid" : "bare"}
						className="justify-start font-mono"
						onClick={() => setSelected(run.id)}
					>
						{run.id} · {run.startedAt} · sequences {run.firstSequence}–
						{run.lastSequence} · {run.completeCuts}/{run.cuts} complete cuts
					</Button>
				))}
			</Flex.Column>
			{runs?.length === 0 && (
				<Typography.Mono className="p-4">No persisted epochs</Typography.Mono>
			)}
			{epoch && (
				<RecordTable
					endpoint={`/hindsight/excursions?run=${encodeURIComponent(epoch)}`}
					title="Excursion fragments"
					revision={revision}
					columns={[
						"symbol",
						"anchor_sequence",
						"ignition_sequence",
						"extremum_sequence",
						"confirmation_sequence",
						"anchor",
						"ignition",
						"extremum",
						"excursion",
						"has_precursor",
						"observation_count",
					]}
				/>
			)}
		</Flex.Column>
	);
};
