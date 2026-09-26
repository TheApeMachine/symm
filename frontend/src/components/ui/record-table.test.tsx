// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RecordTable } from "./record-table";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("RecordTable", () => {
	it("queries native inspection, retains exact money, filters and refreshes after a close", async () => {
		const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify([{symbol:"BTC/USD", pnl:"0.000000000000000001", epoch:"1789999999999999999"}]), {status:200}));
		vi.stubGlobal("fetch", fetcher);
		const view = render(<RecordTable endpoint="/trades" revision="0" />);
		await screen.findByText("0.000000000000000001");
		expect(screen.getByText("1789999999999999999")).toBeDefined();
		fireEvent.change(screen.getByLabelText("Filter stored records"), {target:{value:"ETH"}});
		expect(screen.queryByText("BTC/USD")).toBeNull();
		fetcher.mockResolvedValue(new Response(JSON.stringify([{symbol:"ETH/USD",pnl:"-1.00"}]), {status:200}));
		view.rerender(<RecordTable endpoint="/trades" revision="1" />);
		await screen.findByText("ETH/USD");
		expect(fetcher).toHaveBeenCalledTimes(2);
	});
	it("surfaces query failures instead of displaying fabricated empty history", async () => {
		vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("warehouse unavailable", {status:503})));
		render(<RecordTable endpoint="/trades" />);
		await waitFor(() => expect(screen.getByText(/warehouse unavailable/)).toBeDefined());
		expect(screen.queryByText("No stored records")).toBeNull();
	});
});
