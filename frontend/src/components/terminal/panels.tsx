import {
	ActivityIcon,
	BrainCircuitIcon,
	ChartNoAxesColumnIncreasingIcon,
	GitBranchIcon,
	LayoutDashboardIcon,
	RadioIcon,
	ScanEyeIcon,
	SearchIcon,
	WavesIcon,
} from "lucide-react";
import type { ReactNode } from "react";
import type {
	TerminalModel,
	TerminalSurface,
} from "#/components/terminal/model";
import { toneClasses } from "#/components/terminal/tone";
import { Logo } from "#/components/ui/logo";
import { cn } from "#/lib/utils";

const SURFACE_ITEMS: Array<{
	key: TerminalSurface;
	label: string;
	icon: ReactNode;
}> = [
	{ key: "dashboard", label: "Dashboard", icon: <LayoutDashboardIcon /> },
	{ key: "signals", label: "Signal insight", icon: <ActivityIcon /> },
	{ key: "decisions", label: "Decision tree", icon: <GitBranchIcon /> },
	{ key: "xray", label: "Latent x-ray", icon: <ScanEyeIcon /> },
	{ key: "cortex", label: "Cognitive tree", icon: <BrainCircuitIcon /> },
	{
		key: "allocation",
		label: "Allocation",
		icon: <ChartNoAxesColumnIncreasingIcon />,
	},
];

const TerminalChrome = ({
	children,
	className,
}: {
	children: ReactNode;
	className?: string;
}) => (
	<div
		className={cn(
			"rounded-[3px] border border-(--line) bg-(--sunken)",
			className,
		)}
	>
		{children}
	</div>
);

export const TerminalSection = ({
	title,
	meta,
	children,
	className,
}: {
	title: string;
	meta?: ReactNode;
	children: ReactNode;
	className?: string;
}) => (
	<TerminalChrome className={className}>
		<div className="flex h-full min-h-0 flex-col overflow-hidden rounded-[3px]">
			<div className="flex h-8 shrink-0 items-center justify-between gap-3 border-(--line) border-b px-3">
				<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.16em]">
					{title}
				</span>
				{meta ? (
					<span className="font-mono text-[10px] text-(--f4)">{meta}</span>
				) : null}
			</div>
			{children}
		</div>
	</TerminalChrome>
);

export const TerminalTopBar = ({
	model,
	onOpenPalette,
}: {
	model: TerminalModel;
	onOpenPalette: () => void;
}) => (
	<header className="flex h-[52px] shrink-0 items-center gap-4 border-(--line) border-b bg-(--surface) px-4">
		<div className="flex items-center gap-3">
			<Logo />
		</div>
		<span
			className={cn(
				"inline-flex items-center gap-1.5 rounded-sm border px-2 py-1 font-semibold text-[10px] uppercase tracking-[0.12em]",
				model.online ? toneClasses("good") : toneClasses("bad"),
			)}
		>
			<span
				className={cn(
					"size-1.5 rounded-full",
					model.online ? "bg-emerald-300" : "bg-rose-300",
				)}
			/>
			{model.online ? "live" : "offline"}
		</span>
		<span className="font-mono text-stone-500 text-xs">
			{model.wallet.openText}
		</span>
		<button
			type="button"
			onClick={onOpenPalette}
			className="ml-auto hidden h-8 items-center gap-2 rounded-[3px] border border-(--line) bg-(--sunken) px-2 font-mono text-[12px] text-(--f3) hover:border-(--line2) hover:text-(--f1) md:inline-flex"
		>
			<SearchIcon className="size-3.5" />
			<span>Jump to</span>
			<span className="rounded-[3px] border border-(--line) px-1 font-mono text-[10px] text-(--f4)">
				⌘K
			</span>
		</button>
		<div className="hidden items-center gap-5 md:flex">
			<TopMetric label="Cash" value={model.wallet.cash} />
			<TopMetric label="Available" value={model.wallet.available} />
			<TopMetric label="Reserved" value={model.wallet.reserved} />
			<TopMetric label="Tick" value={model.wallet.tick} accent />
		</div>
	</header>
);

const TopMetric = ({
	label,
	value,
	accent = false,
}: {
	label: string;
	value: string;
	accent?: boolean;
}) => (
	<div className="flex flex-col items-end leading-tight">
		<span className="font-semibold text-[9px] text-stone-600 uppercase tracking-[0.14em]">
			{label}
		</span>
		<span
			className={cn(
				"font-mono text-xs",
				accent ? "text-(--acc)" : "text-(--f1)",
			)}
		>
			{value}
		</span>
	</div>
);

export const TerminalNav = ({
	active,
	model,
	onSelect,
}: {
	active: TerminalSurface;
	model: TerminalModel;
	onSelect: (surface: TerminalSurface) => void;
}) => (
	<nav className="flex w-[210px] shrink-0 flex-col border-(--line) border-r bg-(--surface)">
		<div className="px-3 pt-4 pb-2 font-semibold text-[10px] text-(--f4) uppercase tracking-[0.16em]">
			Surfaces
		</div>
		<div className="flex flex-col gap-1 px-2">
			{SURFACE_ITEMS.map((item) => (
				<button
					key={item.key}
					type="button"
					onClick={() => onSelect(item.key)}
					className={cn(
						"flex h-9 items-center gap-2 rounded-[3px] border border-transparent px-2 text-left text-[13px] text-(--f3) hover:bg-(--raised) hover:text-(--f1) [&_svg]:size-4",
						active === item.key &&
							"border-[rgba(232,163,61,0.45)] bg-[rgba(232,163,61,0.12)] text-(--f1)",
					)}
				>
					<span className="size-4">{item.icon}</span>
					{item.label}
				</button>
			))}
		</div>
		<div className="px-3 pt-5 pb-2 font-semibold text-[10px] text-(--f4) uppercase tracking-[0.16em]">
			Engine
		</div>
		<div className="mx-2 rounded-[3px] border border-(--line) bg-(--sunken) p-3 font-mono text-[11px] text-(--f3)">
			<MetricLine label="seq" value={model.engine.sequence} />
			<MetricLine label="phase" value={model.engine.phase} accent />
			<MetricLine label="cand" value={model.engine.candidates.toString()} />
			<MetricLine label="open" value={model.engine.open.toString()} />
			<ProgressLine
				label="quotes"
				text={model.engine.signalsText}
				value={model.engine.signalsPercent}
			/>
			<ProgressLine
				label="fluid"
				text={model.engine.fluidText}
				value={model.engine.fluidPercent}
				accent
			/>
		</div>
		<div className="mt-auto border-(--line) border-t p-3 font-mono text-[10px] text-(--f4)">
			<div>{model.clockText} UTC</div>
			<div>uptime {model.wallet.tick}</div>
		</div>
	</nav>
);

const MetricLine = ({
	label,
	value,
	accent = false,
}: {
	label: string;
	value: string;
	accent?: boolean;
}) => (
	<div className="flex justify-between gap-3">
		<span className="text-(--f4)">{label}</span>
		<span className={accent ? "text-(--acc)" : "text-(--f2)"}>
			{value}
		</span>
	</div>
);

const ProgressLine = ({
	label,
	text,
	value,
	accent = false,
}: {
	label: string;
	text: string;
	value: number;
	accent?: boolean;
}) => (
	<div className="mt-2">
		<div className="mb-1 flex justify-between text-(--f4)">
			<span>{label}</span>
			<span>{text}</span>
		</div>
		<div className="h-1 overflow-hidden rounded-sm bg-(--line)">
			<div
				className={cn(
					"h-full",
					accent ? "bg-(--acc)" : "bg-info",
				)}
				style={{ width: `${value}%` }}
			/>
		</div>
	</div>
);

export const TerminalToolbar = () => (
	<div className="flex h-8 shrink-0 items-center gap-2 border-(--line) border-b bg-(--sunken) px-3 font-mono text-[10px] text-(--f4)">
		<RadioIcon className="size-3.5 text-info" />
		<span>websocket</span>
		<WavesIcon className="ml-3 size-3.5 text-(--acc)" />
		<span>artifact stream</span>
		<SearchIcon className="ml-auto size-3.5 text-(--f4)" />
	</div>
);
