import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { appStore } from "#/collections/app";
import type { Instrument, Measurement } from "#/collections/types";
import { type TerminalSurface, terminalStore } from "#/collections/terminal";
import { paletteGroupVariant } from "#/components/terminal/badge-tone";
import { Badge } from "@/components/ui/badge";

const SURFACES: Array<{ id: TerminalSurface; label: string; hint: string }> = [
	{
		id: "dashboard",
		label: "Dashboard",
		hint: "Pilot-wave field · live decisions",
	},
	{
		id: "graph",
		label: "Market graph",
		hint: "Relational topology · node inspection",
	},
	{ id: "signals", label: "Signal insight", hint: "Per-kernel forensics" },
	{ id: "decisions", label: "Decision tree", hint: "Gate-by-gate trace" },
	{
		id: "journal",
		label: "Trade journal",
		hint: "Lifecycle · observations · findings",
	},
	{ id: "xray", label: "Latent x-ray", hint: "State-space cross-section" },
	{ id: "cortex", label: "Cognitive tree", hint: "Reasoning graph" },
	{ id: "allocation", label: "Allocation", hint: "Capital & exposure" },
];

let lastInstrumentSymbols: string[] = [];
let lastMeasuredSymbols: string[] = [];

const asRows = <T,>(value: unknown): T[] =>
	(Array.isArray(value) ? value : value != null ? [value] : []) as T[];

const sameSymbols = (left: string[], right: string[]): boolean =>
	left.length === right.length &&
	left.every((symbol, index) => symbol === right[index]);

/*
paintPaletteInstruments discovers instrument symbols from the current DRAW batch.
*/
export const paintPaletteInstruments = (
	value: unknown,
	_focusSymbol: string,
) => {
	const next = asRows<Instrument>(value)
		.map((instrument) => instrument.symbol)
		.sort();

	if (sameSymbols(lastInstrumentSymbols, next)) {
		return;
	}

	lastInstrumentSymbols = next;
};

/*
paintPaletteMeasurements discovers measured symbols from the current DRAW batch.
*/
export const paintPaletteMeasurements = (
	value: unknown,
	_focusSymbol: string,
) => {
	const next = [
		...new Set(
			asRows<Measurement>(value).map((measurement) => measurement.symbol),
		),
	].sort();

	if (sameSymbols(lastMeasuredSymbols, next)) {
		return;
	}

	lastMeasuredSymbols = next;
};

const SearchIcon = () => (
	<svg
		width="16"
		height="16"
		viewBox="0 0 24 24"
		fill="none"
		stroke="var(--f4)"
		strokeWidth="1.7"
		className="shrink-0"
		aria-hidden="true"
	>
		<circle cx="11" cy="11" r="7" />
		<path d="M21 21l-4.3-4.3" />
	</svg>
);

const SurfaceIcon = ({ active }: { active: boolean }) => (
	<svg
		width="13"
		height="13"
		viewBox="0 0 24 24"
		fill="none"
		stroke={active ? "var(--acc)" : "currentColor"}
		strokeWidth="1.7"
		aria-hidden="true"
	>
		<rect x="3" y="3" width="7" height="7" />
		<rect x="14" y="3" width="7" height="7" />
		<rect x="3" y="14" width="7" height="7" />
		<rect x="14" y="14" width="7" height="7" />
	</svg>
);

const KernelIcon = () => (
	<svg
		width="13"
		height="13"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		strokeWidth="1.7"
		aria-hidden="true"
	>
		<path d="M3 12h4l2 7 4-16 2 9h6" />
	</svg>
);

const SymbolIcon = () => (
	<svg
		width="13"
		height="13"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		strokeWidth="1.7"
		aria-hidden="true"
	>
		<path d="M5 12h14" />
		<path d="M12 5v14" />
		<circle cx="12" cy="12" r="7" />
	</svg>
);

type PaletteCommand = {
	key: string;
	label: string;
	hint: string;
	group: "Surface" | "Kernel" | "Symbol";
	surface: TerminalSurface;
	source?: string;
	symbol?: string;
	active: boolean;
};

/*
CommandPalette is the static jump shell. DRAW discovers symbols via
paintPaletteInstruments and paintPaletteMeasurements.
*/
export const CommandPalette = ({
	activeSurface,
	onRun,
}: {
	activeSurface: TerminalSurface;
	onRun: (surface: TerminalSurface, source?: string, symbol?: string) => void;
}) => {
	const app = useSelector(appStore, (state) => state);
	const terminal = useSelector(terminalStore, (state) => state);
	const { closePalette, setPaletteQuery } = terminalStore.actions;
	const inputRef = useRef<HTMLInputElement | null>(null);

	useEffect(() => {
		if (!terminal.paletteOpen) {
			return;
		}

		inputRef.current?.focus();
	}, [terminal.paletteOpen]);

	if (!terminal.paletteOpen) {
		return null;
	}

	const symbolUniverse = [
		...new Set([...lastInstrumentSymbols, ...lastMeasuredSymbols]),
	].sort();

	const commands: PaletteCommand[] = [
		...SURFACES.map(
			(surface): PaletteCommand => ({
				key: `surface:${surface.id}`,
				label: surface.label,
				hint: surface.hint,
				group: "Surface",
				surface: surface.id,
				active: surface.id === activeSurface,
			}),
		),
		...app.kernels.map(
			(kernel): PaletteCommand => ({
				key: `kernel:${kernel}`,
				label: `Inspect · ${kernel}`,
				hint: kernel,
				group: "Kernel",
				surface: "dashboard" as TerminalSurface,
				source: kernel,
				active: false,
			}),
		),
		...symbolUniverse.map(
			(symbol): PaletteCommand => ({
				key: `symbol:${symbol}`,
				label: symbol,
				hint: "Focus symbol",
				group: "Symbol",
				surface: activeSurface,
				symbol,
				active: symbol === app.focusSymbol,
			}),
		),
	].filter((command) => {
		if (terminal.paletteMode === "symbols" && command.group !== "Symbol") {
			return false;
		}

		return `${command.label} ${command.hint}`
			.toLowerCase()
			.includes(terminal.paletteQuery.trim().toLowerCase());
	});
	const selectedIndex =
		commands.length === 0
			? 0
			: ((terminal.paletteIndex % commands.length) + commands.length) %
				commands.length;
	const countText = `${commands.length} command${commands.length === 1 ? "" : "s"}`;
	const emptyCopy =
		terminal.paletteMode === "symbols"
			? symbolUniverse.length === 0
				? "No symbols yet. Waiting for instrument or measurement frames."
				: "No match. Try a symbol like BTC/USD."
			: "No match. Try a surface, kernel, or symbol name.";

	return (
		<div className="absolute inset-0 z-40 animate-[symFade_.16s_ease] [background:color-mix(in_srgb,var(--sunken)_64%,transparent)] backdrop-blur-[3px]">
			<button
				type="button"
				aria-label="Close command palette"
				className="absolute inset-0"
				onClick={closePalette}
			/>
			<div className="pointer-events-none relative z-10 flex items-start justify-center pt-24">
				<div className="pointer-events-auto flex max-h-[60vh] w-[560px] max-w-[calc(100%-48px)] flex-col overflow-hidden rounded-lg border border-(--line2) bg-(--surface) shadow-[0_30px_70px_-18px_rgba(0,0,0,0.78)]">
					<div className="flex items-center gap-2.5 border-(--line) border-b px-[15px] py-[13px]">
						<SearchIcon />
						<input
							ref={inputRef}
							value={terminal.paletteQuery}
							onChange={(event) => setPaletteQuery(event.target.value)}
							onKeyDown={(event) => {
								if (event.key !== "Enter") {
									return;
								}

								const command = commands[selectedIndex];

								if (command === undefined) {
									return;
								}

								event.preventDefault();
								onRun(command.surface, command.source, command.symbol);
							}}
							placeholder={
								terminal.paletteMode === "symbols"
									? "Search symbols…"
									: "Jump to a surface, kernel, or symbol…"
							}
							spellCheck={false}
							className="min-w-0 flex-1 bg-transparent text-[15px] text-(--f1) outline-none"
						/>
						<span className="shrink-0 font-mono text-[10px] text-(--f4)">
							{countText}
						</span>
					</div>
					<div className="flex min-h-0 flex-col gap-0.5 overflow-auto p-1.5">
						{commands.length === 0 ? (
							<div className="px-3.5 py-[26px] text-center font-mono text-[12px] text-(--f4)">
								{emptyCopy}
							</div>
						) : (
							commands.map((command, index) => {
								const selected = index === selectedIndex;
								const tagVariant = paletteGroupVariant(command.group);

								return (
									<button
										key={command.key}
										type="button"
										onClick={() =>
											onRun(
												command.surface,
												command.source,
												command.symbol,
											)
										}
										className="flex cursor-pointer items-center gap-2.5 rounded-[4px] border px-3 py-2.5 text-left hover:bg-(--raised)"
										style={{
											borderColor: selected ? "var(--acc)" : "transparent",
											background: selected
												? "color-mix(in srgb, var(--acc) 10%, transparent)"
												: "transparent",
										}}
									>
										{command.group === "Surface" ? (
											<SurfaceIcon active={command.active} />
										) : command.group === "Symbol" ? (
											<SymbolIcon />
										) : (
											<KernelIcon />
										)}
										<div className="min-w-0 flex-1">
											<div className="truncate font-medium text-[13px] text-(--f1)">
												{command.label}
											</div>
											<div className="truncate font-mono text-[10px] text-(--f4)">
												{command.hint}
											</div>
										</div>
										<Badge
											label={command.group}
											variant={tagVariant}
											size="xs"
											className="shrink-0"
										/>
									</button>
								);
							})
						)}
					</div>
				</div>
			</div>
		</div>
	);
};
