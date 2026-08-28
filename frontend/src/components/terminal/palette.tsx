import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { appStore } from "#/collections/app";
import { type TerminalSurface, terminalStore } from "#/collections/terminal";
import { paletteGroupVariant } from "#/components/terminal/badge-tone";
import { Badge } from "@/components/ui/badge";
import { Icon, type IconName } from "@/components/ui/icon";
import { Input } from "@/components/ui/input";
import { List } from "@/components/ui/list";
import { Modal } from "@/components/ui/modal";
import { Typography } from "@/components/ui/typography";

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
	{
		id: "influence",
		label: "Influence field",
		hint: "Derived weighted force field · coefficient flow",
	},
	{
		id: "fluid",
		label: "Fluid manifold",
		hint: "Particle gas · gas volume · complex wave field",
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
	{
		id: "backtest",
		label: "Backtest",
		hint: "Captured sessions · play, pause, scrub",
	},
	{
		id: "regulator",
		label: "Global regulator",
		hint: "Predictive control · wallet return",
	},
	{
		id: "diagnostics",
		label: "System diagnostics",
		hint: "Stream lanes · commit lag · stalls",
	},
];

/*
GROUP_ICON names each command group's glyph. The palette used to carry four
inline SVGs at its own stroke weight; they are the shared set now, so a palette
row and a rail entry cannot drift apart.
*/
const GROUP_ICON: Record<PaletteCommand["group"], IconName> = {
	Surface: "grid",
	Kernel: "signal",
	Symbol: "target",
};

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
CommandPalette is the jump shell.

The symbol universe is whatever the engine has named this run, accumulated in
the app store as frames arrive. It used to be discovered into module-level
variables that no frame ever wrote, so the palette could only ever offer the
surfaces and kernels it had hard-coded.
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

	const symbolUniverse = app.symbols;

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
		<Modal
			variant="heavy"
			align="top"
			size="lg"
			className="z-40 p-0"
			panelClassName="max-h-[60vh] w-[560px] max-w-[calc(100%-48px)] rounded-lg shadow-[0_30px_70px_-18px_rgba(0,0,0,0.78)]"
			onClose={closePalette}
			closeLabel="Close command palette"
		>
			<Input.Search
				ref={inputRef}
				variant="bare"
				size="xl"
				iconSize="m"
				fieldClassName="shrink-0 border-(--line) border-b px-[15px] py-[13px]"
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
				trailing={
					<Typography.Mono size="s" tone="f4" className="shrink-0">
						{countText}
					</Typography.Mono>
				}
			/>

			<List className="gap-0.5 overflow-auto p-1.5">
				{commands.length === 0 ? (
					<List.Empty>{emptyCopy}</List.Empty>
				) : (
					commands.map((command, index) => (
						<List.Option
							key={command.key}
							selected={index === selectedIndex}
							active={command.active}
							onClick={() =>
								onRun(command.surface, command.source, command.symbol)
							}
							icon={<Icon name={GROUP_ICON[command.group]} size="s" />}
							label={command.label}
							hint={command.hint}
							trailing={
								<Badge
									label={command.group}
									variant={paletteGroupVariant(command.group)}
									size="xs"
									className="shrink-0"
								/>
							}
						/>
					))
				)}
			</List>
		</Modal>
	);
};
