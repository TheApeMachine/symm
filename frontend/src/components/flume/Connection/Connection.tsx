"use client";

import { calculateEdgePath } from "#/components/flume/connectionCalculator";
import { useEdgeRouting } from "#/components/flume/context";
import type { Coordinate } from "#/components/flume/types";
import styles from "./Connection.module.css";

// Local structural ref types to avoid depending on React's Ref alias,
// which can differ across package boundaries and cause type identity issues.
type RefObject<T> = { current: T | null };
type RefLike<T> = ((instance: T | null) => void) | RefObject<T> | null;

interface ConnectionProps {
	from: Coordinate;
	to: Coordinate;
	id?: string;
	lineRef?: RefLike<SVGPathElement>;
	outputNodeId?: string;
	outputPortName?: string;
	inputNodeId?: string;
	inputPortName?: string;
}

const Connection = ({
	from,
	to,
	id,
	lineRef,
	outputNodeId,
	outputPortName,
	inputNodeId,
	inputPortName,
}: ConnectionProps) => {
	const routing = useEdgeRouting();
	const curve = calculateEdgePath(routing, from, to);
	return (
		<svg
			className={styles.svg}
			data-flume-component="connection-svg"
			role="img"
		>
			<title>Connection</title>
			<path
				data-connection-id={id}
				data-output-node-id={outputNodeId}
				data-output-port-name={outputPortName}
				data-input-node-id={inputNodeId}
				data-input-port-name={inputPortName}
				data-flume-component="connection-path"
				stroke="rgb(185, 186, 189)"
				fill="none"
				strokeLinecap="round"
				strokeLinejoin={routing === "orthogonal" ? "miter" : "round"}
				strokeWidth={3}
				d={curve}
				ref={lineRef}
			/>
		</svg>
	);
};

export default Connection;
