// @vitest-environment jsdom
import {
	cleanup,
	fireEvent,
	render,
	screen,
	within,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { MetricInspector, type MetricCutRecord } from "./metric-inspector";

afterEach(cleanup);

const record: MetricCutRecord = {
	typeId: "9516776432569098441",
	value: {
		epoch: "1789999999999999999",
		sequence: "100",
		symbol: "BTC/USD",
		complete: false,
		provenance: "",
		metrics: [
			{
				identity: "trade:net",
				value: -2.5,
				present: true,
				epoch: "1789999999999999999",
				sequence: "97",
			},
			{
				identity: "book:imbalance",
				value: 0,
				present: false,
				epoch: "0",
				sequence: "0",
			},
		],
	},
};

describe("MetricInspector", () => {
	it("renders per-metric causal stamps and never presents an absent value as zero", () => {
		render(<MetricInspector row={record} />);
		expect(screen.getByText("1/2 initialized")).toBeDefined();
		expect(
			screen.getByText("epoch 1789999999999999999 · sequence 100"),
		).toBeDefined();
		const rows = screen.getAllByRole("row");
		expect(within(rows[1]).getByText("97")).toBeDefined();
		expect(within(rows[1]).getByText("-2.5")).toBeDefined();
		expect(within(rows[2]).getByText("undefined")).toBeDefined();
		expect(within(rows[2]).queryByText("0")).toBeNull();
	});

	it("filters missing and named inputs, then updates the same coordinate when it initializes", () => {
		const { rerender } = render(<MetricInspector row={record} />);
		fireEvent.click(screen.getByRole("button", { name: "Missing inputs" }));
		expect(screen.queryByText("trade:net")).toBeNull();
		expect(screen.getByText("book:imbalance")).toBeDefined();
		const complete = {
			...record,
			value: {
				...record.value,
				complete: true,
				sequence: "101",
				metrics: [
					record.value.metrics[0],
					{
						...record.value.metrics[1],
						present: true,
						value: 0.75,
						epoch: record.value.epoch,
						sequence: "101",
					},
				],
			},
		};
		rerender(<MetricInspector row={complete} />);
		expect(screen.queryByText("book:imbalance")).toBeNull();
		fireEvent.click(screen.getByRole("button", { name: "Missing inputs" }));
		fireEvent.change(screen.getByLabelText("Find metric"), {
			target: { value: "BOOK" },
		});
		expect(screen.getByText("0.75")).toBeDefined();
		expect(screen.queryByText("trade:net")).toBeNull();
	});
});
