import "@perspective-dev/viewer/dist/css/themes.css";

import { lazy, Suspense, useEffect, useState } from "react";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
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
opening picks the table the surface starts on.

Every table in the warehouse is offered, and the viewer's own UI switches
between them, but one of them has to be showing when the page opens. Two of the
tables are raw tape whose substance is one opaque payload per row and which run
to tens of millions of rows; the graded outcomes are the analytical heart of
the archive, so that is where the surface opens when it is there.
*/
const opening = (tables: string[]): string =>
	tables.find((table) => table.endsWith(".outcomes")) ?? tables[0] ?? "";

/*
Workbench is the analytical surface.

The viewer is not given a table of its own. It is given a Client backed by a
Perspective Virtual Server over the hub's DuckDB, which means the pivots,
filters, sorts, and expressions configured in its UI are compiled into SQL and
evaluated in the warehouse, against the Iceberg tables, and only the window on
screen is transferred. Grouping a hundred thousand graded outcomes costs a
couple of kilobytes rather than the forty megabytes the rows themselves weigh,
and the tables too large to ever load are queryable on the same terms.
*/
export const Workbench = () => {
	const [client, setClient] = useState<WarehouseClient | undefined>();
	const [table, setTable] = useState("");
	const [error, setError] = useState("");

	useEffect(() => {
		let live = true;

		warehouseClient()
			.then(async (connected) => {
				const tables = await connected.get_hosted_table_names();

				if (!live) {
					return;
				}

				setClient(connected);
				setTable(opening(tables));
			})
			.catch((cause: Error) => {
				if (live) {
					setError(cause.message);
				}
			});

		return () => {
			live = false;
		};
	}, []);

	if (error) {
		return (
			<Flex.Column className="h-full min-h-0 items-center justify-center gap-2 p-6">
				<Typography.Label size="m" tone="f3">
					warehouse unavailable
				</Typography.Label>
				<Typography.Mono size="s" tone="f4" className="max-w-2xl text-center">
					{error}
				</Typography.Mono>
			</Flex.Column>
		);
	}

	/*
		The viewer mounts only once a table is known: given a bare Client it
		creates no panel of its own, and a configuration that names no table is
		rejected rather than showing an empty frame.
	*/
	if (!client || !table) {
		return <div className="h-full min-h-0" />;
	}

	return (
		<div className="relative h-full min-h-0">
			<Suspense fallback={null}>
				<PerspectiveViewer
					client={client}
					config={{ table, theme: "Pro Dark" }}
					className="absolute inset-0 h-full w-full"
				/>
			</Suspense>
		</div>
	);
};
