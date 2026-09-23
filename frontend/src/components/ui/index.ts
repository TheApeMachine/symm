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
	badgeVariants,
	setBadge,
	type BadgeVariant,
	type BadgeSize,
} from "./badge";
export { Button, type ButtonProps, buttonVariants } from "./button";
export { Callout, type CalloutProps, calloutVariants } from "./callout";
export {
	Canvas,
	type CanvasProps,
	CanvasPlot,
	type CanvasPlotProps,
	type CanvasPlotDraw,
} from "./canvas";
export { Card, CardPanel } from "./card";
export { Checkbox, type CheckboxProps } from "./checkbox";
export { Chip, type ChipProps, chipVariants } from "./chip";
export {
	Collapsible,
	CollapsibleTrigger,
	CollapsiblePanel,
	type CollapsibleProps,
} from "./collapsible";
export {
	DataRow,
	type DataRowProps,
	type DataRowGroupProps,
	dataRowVariants,
	dataRowValueVariants,
} from "./data-row";
export { Divider, type DividerProps, dividerVariants } from "./divider";
export { DOT_SIZE_FOR, Dot, type DotProps, dotVariants } from "./dot";
export { AnimatePresence, Flex, flexVariants } from "./flex";
export {
	Frame,
	FrameHeader,
	FrameTitle,
	FrameDescription,
	FrameFooter,
	frameVariants,
	type FrameProps,
} from "./frame";
export {
	type GapType,
	Grid,
	ISLAND_AREA_CLASS,
	ISLAND_GRID_CLASS,
	type IslandAreaKey,
	type SegmentsType,
} from "./grid";
export { ICON_NAMES, Icon, type IconName, type IconProps } from "./icon";
export {
	type FieldProps,
	fieldVariants,
	Input,
	type InputProps,
	inputVariants,
	type SearchProps,
} from "./input";
export {
	HeatmapRow,
	type HeatmapRowProps,
	HeatmapStrip,
	type HeatmapStripProps,
	HeatmapRowMetric,
	type HeatmapRowMetricProps,
	heatmapStripVariants,
} from "./heatmap-row";
export { Key, type KeyProps, keyVariants, type Modifier } from "./key";
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
	meterTrackVariants,
	meterVariants,
	setMeter,
	type MeterVariant,
	type MeterSize,
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
	Overlay,
	type OverlayProps,
	overlayContentVariants,
	overlayVariants,
} from "./overlay";
export {
	type JSONPrimitive,
	type JSONSerializable,
	type Paint,
	type SubscribableStore,
	type PaintMap,
	type PaintFieldRecord,
	type PaintMeterRecord,
	type PaintMeterValue,
	type PaintBadgeRecord,
	type PaintBadgeValue,
	type PaintVarRecord,
	applyPaintMap,
	usePaintStore,
} from "./paint";
export { Panel, type PanelProps, panelVariants } from "./panel";
export { Radar, type RadarProps, type RadarAxis, radarVariants } from "./radar";
export {
	Rail,
	type RailProps,
	type RailHeaderProps,
	type RailBodyProps,
	railVariants,
} from "./rail";
export { Readout, type ReadoutProps } from "./readout";
export { Scanlines, type ScanlinesProps, scanlinesVariants } from "./scanlines";
export {
	Section,
	type SectionHeaderProps,
	type SectionProps,
	sectionHeaderVariants,
	sectionVariants,
} from "./section";
export {
	computeSparklinePath,
	computeSparklinePaths,
	Sparkline,
	type SparklineProps,
	type SparklinePaths,
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
export {
	DistributionCurve,
	type DistributionCurveProps,
	computeDistributionPath,
} from "./distribution-curve";
export {
	RatioBar,
	type RatioBarProps,
	type RatioBarSegment,
	ratioBarTrackVariants,
} from "./ratio-bar";
export {
	Slider,
	type SliderProps,
	type SliderFieldProps,
	sliderVariants,
	sliderFieldVariants,
} from "./slider";
export { Toolbar, type ToolbarProps, toolbarVariants } from "./toolbar";
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
export { EvidenceGraph, type EvidenceGraphProps } from "./evidence-graph";
export type {
	Graph as EvidenceGraphData,
	GraphNode as EvidenceGraphNode,
	GraphEdge as EvidenceGraphEdge,
} from "./evidence-graph.types";
export {
	ImpulseMap,
	type ImpulseMapProps,
	type ImpulsePoint,
	type ImpulseRegion,
	type ImpulseContour,
	type ImpulseConnection,
} from "./impulse-map";

export {
	EpisodeTape,
	type EpisodeTapeProps,
	type TrainingEpisode,
	type TapePoint,
	type TapeMarker,
} from "./episode-tape";
export {
	PolicyBranches,
	type PolicyBranchesProps,
	type PolicyBranch,
} from "./policy-branches";
export {
	OutcomeDistribution,
	type OutcomeDistributionProps,
	type OutcomeBin,
} from "./outcome-distribution";
export {
	ForwardView,
	type ForwardViewProps,
	type ForwardSummary,
} from "./forward-view";
export {
	RecognitionView,
	type RecognitionViewProps,
	type RecognitionMetrics,
	type ActivityRow,
} from "./recognition-view";

export {
	TrieView,
	type TrieViewProps,
	type TrieNode,
	type TrieCandidate,
} from "./trie-view";

export { Explain } from "./explain";
export { KnowledgePanel } from "./knowledge-panel";
export { RecognitionPanel } from "./recognition-panel";
export { LearningPerformanceBanner } from "./performance-banner";
export { TerminalSignalHeatmap } from "./signal-heatmap";
export { FluidLegend } from "./fluid-legend";
export { Text, type TextProps } from "./text";
export {
	PositionList,
	type PositionListProps,
	type OpenPosition,
} from "./position-list";
export {
	DecisionList,
	type DecisionListProps,
	type DecisionRow,
} from "./decision-list";
