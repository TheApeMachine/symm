import { describe, expect, it } from "vitest";
import {
	kernelMeasurement,
	kernelSymbolForAggregate,
	signalFor,
	signals,
	symbolsOnAggregate,
} from "#/collections/app";
import { ingest } from "#/providers/websocket";
import { parseOneValue } from "#/types/decoder";
import { Value } from "#/types/value";

describe("kernelMeasurement", () => {
	it("folds one kernel's layers off the aggregate tick for a symbol", () => {
		const aggregate = new Value({
			scope: "aggregate",
			prefix: "workspace/tick/aggregate/0/100",
		});

		const layer = new TextEncoder().encode(JSON.stringify([0.42]));
		aggregate.layers.push(layer);
		aggregate.index.push(
			`${aggregate.prefix}/cvd/BTC/USD/signed_net_fraction`,
		);

		const reading = kernelMeasurement("cvd", "BTC/USD", aggregate);

		expect(reading?.scope).toBe("BTC/USD");
		expect(reading?.metric("signed_net_fraction")).toBe(0.42);
	});

	it("folds one multi-metric signal layer into named metrics", () => {
		const aggregate = new Value({
			scope: "aggregate",
			prefix: "workspace/tick/aggregate/0/2",
		});
		const payload = [0.1, 0.2, 0.3, 1.5, 0.01, 0.8, 2.0];
		aggregate.layers.push(new TextEncoder().encode(JSON.stringify(payload)));
		aggregate.index.push(
			`${aggregate.prefix}/hawkes/BTC/USD/arrival_rate:buy/arrival_rate:sell/branching_spectral_radius/conditional_intensity/conditional_intensity_velocity/background_rate/excitation_decay`,
		);

		const reading = kernelMeasurement("hawkes", "BTC/USD", aggregate);

		expect(reading?.metric("conditional_intensity")).toBe(1.5);
		expect(reading?.metric("background_rate")).toBe(0.8);
		expect(reading?.metric("excitation_decay")).toBe(2.0);
	});

	it("falls back to a symbol on the tick when focus has no kernel layers", () => {
		const prefix = "workspace/tick/aggregate/0/5";
		const aggregate = new Value({
			scope: "aggregate",
			prefix,
		});
		aggregate.layers.push(new TextEncoder().encode(JSON.stringify([0.5])));
		aggregate.index.push(`${prefix}/cvd/MEGA/USD/signed_net_fraction`);

		expect(symbolsOnAggregate(aggregate)).toEqual(["MEGA/USD"]);
		expect(kernelSymbolForAggregate("BTC/USD", aggregate)).toBe("MEGA/USD");
		expect(
			kernelMeasurement("cvd", kernelSymbolForAggregate("BTC/USD", aggregate), aggregate)?.metric(
				"signed_net_fraction",
			),
		).toBe(0.5);
	});

	it("reads production-shaped multi-metric layers (MEGA/USD on the wire)", () => {
		const prefix =
			"workspace/tick/aggregate/1793078614128996000/0000000000000000282";
		const aggregate = new Value({
			origin: "workspace",
			role: "tick",
			scope: "aggregate",
			prefix,
			tick: 282n,
		});

		aggregate.layers.push(
			new TextEncoder().encode(
				JSON.stringify([0.04528, 0.04534, 0.0013242109909511709, 0, 0]),
			),
		);
		aggregate.index.push(
			`${prefix}/liquidity/MEGA/USD/best_price:bid/best_price:ask/relative_spread/fee_rate/friction_hurdle`,
		);

		aggregate.layers.push(
			new TextEncoder().encode(
				JSON.stringify([-0.02, 0.01, 1200.5, -0.15]),
			),
		);
		aggregate.index.push(
			`${prefix}/cvd/MEGA/USD/midpoint_log_return/flow_aligned_midpoint_return/gross_notional/signed_net_fraction`,
		);

		const cvd = kernelMeasurement("cvd", "MEGA/USD", aggregate);
		expect(cvd?.metric("signed_net_fraction")).toBe(-0.15);

		const liq = kernelMeasurement("liquidity", "MEGA/USD", aggregate);
		expect(liq?.metric("relative_spread")).toBeCloseTo(0.0013242109909511709);

		const viaAlias = kernelMeasurement("cvd", "MEGA-USD", aggregate);
		expect(viaAlias?.metric("signed_net_fraction")).toBe(-0.15);

		const impulsePin = signalFor("cvd", "MEGA/USD", aggregate);
		expect(Array.isArray(impulsePin.signed_net_fraction)).toBe(true);
	});

	it("round-trips a Go-shaped workspace frame through parse + ingest", () => {
		const prefix =
			"workspace/tick/aggregate/0000000000000000000/0000000000000000100";
		const layerIndex = `${prefix}/cvd/BTC/USD/signed_net_fraction`;
		const layerBytes = new TextEncoder().encode(JSON.stringify([0.77]));

		const origin = "workspace";
		const role = "tick";
		const scope = "aggregate";
		const enc = new TextEncoder();
		const originB = enc.encode(origin);
		const roleB = enc.encode(role);
		const scopeB = enc.encode(scope);
		const prefixB = enc.encode(prefix);

		const size =
			48 +
			1 +
			originB.length +
			1 +
			roleB.length +
			1 +
			scopeB.length +
			2 +
			2 +
			prefixB.length +
			2 +
			enc.encode(layerIndex).length +
			4 +
			layerBytes.length;

		const buf = new Uint8Array(size);
		const dv = new DataView(buf.buffer);
		buf.fill(1, 0, 16);
		dv.setBigUint64(16, 100n, true);
		dv.setBigUint64(24, 1n, true);
		dv.setBigUint64(32, 1n, true);
		dv.setBigUint64(40, 0n, true);

		let off = 48;
		buf[off++] = originB.length;
		buf.set(originB, off);
		off += originB.length;
		buf[off++] = roleB.length;
		buf.set(roleB, off);
		off += roleB.length;
		buf[off++] = scopeB.length;
		buf.set(scopeB, off);
		off += scopeB.length;
		dv.setUint16(off, 1, true);
		off += 2;
		dv.setUint16(off, prefixB.length, true);
		off += 2;
		buf.set(prefixB, off);
		off += prefixB.length;

		const idxB = enc.encode(layerIndex);
		dv.setUint16(off, idxB.length, true);
		off += 2;
		buf.set(idxB, off);
		off += idxB.length;
		dv.setUint32(off, layerBytes.length, true);
		off += 4;
		buf.set(layerBytes, off);

		const parsed = parseOneValue(buf);
		expect(parsed?.scope).toBe("aggregate");
		if (!parsed) throw new Error("expected parsed value");

		signals.workspace.setState(() => ({}));
		ingest([parsed]);

		const stored = signals.workspace.state.aggregate;
		expect(stored?.index.length).toBe(1);
		expect(kernelMeasurement("cvd", "BTC/USD", stored)?.metric("signed_net_fraction")).toBe(
			0.77,
		);

		signals.workspace.setState(() => ({}));
	});
});
