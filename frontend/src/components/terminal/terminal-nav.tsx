import { Link } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { type TerminalSurface, terminalStore } from "#/collections/terminal";
import { cn } from "#/lib/utils";
import { Clock } from "@/components/clock";
import { Engine } from "@/components/engine";

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
					aria-hidden="true"
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

export const TerminalNav = ({ active }: { active: TerminalSurface }) => {
	const scanlines = useSelector(terminalStore, (state) => state.scanlines);
	const { toggleScanlines } = terminalStore.actions;

	return (
		<nav className="flex w-52.5 shrink-0 flex-col border-(--line) border-r bg-(--surface)">
			<div className="px-2.5 pt-3 pb-1.5 font-semibold text-[9px] text-(--f4) uppercase tracking-[0.14em]">
				Surfaces
			</div>
			<div className="flex flex-col gap-0.75 px-2">
				{SURFACE_ITEMS.map((item) => (
					<Link
						key={item.key}
						to={item.to}
						className={cn(
							"flex cursor-pointer items-center gap-2 rounded-[3px] border px-2.25 py-2 text-left text-[13px] font-medium hover:bg-(--raised)",
							active === item.key
								? "border-(--line2) bg-(--raised) text-(--f1)"
								: "border-transparent bg-transparent text-(--f3)",
						)}
					>
						<NavIcon surface={item.key} />
						{item.label}
					</Link>
				))}
			</div>
			<div className="px-2.5 pt-4.5 pb-1.5 font-semibold text-[9px] text-(--f4) uppercase tracking-[0.14em]">
				Engine
			</div>
			<Engine />
			<div className="mt-auto border-(--line) border-t p-2.5 font-mono text-[10px] text-(--f4) leading-[1.6]">
				<Clock />
				<button
					type="button"
					onClick={toggleScanlines}
					className="mt-1.5 block cursor-pointer border-0 bg-transparent p-0 font-[inherit] text-(--f3) hover:text-(--acc)"
				>
					scanlines {scanlines ? "on" : "off"}
				</button>
			</div>
		</nav>
	);
};
