import { describe, expect, it } from "vitest";
import { encodeValueWire as encodeValue } from "#/test/encode-value-wire";
import { parseWireValues } from "#/types/decoder";
import { compactS3ListPrefix, partitionWireBatch } from "./s3-prefix-compact";

const wireStub = (
	overrides: Partial<{
		origin: string;
		role: string;
		scope: string;
		layers: Uint8Array[];
		index: string[];
		prefix: string;
	}>,
) => ({
	id: "00",
	epoch: 0n,
	tick: 1n,
	seqIdx: 0n,
	timestamp: 0n,
	origin: "positions",
	role: "update",
	scope: "BTC/USD",
	prefix: "positions/update/BTC/USD/0/1",
	layers: [] as Uint8Array[],
	index: [] as string[],
	byteLength: 0,
	...overrides,
});

describe("s3-prefix-compact", () => {
	it("partitions ListPrefix frames out of the values batch", () => {
		const live = wireStub({});
		const list = wireStub({
			origin: "s3",
			role: "prefix",
			scope: "position/closed",
			prefix: "s3/prefix/position/closed/0/1",
		});

		const { values, s3 } = partitionWireBatch([live, list]);

		expect(values).toEqual([live]);
		expect(s3).toHaveLength(1);
		expect(s3[0]?.kind).toBe("closed");
		expect(s3[0]?.scope).toBe("position/closed");
	});

	it("extracts closed lots from nested stored values", () => {
		const row = {
			id: "hist-compact",
			symbol: "BTC/USD",
			opened_at: "2026-10-18T08:00:00Z",
			closed_at: "2026-10-18T09:00:00Z",
			qty: "1",
			entry_price: "1",
			mark: "1.05",
			pnl: 0.05,
			pnl_pct: 5,
		};

		const nested = encodeValue({
			origin: "position",
			role: "closed",
			scope: "BTC-USD",
			tick: 5n,
			layers: [
				{
					index:
						"position/closed/BTC-USD/0000000000000000000/0000000000000000005/position",
					json: row,
				},
			],
		});

		const listBytes = encodeValue({
			origin: "s3",
			role: "prefix",
			scope: "position/closed",
			tick: 1n,
			layers: [
				{
					index:
						"s3/prefix/position/closed/0000000000000000000/0000000000000000001/obj",
					raw: nested,
				},
			],
		});

		const wire = parseWireValues(listBytes);
		expect(wire?.length).toBe(1);
		const listWire = wire?.[0];
		expect(listWire).toBeDefined();

		const compact = listWire ? compactS3ListPrefix(listWire) : null;
		expect(compact?.kind).toBe("closed");
		if (compact?.kind !== "closed") {
			return;
		}

		expect(compact.entries).toHaveLength(1);
		expect(compact.entries[0]?.row.id).toBe("hist-compact");
		expect(compact.entries[0]?.stamp.tick).toBe(5n);
	});
});
