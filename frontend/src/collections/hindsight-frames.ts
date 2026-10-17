import {
	EXCURSION_KEYS,
	HINDSIGHT_DONE_KEYS,
	HINDSIGHT_SYMBOL_KEYS,
	HINDSIGHT_TICK_KEYS,
	type HindsightFrame,
} from "#/collections/hindsight";

/*
Test fixtures: hindsight frames as the hub serializes them (hindsight/service.go).
*/

const vector = (values: number[]) => {
	const bytes = new Uint8Array(values.length * 8);
	const view = new DataView(bytes.buffer);
	values.forEach((value, idx) => {
		view.setFloat64(idx * 8, value, true);
	});
	return bytes;
};

const key = (role: string, symbol: string, run: string, tick: number) =>
	`hindsight/${role}/${symbol}/${run.padStart(19, "0")}/${String(tick).padStart(19, "0")}.bin`;

export const tickFrame = (
	run: string,
	symbol: string,
	tick: number,
	bid: number,
	ask: number,
	trade: number,
): HindsightFrame => ({
	epoch: BigInt(run),
	tick: BigInt(tick),
	timestamp: BigInt(tick) * 1_000_000_000n,
	role: "tick",
	scope: symbol,
	layers: [
		{
			index: `${key("tick", symbol, run, tick)}/${HINDSIGHT_TICK_KEYS.join("/")}`,
			bytes: vector([bid, ask, trade, 0.25, 1]),
		},
	],
});

export const excursionFrame = (
	run: string,
	symbol: string,
	kind: string,
	fields: Partial<Record<(typeof EXCURSION_KEYS)[number], number>>,
): HindsightFrame => ({
	epoch: BigInt(run),
	tick: 0n,
	timestamp: 0n,
	role: "detector",
	scope: symbol,
	layers: [
		{
			index: `detector/${kind}/${symbol}/${run}/0000000000000000002.bin/${EXCURSION_KEYS.join("/")}`,
			bytes: vector(EXCURSION_KEYS.map((name) => fields[name] ?? 0)),
		},
	],
});

export const policyFrame = (
	run: string,
	symbol: string,
	action: string,
	path: string,
	gain: number,
): HindsightFrame => ({
	epoch: BigInt(run),
	tick: 0n,
	timestamp: 0n,
	role: "policy",
	scope: symbol,
	layers: [
		{
			index: `policy/${action}/all/${run}/0000000000000000001.bin/gain/hold_seconds/visits/mean_reward/confidence/path:${path}`,
			bytes: vector([gain, 30, 4, gain, 0.6]),
		},
	],
});

export const symbolFrame = (
	run: string,
	symbol: string,
	fields: Partial<Record<(typeof HINDSIGHT_SYMBOL_KEYS)[number], number>>,
): HindsightFrame => ({
	epoch: BigInt(run),
	tick: 0n,
	timestamp: 0n,
	role: "symbol",
	scope: "",
	layers: [
		{
			index: `${key("symbol", symbol, run, 0)}/${HINDSIGHT_SYMBOL_KEYS.join("/")}`,
			bytes: vector(HINDSIGHT_SYMBOL_KEYS.map((name) => fields[name] ?? 0)),
		},
	],
});

export const doneFrame = (
	run: string,
	symbol: string,
	ticks: number,
): HindsightFrame => ({
	epoch: BigInt(run),
	tick: 0n,
	timestamp: 0n,
	role: "done",
	scope: symbol,
	layers: [
		{
			index: `${key("done", symbol, run, 0)}/${HINDSIGHT_DONE_KEYS.join("/")}`,
			bytes: vector([ticks, 1, 0, 0, 0]),
		},
	],
});

export const errorFrame = (
	run: string,
	symbol: string,
	message: string,
): HindsightFrame => ({
	epoch: BigInt(run),
	tick: 0n,
	timestamp: 0n,
	role: "error",
	scope: symbol,
	layers: [{ index: "error", bytes: new TextEncoder().encode(message) }],
});
