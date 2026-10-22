/*
The library's public surface.

Import from here rather than from individual files. It keeps call sites short,
and it is the list that tells you at a glance whether the thing you are about to
hand-roll already exists.

The only hard dependency on the host app is `@/lib/utils`'s `cn`. Everything
else is this folder, `theme.css`, and the three packages in package.json:
class-variance-authority, motion, and tailwind-merge/clsx.
*/

export { Alert, alertVariants, type AlertProps } from "./alert";
export {
	Badge,
	badgeVariants,
	setBadge,
	type BadgeProps,
	type BadgeSize,
	type BadgeVariant,
} from "./badge";
export { Button, buttonVariants, type ButtonProps } from "./button";
export { Canvas, type CanvasProps } from "./canvas";
export { Chip, chipVariants, type ChipProps } from "./chip";
export { Component, type ComponentProps } from "./component";
export {
	DistanceBar,
	type DistanceBarProps,
	type DistanceMarker,
} from "./distance-bar";
export { Divider, dividerVariants, type DividerProps } from "./divider";
export {
	formatValue,
	memoizedQuery,
	memoizedQueryAll,
	renderValue,
	resetValue,
	setText,
	type ValueFormat,
} from "./dom";
export { Dot, DOT_SIZE_FOR, dotVariants, type DotProps } from "./dot";
export { AnimatePresence, Flex, flexVariants } from "./flex";
export { Funnel, type FunnelProps, type FunnelStage } from "./funnel";
export { Gauge, toneColor, type GaugeProps, type GaugeTone } from "./gauge";
export {
	Grid,
	ISLAND_AREA_CLASS,
	ISLAND_GRID_CLASS,
	type GapType,
	type IslandAreaKey,
	type SegmentsType,
} from "./grid";
export { Icon, ICON_NAMES, type IconName, type IconProps } from "./icon";
export {
	fieldVariants,
	Input,
	inputVariants,
	type FieldProps,
	type InputProps,
	type SearchProps,
} from "./input";
export { Key, keyVariants, type KeyProps, type Modifier } from "./key";
export {
	List,
	listItemVariants,
	listOptionVariants,
	type ListItemProps,
	type ListOptionProps,
	type ListProps,
} from "./list";
export {
	Meter,
	meterTrackVariants,
	meterVariants,
	setMeter,
	type MeterProps,
	type MeterSize,
	type MeterVariant,
} from "./meter";
export { Metric } from "./metric";
export {
	Modal,
	modalPanelVariants,
	modalScrimVariants,
	type ModalProps,
} from "./modal";
export {
	Nav,
	navItemVariants,
	navVariants,
	type NavItemProps,
	type NavProps,
} from "./nav";
export {
	Overlay,
	overlayContentVariants,
	overlayVariants,
	type OverlayProps,
} from "./overlay";
export type { JSONPrimitive, JSONSerializable, Paint } from "./paint";
export { Panel, panelVariants, type PanelProps } from "./panel";
export {
	RadialHistory,
	sectorHue,
	type RadialHistoryProps,
} from "./radial-history";
export { Readout, type ReadoutProps } from "./readout";
export { Scanlines, scanlinesVariants, type ScanlinesProps } from "./scanlines";
export {
	Section,
	sectionHeaderVariants,
	sectionVariants,
	type SectionHeaderProps,
	type SectionProps,
} from "./section";
export { SeriesChart, type SeriesChartProps } from "./series-chart";
export {
	computeSparklinePath,
	computeSparklinePaths,
	scaleSparklineValues,
	setSparkline,
	Sparkline,
	type SparklinePaths,
	type SparklineProps,
} from "./sparkline";
export { Spinner, type SpinnerProps } from "./spinner";
export { Stat, statVariants, type StatProps } from "./stat";
export {
	Tabs,
	tabsVariants,
	tabVariants,
	type TabProps,
	type TabsProps,
} from "./tabs";
export { TokenRibbon, type TokenRibbonProps } from "./token-ribbon";
export { Toolbar, toolbarVariants, type ToolbarProps } from "./toolbar";
export { Trail, type TrailProps } from "./trail";
export {
	SIZE_ORDER,
	stepSize,
	type Size,
	type Tone,
	type Variant,
} from "./types";
export {
	displayVariants,
	labelVariants,
	monoVariants,
	Typography,
	typographyVariants,
	type DisplayProps,
	type LabelProps,
	type MonoProps,
	type TypographyVariant,
} from "./typography";
