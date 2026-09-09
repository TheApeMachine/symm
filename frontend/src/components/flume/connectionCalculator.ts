/*
Barrel re-export for the connection helpers. The real implementations
live in sibling files split by concern:
  - connection-path-math: pure path geometry (smooth/straight/orthogonal)
  - connection-stage-coords: DOM lookup of stage/canvas elements + coord math
  - connection-ports: connection-id encoding + port handle lookups
  - connection-dom-sync: SVG element lifecycle for connection paths

Existing import sites keep using "#/components/flume/connectionCalculator"
unchanged; touching this barrel is rarely necessary.
*/

export {
	type ConnectionShellDescriptor,
	createSVG,
	deleteConnection,
	deleteConnectionsByNodeId,
	syncConnectionElements,
	updateConnection,
} from "./connection-dom-sync";
export {
	calculateEdgePath,
	calculateOrthogonalEdgePath,
	type EdgeRoutingMode,
	type ObstacleRect,
} from "./connection-path-math";

export {
	connectionId,
	findPortHandle,
	getPortInEditor,
	getPortRect,
	resolvePortDropTarget,
} from "./connection-ports";
export {
	getCanvasRef,
	getStageBounds,
	getStageRef,
	readLiveStageScale,
	screenPointToCanvas,
	screenRectToCanvas,
} from "./connection-stage-coords";
