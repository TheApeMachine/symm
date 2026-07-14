import { Link } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import type { ReactNode } from "react";
import { appStore } from "#/collections/app";
import type { Balance } from "#/collections/balances";
import type { Position } from "#/collections/positions";
import { type TerminalSurface, terminalStore } from "#/collections/terminal";
import { formatUptime } from "#/components/terminal/kernel-meta";
import {
	LiveEngineTicker,
	LiveOpenCount,
	LiveWalletMetrics,
} from "#/components/terminal/live-ticker";
import { cn } from "#/lib/utils";
import { Badge } from "@/components/ui/badge";
import { panelVariants } from "@/components/ui/panel";

type WalletMetrics = {
	asset: string;
	cash: number;
	available: number;
	reserved: number;
	unrealized: number;
	equity: number;
};

export const walletMetrics = (
	balances: Balance[],
	positions: Position[],
): WalletMetrics | null => {
	if (balances.length === 0) {
		return null;
	}

	if (balances.length !== 1) {
		throw new TypeError("wallet requires exactly one quote balance");
	}

	const balance = balances[0];
	const unrealized = positions.reduce(
		(total, position) => total + position.pnl,
		0,
	);

	return {
		asset: balance.asset,
		cash: balance.balance,
		available: balance.available,
		reserved: balance.reserved,
		unrealized,
		equity: balance.balance + unrealized,
	};
};

type TerminalRoutePath =
	| "/"
	| "/signals"
	| "/decisions"
	| "/journal"
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
	{ key: "journal", label: "Trade journal", to: "/journal" },
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
		case "journal":
			return (
				<svg
					width="15"
					height="15"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					strokeWidth="1.6"
				>
					<title>Trade journal</title>
					<path d="M6 4h9l3 3v13H6z" />
					<path d="M15 4v3h3" />
					<path d="M8 11h8M8 15h8" />
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
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const { openPalette } = terminalStore.actions;

	return (
		<header className="relative z-5 flex h-[52px] shrink-0 items-center gap-3.5 border-(--line) border-b bg-(--surface) px-4">
			<div className="flex items-center gap-2">
				<SymmLogo />
				<span className="font-semibold text-[14px] text-(--f1) tracking-[0.22em]">
					SYMM
				</span>
			</div>
			<Badge
				label={online ? "live" : "offline"}
				variant={online ? "success" : "error"}
				dot
				className={online ? "[&_span[aria-hidden]]:animate-pulse" : undefined}
			/>
			<LiveOpenCount />
			<div className="ml-auto flex items-center gap-[22px]">
				<Badge
					label={focusSymbol}
					variant="warning"
					size="m"
					data-symbol={focusSymbol}
					title="Focused symbol"
					className="cursor-pointer font-mono"
				/>
				<button
					type="button"
					onClick={openPalette}
					className={cn(
						panelVariants({ size: "s" }),
						"flex cursor-pointer items-center gap-2 py-[5px] pr-2 pl-[9px] text-(--f3) hover:border-(--line2) hover:text-(--f1)",
					)}
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
				<LiveWalletMetrics />
			</div>
		</header>
	);
};

export const TerminalNav = ({ active }: { active: TerminalSurface }) => {
	const scanlines = useSelector(terminalStore, (state) => state.scanlines);
	const fieldStyle = useSelector(terminalStore, (state) => state.fieldStyle);
	const { toggleScanlines, toggleFieldStyle } = terminalStore.actions;
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
			<LiveEngineTicker />
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
