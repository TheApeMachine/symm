import { Link } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import type { ReactNode } from "react";
import { appStore } from "#/collections/app";
import { balancesStore } from "#/collections/balances";
import { terminalStore, type TerminalSurface } from "#/collections/terminal";
import { tickStore } from "#/collections/tick";
import { formatUptime } from "#/components/terminal/kernel-meta";
import { cn } from "#/lib/utils";

type TerminalRoutePath =
	| "/"
	| "/signals"
	| "/decisions"
	| "/xray"
	| "/cortex"
	| "/allocation";

const SURFACE_ITEMS: Array<{
	key: TerminalSurface;
	label: string;
	to: TerminalRoutePath;
}> = [
	{ key: "dashboard", label: "Dashboard", to: "/" },
	{ key: "signals", label: "Signal insight", to: "/signals" },
	{ key: "decisions", label: "Decision tree", to: "/decisions" },
	{ key: "xray", label: "Latent x-ray", to: "/xray" },
	{ key: "cortex", label: "Cognitive tree", to: "/cortex" },
	{ key: "allocation", label: "Allocation", to: "/allocation" },
];

const SymmLogo = () => (
	<svg
		width="22"
		height="22"
		viewBox="0 0 22 22"
		fill="none"
		className="block"
		aria-hidden="true"
	>
		<circle cx="11" cy="11" r="8.5" stroke="var(--acc)" strokeWidth="1.3" />
		<circle cx="11" cy="11" r="3.4" stroke="var(--acc)" strokeWidth="1.3" />
		<path
			d="M11 0.5V5M11 17V21.5M0.5 11H5M17 11H21.5"
			stroke="var(--acc)"
			strokeWidth="1.3"
		/>
	</svg>
);

const NavIcon = ({ surface }: { surface: TerminalSurface }) => {
	switch (surface) {
		case "dashboard":
			return (
				<svg
					width="15"
					height="15"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					strokeWidth="1.6"
				>
					<title>Dashboard</title>
					<rect x="3" y="3" width="7" height="9" />
					<rect x="14" y="3" width="7" height="5" />
					<rect x="14" y="12" width="7" height="9" />
					<rect x="3" y="16" width="7" height="5" />
				</svg>
			);
		case "signals":
			return (
				<svg
					width="15"
					height="15"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					strokeWidth="1.6"
					aria-hidden="true"
				>
					<title>Signal insight</title>
					<path d="M3 12h4l2 7 4-16 2 9h6" />
				</svg>
			);
		case "decisions":
			return (
				<svg
					width="15"
					height="15"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					strokeWidth="1.6"
				>
					<title>Decision tree</title>
					<circle cx="12" cy="4" r="2" />
					<circle cx="5" cy="20" r="2" />
					<circle cx="19" cy="20" r="2" />
					<path d="M12 6v5M12 11l-6 6M12 11l6 6" />
				</svg>
			);
		case "xray":
			return (
				<svg
					width="15"
					height="15"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					strokeWidth="1.6"
				>
					<title>Latent x-ray</title>
					<path d="M4 7V5a1 1 0 0 1 1-1h2M17 4h2a1 1 0 0 1 1 1v2M20 17v2a1 1 0 0 1-1 1h-2M7 20H5a1 1 0 0 1-1-1v-2" />
					<circle cx="12" cy="12" r="3.2" />
				</svg>
			);
		case "cortex":
			return (
				<svg
					width="15"
					height="15"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					strokeWidth="1.6"
				>
					<title>Cognitive tree</title>
					<circle cx="5" cy="12" r="2" />
					<circle cx="19" cy="6" r="2" />
					<circle cx="19" cy="18" r="2" />
					<path d="M7 12h4M11 12l6-5M11 12l6 5" />
				</svg>
			);
		default:
			return (
				<svg
					width="15"
					height="15"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					strokeWidth="1.6"
				>
					<title>Allocation</title>
					<path d="M3 3v18h18" />
					<rect x="7" y="11" width="3" height="7" />
					<rect x="12" y="7" width="3" height="11" />
					<rect x="17" y="4" width="3" height="14" />
				</svg>
			);
	}
};

const navStyle = (active: boolean) =>
	active
		? {
				borderColor: "var(--line2)",
				background: "var(--raised)",
				color: "var(--f1)",
			}
		: {
				borderColor: "transparent",
				background: "transparent",
				color: "var(--f3)",
			};

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
	<div
		className={cn(
			"flex min-h-0 flex-col overflow-hidden bg-(--surface)",
			className,
		)}
	>
		<div className="flex shrink-0 items-center justify-between border-(--line) border-b px-3 py-2">
			<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
				{title}
			</span>
			{meta ? (
				<span className="font-mono text-[10px] text-(--f4)">{meta}</span>
			) : null}
		</div>
		{children}
	</div>
);

export const TerminalTopBar = () => {
	const online = useSelector(appStore, (state) => state.online);
	const tick = useSelector(tickStore, (state) => state.frame);
	const { openPalette } = terminalStore.actions;

	const balances = useSelector(balancesStore, (state) => state.frame);
	const balancesList = (balances?.rows as Array<Record<string, unknown>>) ?? [];
	const usdBalance =
		balancesList.find((b) => b.asset === "USD" || b.asset === "EUR") ??
		balancesList[0];
	const cashValue = usdBalance
		? `${Number(usdBalance.balance).toFixed(2)} ${usdBalance.asset}`
		: "—";
	const availableValue = usdBalance
		? `${Number(usdBalance.balance).toFixed(2)} ${usdBalance.asset}`
		: "—";
	const reservedValue = usdBalance ? `0.00 ${usdBalance.asset}` : "—";

	return (
		<header className="relative z-5 flex h-[52px] shrink-0 items-center gap-3.5 border-(--line) border-b bg-(--surface) px-4">
			<div className="flex items-center gap-2">
				<SymmLogo />
				<span className="font-semibold text-[14px] text-(--f1) tracking-[0.22em]">
					SYMM
				</span>
			</div>
			<span
				className={cn(
					"inline-flex items-center gap-1.5 rounded-[2px] border px-[7px] py-0.5 font-semibold text-[10px] uppercase tracking-[0.08em]",
					online
						? "border-[color-mix(in_srgb,var(--up)_40%,transparent)] bg-[color-mix(in_srgb,var(--up)_12%,transparent)] text-(--up)"
						: "border-[color-mix(in_srgb,var(--down)_40%,transparent)] bg-[color-mix(in_srgb,var(--down)_12%,transparent)] text-(--down)",
				)}
			>
				<span
					className={cn(
						"size-1.5 rounded-full",
						online ? "animate-pulse bg-(--up)" : "bg-(--down)",
					)}
				/>
				{online ? "live" : "offline"}
			</span>
			<span className="font-mono text-[12px] text-(--f3)">
				{String(tick?.open ?? 0)} open positions
			</span>
			<div className="ml-auto flex items-center gap-[22px]">
				<button
					type="button"
					onClick={openPalette}
					className="flex cursor-pointer items-center gap-2 rounded-[3px] border border-(--line) bg-(--sunken) py-[5px] pr-2 pl-[9px] text-(--f3) hover:border-(--line2) hover:text-(--f1)"
				>
					<svg
						width="13"
						height="13"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						strokeWidth="1.8"
					>
						<title>Jump to</title>
						<circle cx="11" cy="11" r="7" />
						<path d="M21 21l-4.3-4.3" />
					</svg>
					<span className="text-[11px]">Jump to</span>
					<span className="rounded-[2px] border border-(--line) px-1 font-mono text-[10px] text-(--f4)">
						⌘K
					</span>
				</button>
				<TopMetric label="Cash" value={cashValue} strong />
				<TopMetric label="Available" value={availableValue} />
				<TopMetric label="Reserved" value={reservedValue} />
				<TopMetric
					label="Tick"
					value={tick?.count !== undefined ? String(tick.count) : "…"}
					accent
				/>
			</div>
		</header>
	);
};

const TopMetric = ({
	label,
	value,
	accent = false,
	strong = false,
}: {
	label: string;
	value: string;
	accent?: boolean;
	strong?: boolean;
}) => (
	<div className="flex flex-col items-end gap-px">
		<span className="text-[9px] text-(--f4) uppercase tracking-widest">
			{label}
		</span>
		<span
			className={cn(
				"font-mono text-[12px]",
				accent && "font-semibold text-(--acc)",
				strong && !accent && "font-medium text-(--f1)",
				!accent && !strong && "font-medium text-(--f2)",
			)}
		>
			{value}
		</span>
	</div>
);

export const TerminalNav = ({ active }: { active: TerminalSurface }) => {
	const scanlines = useSelector(terminalStore, (state) => state.scanlines);
	const fieldStyle = useSelector(terminalStore, (state) => state.fieldStyle);
	const { toggleScanlines, toggleFieldStyle } = terminalStore.actions;
	const online = useSelector(appStore, (state) => state.online);
	const tick = useSelector(tickStore, (state) => state.frame);
	const storyTicks = tick?.count ?? 0;
	const enginePhase = (tick?.phase as string) ?? "";
	const candidates = (tick?.candidates as number) ?? 0;
	const open = (tick?.open as number) ?? 0;
	const fluid = (tick?.fluid as number) ?? 0;
	const quotesReady = (tick?.quotes_ready as number) ?? 0;
	const quotesTotal = (tick?.quotes_total as number) ?? 0;
	const quotesPercent =
		quotesTotal > 0 ? Math.round((quotesReady / quotesTotal) * 100) : 0;
	const fluidPercent =
		quotesTotal > 0 ? Math.round((fluid / quotesTotal) * 100) : 0;
	const clockText = new Date().toISOString().slice(11, 19);

	return (
		<nav className="flex w-[210px] shrink-0 flex-col border-(--line) border-r bg-(--surface)">
			<div className="px-2.5 pt-3 pb-1.5 font-semibold text-[9px] text-(--f4) uppercase tracking-[0.14em]">
				Surfaces
			</div>
			<div className="flex flex-col gap-[3px] px-2">
				{SURFACE_ITEMS.map((item) => {
					const style = navStyle(active === item.key);

					return (
						<Link
							key={item.key}
							to={item.to}
							className="flex cursor-pointer items-center gap-2 rounded-[3px] border px-[9px] py-2 text-left text-[13px] font-medium hover:bg-(--raised)"
							style={style}
						>
							<NavIcon surface={item.key} />
							{item.label}
						</Link>
					);
				})}
			</div>
			<div className="px-2.5 pt-[18px] pb-1.5 font-semibold text-[9px] text-(--f4) uppercase tracking-[0.14em]">
				Engine
			</div>
			<div className="mx-2 border border-(--line) rounded-[3px] bg-(--sunken) p-2.5 font-mono text-[11px] leading-[1.7]">
				<MetricLine label="seq" value={`#${storyTicks}`} />
				<MetricLine
					label="phase"
					value={online ? enginePhase || "stream" : "offline"}
					accent
				/>
				<MetricLine label="cand" value={candidates.toString()} />
				<MetricLine label="open" value={open.toString()} />
				<ProgressLine
					label="quotes"
					text={quotesTotal > 0 ? `${quotesReady}/${quotesTotal}` : "—"}
					value={quotesPercent}
				/>
				<ProgressLine
					label="fluid"
					text={fluid > 0 ? String(fluid) : "—"}
					value={fluidPercent}
					accent
				/>
			</div>
			<div className="mt-auto border-(--line) border-t p-2.5 font-mono text-[10px] text-(--f4) leading-[1.6]">
				<div>{clockText} UTC</div>
				<div>uptime {formatUptime(null)}</div>
				<button
					type="button"
					onClick={toggleFieldStyle}
					className="mt-1.5 cursor-pointer border-0 bg-transparent p-0 font-[inherit] text-(--f3) hover:text-(--acc)"
				>
					field {fieldStyle.toLowerCase()}
				</button>
				<button
					type="button"
					onClick={toggleScanlines}
					className="mt-1.5 cursor-pointer border-0 bg-transparent p-0 font-[inherit] text-(--f3) hover:text-(--acc)"
				>
					scanlines {scanlines ? "on" : "off"}
				</button>
			</div>
		</nav>
	);
};

const MetricLine = ({
	label,
	value,
	accent = false,
}: {
	label: string;
	value: string;
	accent?: boolean;
}) => (
	<div className="flex justify-between">
		<span className="text-(--f4)">{label}</span>
		<span className={accent ? "text-(--acc)" : "text-(--f1)"}>{value}</span>
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
	<div className={label === "quotes" ? "mt-[7px]" : "mt-1.5"}>
		<div className="mb-[3px] flex justify-between text-(--f4)">
			<span>{label}</span>
			<span>{text}</span>
		</div>
		<div className="h-1 overflow-hidden rounded-[2px] bg-(--line)">
			<div
				className="h-full transition-[width] duration-500 ease-out"
				style={{
					width: `${value}%`,
					background: accent ? "var(--acc)" : "var(--info)",
				}}
			/>
		</div>
	</div>
);
