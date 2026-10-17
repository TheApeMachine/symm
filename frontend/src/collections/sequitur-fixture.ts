import type { SequiturFrame } from "./sequitur";

/*
sequiturFrame is a SequiturFrame shaped exactly as strategy.SequiturFrame
marshals (strategy/sequitur_test.go pins the same field names): S -> M1 R05
M1 with M1 -> R02 R04, the second M1 just completed by the chord R02/R04.
*/
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
	events: [
		{
			kind: "append",
			token: "R02",
			rule: 0,
			into: 0,
			from: 0,
			digram: ["", ""],
		},
		{
			kind: "append",
			token: "R04",
			rule: 0,
			into: 0,
			from: 0,
			digram: ["", ""],
		},
		{ kind: "create", rule: 1, into: 0, from: 0, digram: ["R02", "R04"] },
	],
	completed: "M1",
	path: "R02/R04",
	policy: {
		epoch: "1792187257188073000",
		paths: 12,
		trading: true,
		stored: true,
		match: "R02/R04",
		depth: 2,
		action: "enter",
		outcome: "enter submitted",
	},
	rules: [
		{
			id: 1,
			name: "M1",
			count: 2,
			depth: 1,
			length: 2,
			symbols: ["R02", "R04"],
			terminals: ["R02", "R04"],
			action: "enter",
		},
	],
	start: ["M1", "R05", "M1"],
	rawLength: 5,
	grammarSymbols: 5,
	violations: [],
	...overrides,
});
