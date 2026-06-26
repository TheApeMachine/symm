import {
	ClientOnly,
	createRootRoute,
	HeadContent,
	Scripts,
	useLocation,
	useNavigate,
} from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useEffect } from "react";
import { terminalStore } from "#/collections/terminal";
import type { TerminalSurface } from "#/components/terminal/model";
import { CommandPalette } from "#/components/terminal/palette";
import { TerminalNav, TerminalTopBar } from "#/components/terminal/panels";
import { WsFeed } from "#/providers/websocket";
import appCss from "../app.css?url";

const SURFACE_VALUES = new Set<TerminalSurface>([
	"dashboard",
	"signals",
	"decisions",
	"xray",
	"cortex",
	"allocation",
]);

const SURFACE_ALIASES: Record<string, TerminalSurface> = {
	insight: "signals",
	decision: "decisions",
	alloc: "allocation",
};

const SURFACE_PATHS: Record<TerminalSurface, string> = {
	dashboard: "/",
	signals: "/signals",
	decisions: "/decisions",
	xray: "/xray",
	cortex: "/cortex",
	allocation: "/allocation",
};

export const parseSurface = (path: unknown): TerminalSurface => {
	const value = typeof path === "string" ? path.replace(/^\/+/, "") : "";
	const normalized = SURFACE_ALIASES[value] ?? value;

	return SURFACE_VALUES.has(normalized as TerminalSurface)
		? (normalized as TerminalSurface)
		: "dashboard";
};

const RootDocument = ({ children }: { children: React.ReactNode }) => {
	const navigate = useNavigate();
	const location = useLocation();
	const surface = parseSurface(location.pathname);
	const scanlines = useSelector(terminalStore, (state) => state.scanlines);
	const { inspectSource, openPalette, closePalette, bumpPaletteIndex } =
		terminalStore.actions;

	const runPalette = (nextSurface: TerminalSurface, source?: string) => {
		if (source) {
			inspectSource(source);
		}

		closePalette();
		navigate({ to: SURFACE_PATHS[nextSurface] });
	};

	useEffect(() => {
		const onKey = (event: KeyboardEvent) => {
			if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
				event.preventDefault();
				openPalette();
				return;
			}

			if (!terminalStore.state.paletteOpen) {
				return;
			}

			if (event.key === "Escape") {
				closePalette();
				return;
			}

			if (event.key === "ArrowDown") {
				event.preventDefault();
				bumpPaletteIndex(1);
				return;
			}

			if (event.key === "ArrowUp") {
				event.preventDefault();
				bumpPaletteIndex(-1);
			}
		};

		window.addEventListener("keydown", onKey);

		return () => window.removeEventListener("keydown", onKey);
	}, [bumpPaletteIndex, closePalette, openPalette]);

	return (
		<html lang="en" suppressHydrationWarning>
			<head>
				<HeadContent />
			</head>
			<body className="flex h-full min-h-svh flex-col" suppressHydrationWarning>
				<ClientOnly fallback={null}>
					<WsFeed />
					<div className="fixed inset-0 z-50 flex min-h-0 flex-col overflow-hidden bg-[#0e0c0a] text-[13px] text-[#cbc2b4]">
						{scanlines ? (
							<div
								className="pointer-events-none fixed inset-0 z-60 opacity-(--scan)"
								style={{
									backgroundImage:
										"repeating-linear-gradient(0deg, transparent, transparent 2px, rgba(0,0,0,0.18) 2px, rgba(0,0,0,0.18) 4px)",
								}}
								aria-hidden="true"
							/>
						) : null}
						<TerminalTopBar />
						<div className="flex min-h-0 flex-1">
							<TerminalNav active={surface} />
							<main className="min-w-0 flex-1 overflow-auto bg-[#0e0c0a]">
								<div className="h-full min-h-[720px]">{children}</div>
							</main>
						</div>
						<CommandPalette activeSurface={surface} onRun={runPalette} />
					</div>
				</ClientOnly>
				<Scripts />
			</body>
		</html>
	);
};

export const Route = createRootRoute({
	head: () => ({
		meta: [
			{ charSet: "utf-8" },
			{ name: "viewport", content: "width=device-width, initial-scale=1" },
			{ title: "symm" },
		],
		links: [
			{ rel: "stylesheet", href: appCss },
			{ rel: "preconnect", href: "https://fonts.googleapis.com" },
			{
				rel: "preconnect",
				href: "https://fonts.gstatic.com",
				crossOrigin: "anonymous",
			},
			{
				rel: "stylesheet",
				href: "https://fonts.googleapis.com/css2?family=Inter+Tight:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600;700&family=Source+Serif+4:opsz,wght@8..60,400;8..60,600&display=swap",
			},
		],
	}),
	shellComponent: RootDocument,
});
