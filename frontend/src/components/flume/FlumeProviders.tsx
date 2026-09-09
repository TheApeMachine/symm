import type React from "react";
import type { RefObject } from "react";
import type FlumeCache from "#/components/flume/Cache";
import type { EdgeRoutingMode } from "#/components/flume/connectionCalculator";
import {
	CacheContext,
	ConnectionRecalculateContext,
	ContextContext,
	EdgeRoutingContext,
	EditorIdContext,
	FlumeGraphWorkerContext,
	type FlumeGraphWorkerHandle,
	GraphIdContext,
	NodeActionsContext,
	NodeDragOverrideContext,
	NodeMapContext,
	NodeTypesContext,
	PortTypesContext,
	RecalculateStageRectContext,
	StageContext,
} from "#/components/flume/context";
import type { NodeActions } from "#/components/flume/nodes-actions";
import type { SpatialIndexSnapshot } from "#/components/flume/spatial-index";
import type { StageState } from "#/components/flume/stageReducer";
import type {
	Coordinate,
	NodeMap,
	NodeTypeMap,
	PortTypeMap,
} from "#/components/flume/types";
import {
	ObstacleIndexContext,
	PortLayoutRegistrationContext,
	type RegisterPortLayout,
} from "#/components/flume/useSpatialIndex";

/*
FlumeProviders bundles every context the Flume editor stack relies on
into a single component. NodeEditor used to nest 15 providers inline,
which was load-bearing react-flume legacy and made the return JSX
opaque. Adding a new context now means extending this value object,
not the wall of opening tags.
*/

export type FlumeProvidersValue = {
	indexRef: RefObject<SpatialIndexSnapshot>;
	registerPortLayout: RegisterPortLayout;
	graphWorker: FlumeGraphWorkerHandle;
	dragOverride: Record<string, Coordinate> | null;
	nodes: NodeMap;
	edgeRoutingMode: EdgeRoutingMode;
	portTypes: PortTypeMap;
	nodeTypes: NodeTypeMap;
	nodeActions: NodeActions;
	triggerRecalculation: () => void;
	context: unknown;
	stageState: StageState;
	cache: RefObject<FlumeCache>;
	graphId: string;
	editorId: string;
	recalculateStageRect: () => void;
};

export const FlumeProviders = ({
	value,
	children,
}: {
	value: FlumeProvidersValue;
	children: React.ReactNode;
}): React.ReactElement => {
	return (
		<ObstacleIndexContext.Provider value={value.indexRef}>
			<PortLayoutRegistrationContext.Provider value={value.registerPortLayout}>
				<FlumeGraphWorkerContext.Provider value={value.graphWorker}>
					<NodeDragOverrideContext.Provider value={value.dragOverride}>
						<NodeMapContext.Provider value={value.nodes}>
							<EdgeRoutingContext.Provider value={value.edgeRoutingMode}>
								<PortTypesContext.Provider value={value.portTypes}>
									<NodeTypesContext.Provider value={value.nodeTypes}>
										<NodeActionsContext.Provider value={value.nodeActions}>
											<ConnectionRecalculateContext.Provider
												value={value.triggerRecalculation}
											>
												<ContextContext.Provider value={value.context}>
													<StageContext.Provider value={value.stageState}>
														<CacheContext.Provider value={value.cache}>
															<GraphIdContext.Provider value={value.graphId}>
																<EditorIdContext.Provider
																	value={value.editorId}
																>
																	<RecalculateStageRectContext.Provider
																		value={value.recalculateStageRect}
																	>
																		{children}
																	</RecalculateStageRectContext.Provider>
																</EditorIdContext.Provider>
															</GraphIdContext.Provider>
														</CacheContext.Provider>
													</StageContext.Provider>
												</ContextContext.Provider>
											</ConnectionRecalculateContext.Provider>
										</NodeActionsContext.Provider>
									</NodeTypesContext.Provider>
								</PortTypesContext.Provider>
							</EdgeRoutingContext.Provider>
						</NodeMapContext.Provider>
					</NodeDragOverrideContext.Provider>
				</FlumeGraphWorkerContext.Provider>
			</PortLayoutRegistrationContext.Provider>
		</ObstacleIndexContext.Provider>
	);
};
