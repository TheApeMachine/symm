import { afterEach, describe, expect, it, vi } from "vitest";
import { runWarehouseStatement } from "./workbench-api";

afterEach(() => vi.unstubAllGlobals());

const respond = (status: number, body: BodyInit | null) => {
	vi.stubGlobal("window", {
		location: { protocol: "http:", hostname: "localhost" },
	});

	const fetcher = vi.fn().mockResolvedValue(new Response(body, { status }));
	vi.stubGlobal("fetch", fetcher);

	return fetcher;
};

describe("Warehouse statements", () => {
	it("posts the statement to the hub and returns the Arrow stream verbatim", async () => {
		const stream = new Uint8Array([255, 255, 255, 255, 0, 0, 0, 0]);
		const fetcher = respond(200, stream);

		expect(await runWarehouseStatement("SELECT 1")).toEqual(stream);
		expect(fetcher).toHaveBeenCalledWith(
			"http://127.0.0.1:8765/workbench/query",
			expect.objectContaining({
				method: "POST",
				body: JSON.stringify({ sql: "SELECT 1" }),
			}),
		);
	});

	/*
		The viewer materializes and drops its views through the same call it
		reads data with. Those statements answer with no rows, which must reach
		the handler as an empty result rather than as a failure.
	*/
	it("returns an empty stream for a statement that produces no rows", async () => {
		respond(200, null);

		expect((await runWarehouseStatement("DROP TABLE v1")).byteLength).toBe(0);
	});

	it("raises the engine's own message when a statement is rejected", async () => {
		respond(400, 'workbench: evaluate query: Table with name "nowhere" ...');

		await expect(
			runWarehouseStatement("SELECT * FROM nowhere"),
		).rejects.toThrow("nowhere");
	});
});
