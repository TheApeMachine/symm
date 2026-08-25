/*
The library's public surface.

Import from here rather than from individual files. It keeps call sites short,
and it is the list that tells you at a glance whether the thing you are about to
hand-roll already exists.

The only hard dependency on the host app is `@/lib/utils`'s `cn`. Everything
else is this folder, `theme.css`, and the three packages in package.json:
class-variance-authority, motion, and tailwind-merge/clsx.
*/

export { Badge, type BadgeProps, badgeVariants } from "./badge";
export { Button, type ButtonProps, buttonVariants } from "./button";
export { Canvas, type CanvasProps } from "./canvas";
export { Chip, type ChipProps, chipVariants } from "./chip";
export { Divider, type DividerProps, dividerVariants } from "./divider";
export { DOT_SIZE_FOR, Dot, type DotProps, dotVariants } from "./dot";
export { AnimatePresence, Flex, flexVariants } from "./flex";
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
export type { JSONPrimitive, JSONSerializable, Paint } from "./paint";
export { Panel, type PanelProps, panelVariants } from "./panel";
export { Scanlines, type ScanlinesProps, scanlinesVariants } from "./scanlines";
export {
	Section,
	type SectionHeaderProps,
	type SectionProps,
	sectionHeaderVariants,
	sectionVariants,
} from "./section";
export { Spinner, type SpinnerProps } from "./spinner";
export { Stat, type StatProps, statVariants } from "./stat";
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