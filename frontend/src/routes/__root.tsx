import {
	ClientOnly,
	createRootRoute,
	HeadContent,
	Scripts,
	useLocation,
	useNavigate,
} from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { appStore } from "#/collections/app";
import { type TerminalSurface, terminalStore } from "#/collections/terminal";
import { openInspectorShell } from "#/components/terminal/kernel-list";
import { CommandPalette } from "#/components/terminal/palette";
import { SymbolFocusLayer } from "#/components/terminal/symbol-focus";
import { TerminalNav } from "#/components/terminal/terminal-nav";
import { TerminalTopBar } from "#/components/terminal/terminal-top-bar";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { WsFeed } from "#/providers/websocket";
import appCss from "../app.css?url";

const SURFACE_VALUES = new Set<TerminalSurface>([
	"dashboard",
	"graph",
	"signals",
	"decisions",
	"journal",
	"xray",
	"cortex",
	"allocation",
]);

const SURFACE_ALIASES: Record<string, TerminalSurface> = {
	insight: "signals",
	decision: "decisions",
	alloc: "allocation",
	trade: "journal",
	graphs: "graph",
};

const SURFACE_PATHS: Record<TerminalSurface, string> = {
	dashboard: "/",
	graph: "/graph",
	signals: "/signals",
	decisions: "/decisions",
	journal: "/journal",
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
	const app = useSelector(appStore, (state) => state);
	const scanlines = useSelector(terminalStore, (state) => state.scanlines);
	const errorDialogRef = useRef<HTMLDialogElement>(null);
	const dismissRef = useRef<HTMLButtonElement>(null);
	const { openPalette, closePalette, bumpPaletteIndex, selectFocusSymbol } =
		terminalStore.actions;
	const { updateFocusSymbol } = appStore.actions;

	useEffect(() => {
		const dialog = errorDialogRef.current;

		if (dialog === null) {
			return;
		}

		if (app.error) {
			if (!dialog.open) {
				dialog.showModal();
			}

			dismissRef.current?.focus();

			return;
		}

		if (dialog.open) {
			dialog.close();
		}
	}, [app.error]);

	const runPalette = (
		nextSurface: TerminalSurface,
		source?: string,
		focusSymbol?: string,
	) => {
		if (source) {
			openInspectorShell(source);
		}

		if (focusSymbol) {
			updateFocusSymbol(focusSymbol);
			selectFocusSymbol(focusSymbol);
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

			if (event.key === "Escape" && appStore.state.error) {
				event.preventDefault();
				appStore.actions.clearError();
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
					<Flex.Column className="fixed inset-0 z-50 min-h-0 overflow-hidden bg-[#0e0c0a] text-[13px] text-[#cbc2b4]">
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
						<SymbolFocusLayer>
							<TerminalTopBar />
							<Flex.Row className="min-h-0 flex-1">
								<TerminalNav active={surface} />
								<main className="min-w-0 flex-1 overflow-auto bg-[#0e0c0a]">
									<div className="h-full min-h-180">{children}</div>
								</main>
							</Flex.Row>
						</SymbolFocusLayer>
						<CommandPalette activeSurface={surface} onRun={runPalette} />
						<dialog
							ref={errorDialogRef}
							aria-label="Backend error"
							className="fixed inset-0 z-80 m-0 h-full w-full max-h-none max-w-none border-0 bg-[rgba(8,6,5,0.82)] p-6 text-[#f1d7cf]"
							onCancel={(event) => {
								event.preventDefault();
								appStore.actions.clearError();
							}}
							onClick={(event) => {
								if (event.target === event.currentTarget) {
									appStore.actions.clearError();
								}
							}}
							onKeyDown={(event) => {
								if (
									(event.key === "Enter" || event.key === " ") &&
									event.target === event.currentTarget
								) {
									event.preventDefault();
									appStore.actions.clearError();
								}
							}}
						>
							{app.error ? (
								<Flex.Column
									className="mx-auto max-h-full w-full max-w-3xl border border-[#5f2d2d] bg-[#1a0f0e] shadow-[0_18px_80px_rgba(0,0,0,0.55)]"
									fullWidth
								>
									<Flex.Row
										justify="between"
										align="center"
										className="border-[#5f2d2d] border-b px-4 py-3"
										fullWidth
									>
										<span className="font-mono text-[11px] tracking-[0.14em] text-[#d56b61] uppercase">
											backend error
										</span>
										<button
											ref={dismissRef}
											type="button"
											className="font-mono text-[11px] text-[#f1d7cf] underline-offset-2 hover:underline"
											onClick={() => appStore.actions.clearError()}
										>
											dismiss
										</button>
									</Flex.Row>
									<Grid
										cols={2}
										className="min-h-0 flex-1 grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-2 overflow-auto p-4 font-mono text-[11px]"
									>
										{Object.entries(app.error).map(([key, value]) => (
											<Flex
												key={`${key}:${value === null ? "null" : typeof value === "object" ? JSON.stringify(value) : String(value)}`}
												className="contents"
											>
												<span className="text-[#d56b61]">{key}</span>
												<span className="min-w-0 wrap-break-word text-[#f1d7cf]">
													{value === null
														? "null"
														: typeof value === "object"
															? JSON.stringify(value)
															: String(value)}
												</span>
											</Flex>
										))}
									</Grid>
								</Flex.Column>
							) : null}
						</dialog>
					</Flex.Column>
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
