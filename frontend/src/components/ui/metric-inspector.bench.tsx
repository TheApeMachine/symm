import { renderToStaticMarkup } from "react-dom/server";
import { bench, describe } from "vitest";
import cut from "../../../../manifest/cut.json";
import { MetricInspector, type MetricCutRecord } from "./metric-inspector";

const row: MetricCutRecord = {
	typeId: "9516776432569098441",
	value: {
		epoch: "1789999999999999999",
		sequence: "100",
		symbol: "BTC/USD",
		complete: true,
		provenance: "",
		metrics: cut.nodes.gather.inputData.identities.value.map(
			(identity, index) => ({
				identity,
				value: index / 100,
				present: true,
				epoch: "1789999999999999999",
				sequence: "97",
			}),
		),
	},
};

describe("MetricInspector", () => {
	bench("renders the shipping cut vocabulary with native stamps", () => {
		const markup = renderToStaticMarkup(<MetricInspector row={row} />);
		if (
			!markup.includes(row.value.metrics[row.value.metrics.length - 1].identity)
		)
			throw new Error("last metric missing");
	});
});
