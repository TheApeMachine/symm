import { useNavigate, useSearch } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { type CSSProperties, useEffect } from "react";
import { terminalStore } from "#/collections/terminal";
import type { TerminalSurface } from "#/components/terminal/model";
import { CommandPalette } from "#/components/terminal/palette";
import { TerminalNav, TerminalTopBar } from "#/components/terminal/panels";
import { SurfaceBody } from "#/components/terminal/surfaces";

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

export const parseSurface = (value: unknown): TerminalSurface => {
	if (typeof value !== "string") {
		return "dashboard";
	}

	const normalized = SURFACE_ALIASES[value] ?? value;

	if (SURFACE_VALUES.has(normalized as TerminalSurface)) {
		return normalized as TerminalSurface;
	}

	return "dashboard";
};

export const SymmTerminal = () => {
	const navigate = useNavigate({ from: "/" });
	const search = useSearch({ strict: false }) as { surface?: string };
	const surface = parseSurface(search.surface);
	const scanlines = useSelector(terminalStore, (state) => state.scanlines);
	const { inspectSource, openPalette, closePalette, bumpPaletteIndex } =
		terminalStore.actions;

	const selectSurface = (nextSurface: TerminalSurface) => {
		navigate({ search: { surface: nextSurface } });
	};

	const runPalette = (nextSurface: TerminalSurface, source?: string) => {
		if (source) {
			inspectSource(source);
		}

		closePalette();
		navigate({ search: { surface: nextSurface } });
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
		<div
			className="fixed inset-0 z-50 flex min-h-0 flex-col overflow-hidden bg-[#0e0c0a] text-[13px] text-[#cbc2b4]"
			style={terminalVars(scanlines)}
		>
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
			<style>{`
				@keyframes symFade {
					from { opacity: 0; transform: translateY(3px); }
					to { opacity: 1; transform: none; }
				}
			`}</style>
			<TerminalTopBar />
			<div className="flex min-h-0 flex-1">
				<TerminalNav active={surface} onSelect={selectSurface} />
				<main className="min-w-0 flex-1 overflow-auto bg-[#0e0c0a]">
					<div className="h-full min-h-[720px]">
						<SurfaceBody surface={surface} />
					</div>
				</main>
			</div>
			<CommandPalette activeSurface={surface} onRun={runPalette} />
		</div>
	);
};

const terminalVars = (scanlines: boolean): CSSProperties =>
	({
		"--acc": "#e8a33d",
		"--scan": scanlines ? "0.5" : "0",
		"--up": "#9cc06e",
		"--down": "#d5786a",
		"--info": "#7fbacb",
		"--warn": "#e8a33d",
		"--bg": "#0e0c0a",
		"--surface": "#17140f",
		"--raised": "#1f1a14",
		"--sunken": "#0a0907",
		"--line": "#2b251e",
		"--line2": "#3a342b",
		"--f1": "#f4efe5",
		"--f2": "#cbc2b4",
		"--f3": "#938a7e",
		"--f4": "#5f584e",
		fontFamily: '"Inter Tight", Inter, system-ui, sans-serif',
	}) as CSSProperties;
