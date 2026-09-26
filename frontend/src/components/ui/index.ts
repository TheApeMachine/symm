/*
The library's public surface.

Import from here rather than from individual files. It keeps call sites short,
and it is the list that tells you at a glance whether the thing you are about to
hand-roll already exists.

The only hard dependency on the host app is `@/lib/utils`'s `cn`. Everything
else is this folder, `theme.css`, and the three packages in package.json:
class-variance-authority, motion, and tailwind-merge/clsx.
*/

export { Alert, type AlertProps, alertVariants } from "./alert";
export {
	Badge,
	type BadgeProps,
	type BadgeSize,
	type BadgeVariant,
	badgeVariants,
	setBadge,
} from "./badge";
export { Button, type ButtonProps, buttonVariants } from "./button";
export { Callout, type CalloutProps, calloutVariants } from "./callout";
export {
	Canvas,
	CanvasPlot,
	type CanvasPlotDraw,
	type CanvasPlotProps,
	type CanvasProps,
} from "./canvas";
export { Card, CardPanel } from "./card";
export { Checkbox, type CheckboxProps } from "./checkbox";
export { Chip, type ChipProps, chipVariants } from "./chip";
export {
	Collapsible,
	CollapsiblePanel,
	type CollapsibleProps,
	CollapsibleTrigger,
} from "./collapsible";
export {
	DataRow,
	type DataRowGroupProps,
	type DataRowProps,
	dataRowValueVariants,
	dataRowVariants,
} from "./data-row";
export {
	DecisionList,
	type DecisionListProps,
	type DecisionRow,
} from "./decision-list";
export { Decisions, type DecisionsProps } from "./decisions";
export {
	computeDistributionPath,
	DistributionCurve,
	type DistributionCurveProps,
} from "./distribution-curve";
export { Divider, type DividerProps, dividerVariants } from "./divider";
export { DOT_SIZE_FOR, Dot, type DotProps, dotVariants } from "./dot";
export {
	EpisodeTape,
	type EpisodeTapeProps,
	type TapeMarker,
	type TapePoint,
	type TrainingEpisode,
} from "./episode-tape";
export { EvidenceGraph, type EvidenceGraphProps } from "./evidence-graph";
export type {
	Graph as EvidenceGraphData,
	GraphEdge as EvidenceGraphEdge,
	GraphNode as EvidenceGraphNode,
} from "./evidence-graph.types";
export { Explain } from "./explain";
export { AnimatePresence, Flex, flexVariants } from "./flex";
export { FluidLegend } from "./fluid-legend";
export {
	type ForwardSummary,
	ForwardView,
	type ForwardViewProps,
} from "./forward-view";
export {
	Frame,
	FrameDescription,
	FrameFooter,
	FrameHeader,
	type FrameProps,
	FrameTitle,
	frameVariants,
} from "./frame";
export {
	type GapType,
	Grid,
	ISLAND_AREA_CLASS,
	ISLAND_GRID_CLASS,
	type IslandAreaKey,
	type SegmentsType,
} from "./grid";
export {
	HeatmapRow,
	HeatmapRowMetric,
	type HeatmapRowMetricProps,
	type HeatmapRowProps,
	HeatmapStrip,
	type HeatmapStripProps,
	heatmapStripVariants,
} from "./heatmap-row";
export { ICON_NAMES, Icon, type IconName, type IconProps } from "./icon";
export {
	type ImpulseConnection,
	type ImpulseContour,
	ImpulseMap,
	type ImpulseMapProps,
	type ImpulsePoint,
	type ImpulseRegion,
} from "./impulse-map";
export {
	type FieldProps,
	fieldVariants,
	Input,
	type InputProps,
	inputVariants,
	type SearchProps,
} from "./input";
export { KernelCard, type KernelCardProps } from "./kernel-card";
export {
	KernelInspector,
	type KernelInspectorProps,
} from "./kernel-inspector";
export { KernelList, type KernelListProps } from "./kernel-list";
export { Key, type KeyProps, keyVariants, type Modifier } from "./key";
export { KnowledgePanel } from "./knowledge-panel";
export {
	List,
	type ListItemProps,
	type ListOptionProps,
	type ListProps,
	listItemVariants,
	listOptionVariants,
} from "./list";
export {
	Meter,
	type MeterProps,
	type MeterSize,
	type MeterVariant,
	meterTrackVariants,
	meterVariants,
	setMeter,
} from "./meter";
export {
	Modal,
	type ModalProps,
	modalPanelVariants,
	modalScrimVariants,
} from "./modal";
export {
	Nav,
	type NavItemProps,
	type NavProps,
	navItemVariants,
	navVariants,
} from "./nav";
export {
	type OutcomeBin,
	OutcomeDistribution,
	type OutcomeDistributionProps,
} from "./outcome-distribution";
export {
	Overlay,
	type OverlayProps,
	overlayContentVariants,
	overlayVariants,
} from "./overlay";
export {
	applyPaintMap,
	type JSONPrimitive,
	type JSONSerializable,
	type Paint,
	type PaintBadgeRecord,
	type PaintBadgeValue,
	type PaintFieldRecord,
	type PaintMap,
	type PaintMeterRecord,
	type PaintMeterValue,
	type PaintVarRecord,
	type SubscribableStore,
	usePaintStore,
} from "./paint";
export { Panel, type PanelProps, panelVariants } from "./panel";
export { LearningPerformanceBanner } from "./performance-banner";
export {
	type PolicyBranch,
	PolicyBranches,
	type PolicyBranchesProps,
} from "./policy-branches";
export {
	type OpenPosition,
	PositionList,
	type PositionListProps,
} from "./position-list";
export { Positions, type PositionsProps } from "./positions";
export {
	HierarchyLanes,
	type HierarchyLanesProps,
	PredictionChart,
	type PredictionChartProps,
	type PredictionLayer,
	ScalarDiagnostics,
	type ScalarDiagnosticsProps,
	TerminalPredictionChart,
	VectorLane,
	type VectorLaneProps,
	VerdictRow,
	type VerdictRowProps,
} from "./prediction-chart";
export {
	PredictiveCodingCanvas,
	type PredictiveCodingCanvasProps,
} from "./predictive-coding-canvas";
export { Pulse, type PulseProps } from "./pulse";
export { Radar, type RadarAxis, type RadarProps, radarVariants } from "./radar";
export {
	Rail,
	type RailBodyProps,
	type RailHeaderProps,
	type RailProps,
	railVariants,
} from "./rail";
export {
	RatioBar,
	type RatioBarProps,
	type RatioBarSegment,
	ratioBarTrackVariants,
} from "./ratio-bar";
export { Readout, type ReadoutProps } from "./readout";
export { RecognitionPanel } from "./recognition-panel";
export {
	type ActivityRow,
	type RecognitionMetrics,
	RecognitionView,
	type RecognitionViewProps,
} from "./recognition-view";
export { Scanlines, type ScanlinesProps, scanlinesVariants } from "./scanlines";
export {
	Section,
	type SectionHeaderProps,
	type SectionProps,
	sectionHeaderVariants,
	sectionVariants,
} from "./section";
export { TerminalSignalHeatmap } from "./signal-heatmap";
export {
	type SkillActivity,
	SkillPanel,
	type SkillPanelProps,
} from "./skill-panel";
export {
	Slider,
	type SliderFieldProps,
	type SliderProps,
	sliderFieldVariants,
	sliderVariants,
} from "./slider";
export {
	computeSparklinePath,
	computeSparklinePaths,
	Sparkline,
	type SparklinePaths,
	type SparklineProps,
	setSparkline,
} from "./sparkline";
export { Spinner, type SpinnerProps } from "./spinner";
export { Stat, type StatProps, statVariants } from "./stat";
export {
	StepCard,
	type StepCardProps,
	stepCardVariants,
} from "./step-card";
export {
	type TabProps,
	Tabs,
	type TabsProps,
	tabsVariants,
	tabVariants,
} from "./tabs";
export { Text, type TextProps } from "./text";
export { ThesisModal, type ThesisModalProps } from "./thesis-modal";
export { Toolbar, type ToolbarProps, toolbarVariants } from "./toolbar";
export {
	type TrieCandidate,
	type TrieNode,
	TrieView,
	type TrieViewProps,
} from "./trie-view";
export {
	SIZE_ORDER,
	type Size,
	stepSize,
	type Tone,
	type Variant,
} from "./types";
export {
	type DisplayProps,
	displayVariants,
	type LabelProps,
	labelVariants,
	type MonoProps,
	monoVariants,
	Typography,
	type TypographyVariant,
	typographyVariants,
} from "./typography";
