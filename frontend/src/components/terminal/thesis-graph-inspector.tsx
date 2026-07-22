import {
	type GraphHit,
	measurementNumber,
	measurementString,
	nodeKind,
	nodeLabel,
} from "#/components/terminal/evidence-graph-viz";
import type { GraphNode } from "#/types/thesis";

const Row = ({ label, value }: { label: string; value: string }) => (
	<div className="flex justify-between gap-3">
		<span className="text-(--f4)">{label}</span>
		<span className="text-(--f1)">{value}</span>
	</div>
);

const validityLabel = (measurement: Record<string, unknown>): string => {
	const validity = measurement.validity;

	if (typeof validity === "object" && validity !== null) {
		const record = validity as Record<string, unknown>;
		const state = measurementString(record, "state");
		const reason = measurementString(record, "reason");

		return reason ? `${state} — ${reason}` : state;
	}

	return "";
};

const uncertaintyLabel = (measurement: Record<string, unknown>): string => {
	const uncertainty = measurement.uncertainty;

	if (typeof uncertainty === "object" && uncertainty !== null) {
		const record = uncertainty as Record<string, unknown>;
		const lower = measurementNumber(record, "lower");
		const upper = measurementNumber(record, "upper");

		if (lower !== null && upper !== null) {
			return `[${lower.toFixed(4)}, ${upper.toFixed(4)}]`;
		}
	}

	return "";
};

const NodeDetail = ({ node }: { node: GraphNode }) => {
	const measurement = node.measurement;
	const kind = nodeKind(node);
	const normalized = measurementNumber(measurement, "normalized");
	const raw = measurementNumber(measurement, "raw");
	const maturity = measurementNumber(measurement, "maturity");
	const at = measurementString(measurement, "at");
	const peer = measurementString(measurement, "peer");
	const validity = validityLabel(measurement);
	const uncertainty = uncertaintyLabel(measurement);

	return (
		<>
			<div className="mb-1 flex items-center gap-2 border-(--line) border-b pb-1">
				<span className="text-(--acc)">{nodeLabel(node)}</span>
				<span className="text-[9px] text-(--f4) uppercase">{kind}</span>
			</div>
			{kind === "measurement" ? (
				<>
					<Row label="source" value={measurementString(measurement, "source")} />
					{measurementString(measurement, "subject") && (
						<Row label="subject" value={measurementString(measurement, "subject")} />
					)}
					{measurementString(measurement, "side") && (
						<Row label="side" value={measurementString(measurement, "side")} />
					)}
					{raw !== null && <Row label="raw" value={raw.toFixed(4)} />}
					{normalized !== null && (
						<Row label="normalized" value={normalized.toFixed(4)} />
					)}
					{uncertainty && <Row label="uncertainty" value={uncertainty} />}
					{maturity !== null && (
						<Row label="maturity" value={maturity.toFixed(3)} />
					)}
					{validity && <Row label="validity" value={validity} />}
					{peer && <Row label="peer" value={peer} />}
				</>
			) : (
				<div className="text-(--f3)">
					{kind === "category"
						? "hypothesis · measurements draw supports/contradicts edges here"
						: "causal variable · conditions edge endpoint"}
				</div>
			)}
			{at && <Row label="at" value={at.slice(11, 23)} />}
		</>
	);
};

const EdgeDetail = ({
	edge,
}: {
	edge: { from: string; to: string; type: string; observedFrom: string };
}) => {
	const head = (key: string) => {
		const parts = key.split("/");

		return parts.length >= 3 ? `${parts[0]}/${parts[2]}` : key;
	};

	return (
		<>
			<div className="mb-1 border-(--line) border-b pb-1 text-(--acc) uppercase">
				{edge.type}
			</div>
			<Row label="from" value={head(edge.from)} />
			<Row label="to" value={head(edge.to)} />
			{edge.observedFrom && (
				<Row label="since" value={edge.observedFrom.slice(11, 19)} />
			)}
		</>
	);
};

/*
GraphInspector renders the hover tooltip for a node or edge, clamped near the
pointer, exposing the full measurement provenance the canvas cannot show inline.
*/
export const GraphInspector = ({
	hit,
	x,
	y,
}: {
	hit: GraphHit;
	x: number;
	y: number;
}) => {
	const left = x + 14;
	const top = y + 14;

	return (
		<div
			className="pointer-events-none absolute z-10 max-w-[240px] rounded border border-(--line) bg-(--surface)/95 px-2 py-1.5 font-mono text-[10px] shadow-lg"
			style={{ left, top }}
		>
			{hit.kind === "node" ? (
				<NodeDetail node={hit.node} />
			) : (
				<EdgeDetail edge={hit.edge} />
			)}
		</div>
	);
};
