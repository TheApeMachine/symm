import "@perspective-dev/viewer/dist/css/themes.css";

import {
	lazy,
	Suspense,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Tabs } from "#/components/ui/tabs";
import { Typography } from "#/components/ui/typography";
import { Agent } from "#/components/workbench/agent";
import {
	type WarehouseClient,
	warehouseClient,
} from "#/components/workbench/perspective";

/*
The viewer is loaded on demand for the same reason its engine is: the module
reaches for Custom Elements at import time, and the route tree is evaluated
during server rendering.
*/
const PerspectiveViewer = lazy(() =>
	import("@perspective-dev/react").then((module) => ({
		default: module.PerspectiveViewer,
	})),
);

/*
ViewerWithAgent mounts the Perspective surface and opts its element into the
LM Studio agent once the custom element is in the DOM. The React wrapper does
not forward a ref to that element.
*/
const ViewerWithAgent = ({
	client,
	table,
}: {
	client: WarehouseClient;
	table: string;
}) => {
	const host = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const viewer = host.current?.querySelector("perspective-viewer") as
			| { agentConfig: (config: unknown) => void }
			| null
			| undefined;

		if (!viewer) {
			return;
		}

		void Agent.configure(viewer);
	}, []);

	return (
		<div ref={host} className="absolute inset-0 h-full w-full">
			<PerspectiveViewer
				client={client}
				config={{ table, theme: "Pro Dark" }}
				className="h-full w-full"
			/>
		</div>
	);
};

/*
opening picks the table the surface starts on.

Every table in the warehouse is offered, and the viewer's own UI switches
between them, but one of them has to be showing when the page opens. Closed
lots are the usual analytical start; Level3 events are next when lots are
absent.
*/
const opening = (tables: string[]): string =>
	tables.find((table) => table.includes("closed_")) ??
	tables.find((table) => table.includes("level3_events")) ??
	tables[0] ??
	"";

const shortName = (name: string): string => {
	const parts = name.split(".");
	return parts[parts.length - 1] ?? name;
};

/*
Workbench is the analytical surface.

The viewer is not given a table of its own. It is given a Client backed by a
Perspective Virtual Server over DuckDB (hub proxy → workbench service), which
means the pivots, filters, sorts, and expressions configured in its UI are
compiled into SQL and evaluated against S3 JSON views (open/closed lots and
Level3 NDJSON), and only the window on screen is transferred.
*/
export const Workbench = () => {
	const [client, setClient] = useState<WarehouseClient | undefined>();
	const [tables, setTables] = useState<string[]>([]);
	const [table, setTable] = useState("");
	const [error, setError] = useState("");
	const [connecting, setConnecting] = useState(true);

	const connect = useCallback(() => {
		let live = true;
		setConnecting(true);
		setError("");

		warehouseClient()
			.then(async (connected) => {
				const fetched = await connected.get_hosted_table_names();

				if (!live) {
					return;
				}

				setClient(connected);
				setTables(fetched);
				setTable(opening(fetched));
				setError("");
				setConnecting(false);
			})
			.catch((cause: Error) => {
				if (!live) {
					return;
				}

				setError(cause.message);
				setConnecting(false);
			});

		return () => {
			live = false;
		};
	}, []);

	useEffect(() => {
		return connect();
	}, [connect]);

	if (error && !client) {
		return (
			<Flex.Column className="h-full min-h-0 items-center justify-center gap-3 p-6">
				<Typography.Label size="m" tone="f3">
					warehouse unavailable
				</Typography.Label>
				<Typography.Mono size="s" tone="f4" className="max-w-2xl text-center">
					{error}
				</Typography.Mono>
				<Button variant="outline" size="s" onClick={() => connect()}>
					retry connection
				</Button>
			</Flex.Column>
		);
	}

	if (connecting && !client) {
		return (
			<Flex.Column className="h-full min-h-0 items-center justify-center gap-2 p-6">
				<Typography.Label size="m" tone="f4">
					connecting to analytical warehouse...
				</Typography.Label>
			</Flex.Column>
		);
	}

	if (client && !table) {
		return (
			<Flex.Column
				align="center"
				justify="center"
				gap={2}
				padding={6}
				fullWidth
				fullHeight
			>
				<Typography.Label size="m" tone="f3">
					no tables in warehouse
				</Typography.Label>
				<Typography.Mono size="s" tone="f4">
					waiting for lot or level3 views to appear in the warehouse
				</Typography.Mono>
			</Flex.Column>
		);
	}

	if (!client || !table) {
		return <div className="h-full min-h-0" />;
	}

	return (
		<Flex.Column className="h-full min-h-0 w-full">
			<div className="flex shrink-0 items-center justify-between border-b border-(--line) bg-(--bg) px-3 py-1.5">
				<Flex.Row align="center" gap={2}>
					<Typography.Label
						size="xs"
						tone="f4"
						className="uppercase tracking-wider"
					>
						Table
					</Typography.Label>
					<Tabs size="xs">
						{tables.map((tableName) => {
							const active = tableName === table;
							return (
								<Tabs.Tab
									key={tableName}
									active={active}
									onClick={() => setTable(tableName)}
									title={tableName}
								>
									{shortName(tableName)}
								</Tabs.Tab>
							);
						})}
					</Tabs>
				</Flex.Row>
				<Typography.Mono size="s" tone="f4">
					{table}
				</Typography.Mono>
			</div>

			<div className="relative min-h-0 flex-1">
				<Suspense fallback={null}>
					<ViewerWithAgent key={table} client={client} table={table} />
				</Suspense>
			</div>
		</Flex.Column>
	);
};
