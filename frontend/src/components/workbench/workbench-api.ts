import { hubBaseUrl } from "#/lib/hub";

/*
runWarehouseStatement evaluates one statement in the hub's DuckDB and returns
its result as the bytes of an Apache Arrow IPC stream.

Every statement the workbench issues comes through here. They are written by
Perspective rather than by a person: the viewer compiles its own configuration
into SQL, so the traffic is a stream of small statements — describe this table,
materialize this view, read rows 40 through 80 of it — and some of them, the
ones that materialize or drop a view, answer with no rows at all. An empty body
is that answer, not a failure.

The bytes are handed on untouched. Decoding them into JavaScript objects here
would only mean re-encoding them for the viewer.
*/
export const runWarehouseStatement = async (
	sql: string,
): Promise<Uint8Array> => {
	const response = await fetch(`${hubBaseUrl()}/workbench/query`, {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ sql }),
	});

	if (!response.ok) {
		throw new Error(await response.text());
	}

	return new Uint8Array(await response.arrayBuffer());
};
