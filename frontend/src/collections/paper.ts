import { createStore } from "@tanstack/store";

/*
Paper-trade records the dashboard run streams as paper/<action>/<symbol>
Values (data.PaperKeys in data/schema.go): one per traded decision of a
closed round trip. The run appends the same records to its local file; this
is only the view of them.
*/

export const PAPER_ORIGIN = "paper";

export const PAPER_KEYS = [
	"depth",
	"intended_price",
	"fill_price",
	"decided_at",
	"filled_at",
	"latency_seconds",
	"slippage",
	"qty",
	"entry_price",
	"exit_price",
	"entered_at",
	"exited_at",
	"fees",
	"pnl",
	"net_return",
] as const;

export type PaperKey = (typeof PAPER_KEYS)[number];

export type PaperRecord = {
	seq: number;
	action: string;
	symbol: string;
	path: string;
	policyEpoch: string;
	fields: Record<PaperKey, number>;
};

export const PAPER_LOG = 200;

export const paperStore = createStore<PaperRecord[]>([]);

let paperSeq = 0;

const PATH_TAG = "/path:";
const EPOCH_TAG = "/policy_epoch:";

/*
parsePaperRecord reads one paper Value's vector layer, indexed
<prefix>.bin/<PaperKeys...>/path:<path>/policy_epoch:<epoch>. A layer that
does not carry every key, the path and the policy epoch is malformed: it is
reported and yields null.
*/
export const parsePaperRecord = (
	action: string,
	symbol: string,
	layers: Array<{ index: string; bytes: Uint8Array }>,
): PaperRecord | null => {
	const layer = layers.find((entry) => entry.index.includes(".bin/depth/"));

	if (!layer) {
		console.error(
			"parsePaperRecord: paper Value without its vector layer",
			action,
			symbol,
		);
		return null;
	}

	const rest = layer.index.slice(layer.index.indexOf(".bin/") + ".bin/".length);
	const pathAt = rest.indexOf(PATH_TAG);
	const epochAt = rest.lastIndexOf(EPOCH_TAG);
	const labels = pathAt >= 0 ? rest.slice(0, pathAt).split("/") : [];
	const path =
		pathAt >= 0 && epochAt > pathAt
			? rest.slice(pathAt + PATH_TAG.length, epochAt)
			: "";
	const policyEpoch =
		epochAt >= 0 ? rest.slice(epochAt + EPOCH_TAG.length) : "";
	const view = new DataView(
		layer.bytes.buffer,
		layer.bytes.byteOffset,
		layer.bytes.byteLength,
	);

	if (
		path === "" ||
		policyEpoch === "" ||
		labels.join("/") !== PAPER_KEYS.join("/") ||
		layer.bytes.byteLength !== PAPER_KEYS.length * 8
	) {
		console.error(
			"parsePaperRecord: malformed paper record layer",
			layer.index,
		);
		return null;
	}

	const fields = {} as Record<PaperKey, number>;

	PAPER_KEYS.forEach((key, idx) => {
		fields[key] = view.getFloat64(idx * 8, true);
	});

	return { seq: paperSeq++, action, symbol, path, policyEpoch, fields };
};

export const ingestPaperRecord = (record: PaperRecord | null) => {
	if (!record) return false;
	paperStore.setState((records) => records.concat(record).slice(-PAPER_LOG));
	return true;
};
