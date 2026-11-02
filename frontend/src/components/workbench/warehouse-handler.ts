import type { GenericSQLVirtualServerModel } from "@perspective-dev/client";
import { DuckDBHandler } from "@perspective-dev/client/dist/esm/virtual_servers/duckdb.js";
import { type Table, tableFromIPC, tableToIPC } from "apache-arrow";
import { runWarehouseStatement } from "#/components/workbench/workbench-api";

/*
This module makes Perspective's DuckDB Virtual Server talk to the DuckDB in the
hub rather than to one compiled into the browser.

Perspective ships the handler for `@duckdb/duckdb-wasm`, which loads the whole
dataset into the tab. The translation from a viewer configuration to SQL is the
valuable part of it and is identical wherever the database runs, so the handler
is kept and only its database is replaced: it reaches its connection through
two methods, and both are answered here by the hub.
*/

/*
ClientWasm is the part of Perspective's compiled client this module builds on.

The package also exports these classes at the top level, but those bindings
belong to a wasm instance nothing here initializes: reaching for one before its
module is instantiated fails inside wasm-bindgen rather than at the call. The
instance that is initialized is the one the viewer element publishes once
`init_client` has resolved, so that is the only one used.
*/
type ClientWasm = {
	GenericSQLVirtualServerModel: new (config: {
		column_separator: string;
		like_escape_clause: string;
		regex_fn: string;
		row_id_expr?: string;
	}) => GenericSQLVirtualServerModel;
};

const clientWasm = (): ClientWasm => {
	const element = customElements.get("perspective-viewer") as
		| { __wasm_module__?: ClientWasm }
		| undefined;

	const wasm = element?.__wasm_module__;

	if (!wasm) {
		throw new Error(
			"Perspective's client wasm is not initialized; the workbench must " +
				"await init_client before constructing its handler",
		);
	}

	return wasm;
};

/*
rowless is what a statement that produces no rows decodes to. The handler runs
its `CREATE`/`DROP` statements through the same call it uses for queries and
reads the result, so the empty answer has to be shaped like a result.
*/
const rowless = { toArray: () => [], schema: { fields: [] } };

/*
Perspective sometimes emits `SELECT FROM` with no projection. DuckDB rejects
that; a NULL projection is enough for those metadata probes.
*/
const sanitizeSql = (sql: string): string =>
	sql.replace(/\bSELECT\s+FROM\b/gi, "SELECT NULL FROM");

/*
warehouse is the connection the handler is given. `query` is used for the
metadata statements, whose few rows it reads as Arrow; `useUnsafe` is used for
the one statement that fetches a viewport, which the handler wants as raw IPC
bytes to hand to Perspective's own decoder — which is exactly what the hub
sends, so those bytes are never decoded on the way through.
*/
const warehouse = {
	async query(sql: string) {
		const ipc = await runWarehouseStatement(sanitizeSql(sql));

		return ipc.byteLength === 0 ? rowless : tableFromIPC(ipc);
	},

	async useUnsafe(
		callback: (
			bindings: { runQuery: (conn: null, sql: string) => Promise<Uint8Array> },
			conn: null,
		) => Promise<Uint8Array>,
	) {
		return callback(
			{ runQuery: (_conn, sql) => runWarehouseStatement(sanitizeSql(sql)) },
			null,
		);
	},
};

/*
remember resolves a name once and hands every later caller the same answer.

The in-flight promise is what is stored, not its result: the repeated questions
arrive before the first has answered, so caching the value would let them all
through.
*/
const remember = <T>(
	into: Map<string, Promise<T>>,
	name: string,
	resolve: () => Promise<T>,
): Promise<T> => {
	const known = into.get(name);

	if (known) {
		return known;
	}

	/*
		A failure is not kept. The question is asked again on the next render,
		and answering it with a stale rejection would strand the viewer on an
		error the warehouse has since recovered from.
	*/
	const pending = resolve().catch((cause) => {
		into.delete(name);

		throw cause;
	});

	into.set(name, pending);

	return pending;
};

/*
The cached shapes are the handler's own return types rather than restatements
of them, so a change upstream is a compile error here instead of a cast.
*/
type Schema = Awaited<ReturnType<DuckDBHandler["tableSchema"]>>;

const schemas = new Map<string, Promise<Schema>>();
const sizes = new Map<string, Promise<number>>();
const widths = new Map<string, Promise<number>>();

type ViewportBuffer = {
	viewId: string;
	startRow: number;
	endRow: number;
	startCol?: number;
	endCol?: number;
	table: Table;
};

const viewportBuffers = new Map<string, ViewportBuffer>();
const inFlightFetches = new Map<string, Promise<void>>();

/*
How many rows to pull around the visible window. The datagrid asks for ~one
screen at a time; without a buffer every wheel tick is another round trip.
*/
const BUFFER_ROWS = 2_000;
const OVERSCAN_ROWS = 400;

type Viewport = {
	start_row?: number;
	end_row?: number;
	start_col?: number;
	end_col?: number;
};

const covers = (
	cached: ViewportBuffer,
	startRow: number,
	endRow: number,
	viewport: Viewport,
): boolean =>
	cached.startCol === viewport.start_col &&
	cached.endCol === viewport.end_col &&
	startRow >= cached.startRow &&
	endRow <= cached.endRow;

const fillSlice = (
	cached: ViewportBuffer,
	startRow: number,
	endRow: number,
	dataSlice: { fromArrowIpc: (ipc: Uint8Array) => void },
): void => {
	const relStart = Math.min(
		cached.table.numRows,
		Math.max(0, startRow - cached.startRow),
	);
	const relEnd = Math.min(
		cached.table.numRows,
		Math.max(relStart, endRow - cached.startRow),
	);

	dataSlice.fromArrowIpc(
		tableToIPC(cached.table.slice(relStart, relEnd), "stream"),
	);
};

/*
WarehouseHandler is the shipped DuckDB handler with one method replaced.

It rebuilds hosted names as `database.schema.name` from SHOW ALL TABLES so
seeded S3 views remain addressable. Every other method interpolates the name it
was handed and needs no change.

Tables in `memory` are the handler's own workings: it materializes each view as
a real table there and drops it afterwards. They are not data anyone asked for,
so they are not offered as sources.
*/
export class WarehouseHandler extends DuckDBHandler {
	constructor() {
		const wasm = clientWasm();

		super(warehouse as never, wasm as never);

		/*
			A view with no group-by is ordered by `rowid`. S3 JSON views have no
			such column, so the ordering is declared to be no ordering, which
			DuckDB then drops from the plan. Scrolling stays consistent because
			the handler pages through a materialized copy.

			The builder is replaced rather than configured because the class
			constructs its own and takes no options.
		*/
		(
			this as unknown as { sqlBuilder: GenericSQLVirtualServerModel }
		).sqlBuilder = new wasm.GenericSQLVirtualServerModel({
			column_separator: "|",
			like_escape_clause: "\\",
			regex_fn: "regexp_matches",
			row_id_expr: "NULL",
		});
	}

	async getHostedTables(): Promise<string[]> {
		const listing = await warehouse.query("SHOW ALL TABLES");

		return listing
			.toArray()
			.map((row: { toJSON: () => Record<string, string> }) => row.toJSON())
			.filter((row: Record<string, string>) => row.database !== "memory")
			.map(
				(row: Record<string, string>) =>
					`${row.database}.${row.schema}.${row.name}`,
			);
	}

	/*
		The viewer re-reads a schema, a row count, and a column count on every
		render, resize, and configuration step. Against S3-backed views a row
		count is a scan, and a single pivot asked for one nine times over.

		None of the three can change while they are being asked for. A view is
		`CREATE TABLE`d as a snapshot and dropped whole, so its size and shape
		are fixed for as long as it exists. Each is resolved once per name and
		kept until that name is dropped — the promise, not the value, because
		the repeats arrive together and would otherwise all miss.
	*/
	async tableSchema(...args: Parameters<DuckDBHandler["tableSchema"]>) {
		return remember(schemas, args[0], () => super.tableSchema(...args));
	}

	async tableSize(...args: Parameters<DuckDBHandler["tableSize"]>) {
		return remember(sizes, args[0], async () => super.tableSize(...args));
	}

	async viewColumnSize(...args: Parameters<DuckDBHandler["viewColumnSize"]>) {
		return remember(widths, args[0], () => super.viewColumnSize(...args));
	}

	/*
		Scroll traffic is a stream of tiny viewports. Fetching exactly those
		rows makes every wheel tick a warehouse round trip; aborting the
		previous trip left the grid with nothing until scrolling stopped.

		One buffered window is kept per view. Concurrent callers share a single
		in-flight fetch and re-check the buffer when it lands, so a miss becomes
		at most one request and never returns empty mid-scroll.
	*/
	async viewGetData(...args: Parameters<DuckDBHandler["viewGetData"]>) {
		const [viewId, config, schema, viewport, dataSlice] = args;
		const startRow = viewport.start_row ?? 0;
		const endRow = viewport.end_row ?? startRow + 33;
		const rowCount = endRow - startRow;

		if (rowCount <= 0) {
			return super.viewGetData(...args);
		}

		const fromCache = (): boolean => {
			const cached = viewportBuffers.get(viewId);

			if (!cached || !covers(cached, startRow, endRow, viewport)) {
				return false;
			}

			fillSlice(cached, startRow, endRow, dataSlice);

			return true;
		};

		for (let attempt = 0; attempt < 4; attempt++) {
			if (fromCache()) {
				return;
			}

			let pending = inFlightFetches.get(viewId);

			if (!pending) {
				pending = this.fetchViewportBuffer(
					viewId,
					config,
					schema,
					viewport,
					startRow,
					rowCount,
				).finally(() => {
					if (inFlightFetches.get(viewId) === pending) {
						inFlightFetches.delete(viewId);
					}
				});

				inFlightFetches.set(viewId, pending);
			}

			await pending;
		}

		if (fromCache()) {
			return;
		}

		return super.viewGetData(...args);
	}

	private async fetchViewportBuffer(
		viewId: string,
		config: Parameters<DuckDBHandler["viewGetData"]>[1],
		schema: Parameters<DuckDBHandler["viewGetData"]>[2],
		viewport: Viewport,
		startRow: number,
		rowCount: number,
	): Promise<void> {
		const targetStart = Math.max(0, startRow - OVERSCAN_ROWS);
		const targetEnd =
			targetStart + Math.max(BUFFER_ROWS, rowCount + OVERSCAN_ROWS);

		const query = (
			this as unknown as {
				sqlBuilder: {
					viewGetData: (
						viewId: string,
						config: unknown,
						viewport: unknown,
						schema: unknown,
					) => string;
				};
			}
		).sqlBuilder.viewGetData(
			viewId,
			config,
			{
				...viewport,
				start_row: targetStart,
				end_row: targetEnd,
			},
			schema,
		);

		const ipc = await runWarehouseStatement(sanitizeSql(query));

		if (ipc.byteLength === 0) {
			return;
		}

		const table = tableFromIPC(ipc);

		viewportBuffers.set(viewId, {
			viewId,
			startRow: targetStart,
			endRow: targetStart + table.numRows,
			startCol: viewport.start_col,
			endCol: viewport.end_col,
			table,
		});
	}

	async viewDelete(viewId: string) {
		schemas.delete(viewId);
		sizes.delete(viewId);
		widths.delete(viewId);
		viewportBuffers.delete(viewId);
		inFlightFetches.delete(viewId);

		return super.viewDelete(viewId);
	}
};
