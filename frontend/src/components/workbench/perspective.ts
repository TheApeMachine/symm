import SERVER_WASM from "@perspective-dev/server/dist/wasm/perspective-server.wasm?url";
import CLIENT_WASM from "@perspective-dev/viewer/dist/wasm/perspective-viewer.wasm?url";

/*
Perspective's engine and its UI are two WebAssembly binaries that must be
initialized once, before the first viewer renders, and its plugins register
themselves as an import side effect.

Everything here is imported dynamically. The route tree is evaluated during
server rendering, and these modules reach for the DOM, Web Workers, and
WebAssembly, none of which exist there. The `?url` imports above are the
exception: they resolve to plain asset URLs, not to the binaries.
*/
let booted: Promise<WarehouseClient> | undefined;

/*
WarehouseClient is a Perspective Client whose tables are the warehouse's, not
copies of them held in the browser.
*/
export type WarehouseClient = Awaited<
	ReturnType<typeof import("@perspective-dev/client").worker>
>;

const boot = async (): Promise<WarehouseClient> => {
	const [client, viewer] = await Promise.all([
		import("@perspective-dev/client"),
		import("@perspective-dev/viewer"),
		import("@perspective-dev/viewer-datagrid"),
		import("@perspective-dev/viewer-charts"),
	]);

	await Promise.all([
		client.default.init_server(fetch(SERVER_WASM)),
		viewer.default.init_client(fetch(CLIENT_WASM)),
	]);

	/*
		The handler is imported only now: it subclasses a module that resolves
		the client wasm from the registered `<perspective-viewer>` element, so
		it cannot be constructed before the two initializations above.
	*/
	const { WarehouseHandler } = await import(
		"#/components/workbench/warehouse-handler"
	);

	const port = await client.createMessageHandler(new WarehouseHandler());

	return client.worker(Promise.resolve(port));
};

/*
warehouseClient initializes Perspective on first call and returns the same
client to every caller afterwards. A failed boot is not retained, so a reload
that failed on a cold network can be retried.
*/
export const warehouseClient = (): Promise<WarehouseClient> => {
	if (!booted) {
		booted = boot().catch((error) => {
			booted = undefined;
			throw error;
		});
	}

	return booted;
};
