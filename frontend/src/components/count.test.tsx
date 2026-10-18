// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { RingBuffer, signals } from "#/collections/app";
import { bookAtom } from "#/collections/book";
import { Value } from "#/types/value";
import { Count } from "./count";

const lot = (scope: string, open: number) => {
	const ring = new RingBuffer<Value>(4);
	ring.add(
		new Value({
			origin: "training",
			scope,
			metrics: [{ name: "position_open", raw: open }],
		}),
	);
	return ring;
};

afterEach(() => {
	cleanup();
	bookAtom.set("paper");
	signals.training.setState(() => ({}));
});

describe("Count", () => {
	it("counts the paper lots the open-positions panel is showing", () => {
		signals.training.setState(() => ({
			"BTC/USD": lot("BTC/USD", 1),
			"SENT/USD": lot("SENT/USD", 1),
			"APE/USD · R05/R02": lot("APE/USD · R05/R02", 1),
		}));

		const view = render(<Count />);

		expect(view.getByText("2")).toBeTruthy();
	});

	it("counts virtual lots when that book is selected", () => {
		bookAtom.set("virtual");
		signals.training.setState(() => ({
			"BTC/USD": lot("BTC/USD", 1),
			"APE/USD · R05/R02": lot("APE/USD · R05/R02", 1),
		}));

		const view = render(<Count />);

		expect(view.getByText("1")).toBeTruthy();
	});
});
