import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";
import { registerPainter } from "#/providers/ws-stores";
import { Balance } from "@/components/balance";
import { Count } from "@/components/count";
import { Badge } from "@/components/ui/badge";
import { panelVariants } from "@/components/ui/panel";

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

export const TerminalTopBar = () => {
	const online = useSelector(appStore, (state) => state.online);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const { openPalette, openSymbolPalette } = terminalStore.actions;

	return (
		<header className="relative z-5 flex h-13 shrink-0 items-center gap-3.5 border-(--line) border-b bg-(--surface) px-4">
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
			<Count />
			<div className="ml-auto flex items-center gap-5.5">
				<button
					type="button"
					onClick={openSymbolPalette}
					data-symbol={focusSymbol}
					title="Search focused symbol"
					className="cursor-pointer"
				>
					<Badge
						label={focusSymbol}
						variant="warning"
						size="m"
						className="font-mono"
					/>
				</button>
				<button
					type="button"
					onClick={openPalette}
					className={cn(
						panelVariants({ size: "s" }),
						"flex cursor-pointer items-center gap-2 py-1.25 pr-2 pl-2.25 text-(--f3) hover:border-(--line2) hover:text-(--f1)",
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
					<span className="rounded-xs border border-(--line) px-1 font-mono text-[10px] text-(--f4)">
						⌘K
					</span>
				</button>
				<Balance />
				<Component register={(paint) => registerPainter("tick", paint)}>
					{({ ref }) => (
						<span ref={ref} className="font-mono text-[11px] text-(--f3)">
							tick <span data-paint="count" className="text-(--f1)" />
						</span>
					)}
				</Component>
			</div>
		</header>
	);
};
