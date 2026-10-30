import type { SequiturFrame } from "./sequitur";

export const sequiturFrame = (
	overrides: Partial<SequiturFrame> = {},
): SequiturFrame => ({
	symbol: "BTC/USD",
	epoch: "1792187257188073000",
	tick: 41,
	timestamp: "1792187300000000000",
	active: [
		{ key: 2, brightness: 0.8 },
		{ key: 4, brightness: 0.6 },
		{ key: 9, brightness: 0.1 },
	],
	midpoint: 0.45,
	fired: ["R02", "R04"],
	held: 3,
	gate: { surprise: 0.42, z: 2.1, warm: true, passed: true },
	blocked: false,
	path: "R02/R04",
	policy: {
		epoch: "1792187257188073000",
		paths: 12,
		trading: true,
		stored: true,
		match: "R02/R04",
		livePath: "R02/R04",
		depth: 2,
		action: "enter",
		outcome: "enter submitted",
		similarity: 1,
		exactMatch: true,
	},
	...overrides,
});
