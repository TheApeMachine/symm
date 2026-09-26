import { useEffect, useState } from "react";
import { hubBaseUrl } from "#/lib/hub";
import { Alert } from "./alert";
import { Button } from "./button";
import { Flex } from "./flex";
import { Input } from "./input";
import { Typography } from "./typography";

export interface RecordTableProps {
	endpoint: string;
	title?: string;
	columns?: string[];
	revision?: number | string;
	className?: string;
}

/* RecordTable reads immutable rows from the graph's inspection query node. */
export const RecordTable = ({
	endpoint,
	title,
	columns,
	revision,
	className,
}: RecordTableProps) => {
	const [rows, setRows] = useState<Record<string, unknown>[] | undefined>();
	const [error, setError] = useState<string>();
	const [refresh, setRefresh] = useState(0);
	const [query, setQuery] = useState("");
	const [loading, setLoading] = useState(false);
	useEffect(() => {
		const controller = new AbortController();
		setLoading(true);
		setError(undefined);
		fetch(`${hubBaseUrl()}${endpoint}`, { signal: controller.signal })
			.then(async (response) => {
				if (!response.ok)
					throw new Error(
						`History request failed (${response.status}): ${await response.text()}`,
					);
				const value: unknown = await response.json();
				if (
					!Array.isArray(value) ||
					value.some(
						(row) => !row || typeof row !== "object" || Array.isArray(row),
					)
				)
					throw new Error("History query returned invalid rows");
				if (!controller.signal.aborted) setRows(value);
			})
			.catch((failure: unknown) => {
				if (!controller.signal.aborted) setError(String(failure));
			})
			.finally(() => {
				if (!controller.signal.aborted) setLoading(false);
			});
		return () => controller.abort();
	}, [endpoint, revision, refresh]);
	const keys =
		columns ?? Array.from(new Set(rows?.flatMap((row) => Object.keys(row))));
	const shown = rows?.filter((row) =>
		Object.values(row).some((value) =>
			String(value).toLowerCase().includes(query.toLowerCase()),
		),
	);
	return (
		<Flex.Column
			className={`min-h-0 flex-1 overflow-hidden p-3 ${className ?? ""}`}
		>
			<Flex.Row className="items-center gap-3">
				<Typography.Label>{title ?? "Stored records"}</Typography.Label>
				<Input
					aria-label="Filter stored records"
					placeholder="Filter records"
					value={query}
					onChange={(event) => setQuery(event.target.value)}
				/>
				<Button
					variant="outline"
					onClick={() => setRefresh((value) => value + 1)}
					disabled={loading}
				>
					Refresh
				</Button>
				<Typography.Mono>
					{loading ? "Loading" : `${shown?.length ?? 0} records`}
				</Typography.Mono>
			</Flex.Row>
			{error && <Alert variant="error">{error}</Alert>}
			{rows?.length === 0 && (
				<Typography.Mono className="p-4">No stored records</Typography.Mono>
			)}
			<div className="min-h-0 overflow-auto">
				<table className="mt-3 w-full text-left font-mono text-xs">
					<thead>
						<tr>
							{keys.map((key) => (
								<th
									key={key}
									className="whitespace-nowrap border-b border-(--line) px-2 py-2 text-(--f4)"
								>
									{key}
								</th>
							))}
						</tr>
					</thead>
					<tbody>
						{shown?.map((row, index) => (
							<tr
								key={`${row.epoch ?? ""}/${row.sequence ?? ""}/${row.symbol ?? ""}/${index}`}
								className="border-b border-(--line)"
							>
								{keys.map((key) => (
									<td key={key} className="whitespace-nowrap px-2 py-2">
										{row[key] === undefined || row[key] === null
											? "—"
											: typeof row[key] === "object"
												? JSON.stringify(row[key])
												: String(row[key])}
									</td>
								))}
							</tr>
						))}
					</tbody>
				</table>
			</div>
		</Flex.Column>
	);
};
