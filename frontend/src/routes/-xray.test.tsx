// @vitest-environment jsdom
import { act, cleanup, render, waitFor } from "@testing-library/react";
import type { ComponentType } from "react";
import { RingBuffer } from "#/collections/ring";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { focusAtom, signals } from "#/collections/app";
import { paintXrayLatent } from "#/components/terminal/xray-latent";
import {
	clearRetainedTelemetry,
	latentPointsFromFrames,
} from "#/components/terminal/xray-view";
import { ResonanceT } from "#/providers/telemetry/telemetry/resonance";
import { Route } from "./xray";

vi.mock("#/components/terminal/xray-hierarchy", () => ({
	paintXrayHierarchy: vi.fn(),
}));
vi.mock("#/components/terminal/xray-latent", () => ({
	paintXrayLatent: vi.fn(),
}));
vi.mock("#/components/terminal/xray-panels", () => ({
	XrayFactsPanel: () => null,
	XrayHawkesPanel: () => null,
	XrayHierarchyPanel: () => null,
	XrayLatentPanel: () => null,
	XrayManifoldPanel: () => null,
}));

const publish = (symbol: string, embedding: number[]) => {
	const row = new ResonanceT();
	row.symbol = symbol;
	row.embedding = embedding;
	const ring = new RingBuffer<ResonanceT>(1);
	ring.add(row);
	signals.resonance.setState((state: any) => ({ ...state, [symbol]: ring }));
};

const points = () => {
	const frames = vi.mocked(paintXrayLatent).mock.calls.at(-1)?.[0];
	return latentPointsFromFrames(frames as Record<string, unknown>[]);
};

describe("XrayPaintBridge", () => {
	beforeEach(() => {
		clearRetainedTelemetry();
		signals.resonance.setState(() => ({}));
		focusAtom.set("BTC/USD");
		vi.clearAllMocks();
	});
	afterEach(cleanup);

	it("paints and updates the entire universe without cycling focus", async () => {
		publish("BTC/USD", [0.2, -0.4]);
		publish("ETH/USD", [-0.3, 0.8]);
		const View = Route.options.component as ComponentType;
		await Route.options.component?.preload?.();
		await act(async () => {
			render(<View />);
		});
		await waitFor(() => expect(paintXrayLatent).toHaveBeenCalled());
		expect(points().map(({ symbol }) => symbol)).toEqual([
			"BTC/USD",
			"ETH/USD",
		]);
		await act(() => publish("ETH/USD", [0.7, -0.2]));
		expect(points().find(({ symbol }) => symbol === "ETH/USD")).toMatchObject({
			x: 0.7,
			y: -0.2,
		});
		await act(() => publish("SOL/USD", [0.1, 0.9]));
		expect(points()).toHaveLength(3);
		await act(() => focusAtom.set("ETH/USD"));
		expect(points()).toHaveLength(3);
		expect(vi.mocked(paintXrayLatent).mock.calls.at(-1)?.[1]).toBe("ETH/USD");
	});
});
