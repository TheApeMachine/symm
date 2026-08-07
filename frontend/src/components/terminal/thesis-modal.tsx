import { useSelector } from "@tanstack/react-store";
import { terminalStore } from "#/collections/terminal";
import { ThesisDetailRail } from "#/components/terminal/thesis-detail-rail";
import { ThesisEvidenceCanvas } from "#/components/terminal/thesis-evidence-canvas";
import { Component } from "#/components/ui/component";
import { Typography } from "@/components/ui/typography";
import { cn } from "#/lib/utils";

/*
ThesisModal is the carrier for one symbol's thesis.

Open and close are store state, not a hidden attribute written by a paint pass:
the modal simply is not mounted when no symbol is selected, which is also what
keeps its canvas from holding a graph subscription while it is off screen.

Everything inside is bound straight to the frames that own it. The previous
shell merged nine wire keys into a snapshot before painting, and five of those
keys — forecasts, hypotheses, categories, findings, journal — are not published
by the backend at all, so the merge mostly propagated absence.
*/
export const openThesisShell = (symbol: string) => {
	terminalStore.actions.openThesis(symbol);
};

export const closeThesisShell = () => {
	terminalStore.actions.closeThesis();
};

export const ThesisModal = () => {
	const symbol = useSelector(terminalStore, (state) => state.thesisSymbol);

	if (symbol === null || symbol === "") {
		return null;
	}

	return (
		<div className="absolute inset-0 z-20 flex animate-[symFade_0.18s_ease] items-center justify-center bg-[color-mix(in_srgb,var(--sunken)_52%,transparent)] p-6 backdrop-blur-sm">
			<button
				type="button"
				aria-label="Close thesis modal"
				className="absolute inset-0"
				onClick={closeThesisShell}
			/>
			<div
				className={cn(
					"pointer-events-auto relative z-10 flex h-[min(88vh,920px)] w-[min(1180px,96vw)] flex-col overflow-hidden",
					"rounded-lg border border-(--line2) bg-(--surface) shadow-[0_28px_72px_-18px_rgba(0,0,0,0.78)]",
				)}
			>
				<div className="flex shrink-0 items-start justify-between gap-3 border-(--line) border-b px-5 py-4">
					<div className="min-w-0">
						<div className="flex flex-wrap items-center gap-2">
							<Typography.Display size="lg">{symbol}</Typography.Display>
							<Component registerKey="strategy" select="decisions">
								{({ ref }) => (
									<span
										ref={ref}
										data-scope="symbol"
										data-filter={symbol}
										className="contents"
									>
										<span
											data-paint="action"
											data-paint-class="enter:text-(--up) exit:text-(--down) hold:text-(--warn) nothing:text-(--f4)"
											className="rounded-full border border-(--line2) px-2 py-px font-mono text-[9px] uppercase"
										/>
									</span>
								)}
							</Component>
						</div>
						<Component registerKey="tick">
							{({ ref }) => (
								<div
									ref={ref}
									className="mt-1 font-mono text-[10px] text-(--f4)"
								>
									thesis carrier · tick{" "}
									<span data-paint="count" data-paint-absent="—" />
								</div>
							)}
						</Component>
					</div>
					<button
						type="button"
						onClick={closeThesisShell}
						className="flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-[3px] border border-(--line) bg-(--raised) p-0 text-(--f3) hover:border-(--line2) hover:text-(--f1)"
					>
						<svg
							width="13"
							height="13"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							strokeWidth="2"
							aria-hidden="true"
						>
							<title>Close</title>
							<path d="M6 6l12 12M18 6L6 18" />
						</svg>
					</button>
				</div>

				<div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1.55fr)_minmax(280px,360px)]">
					<div className="relative min-h-0 border-(--line) border-r">
						<ThesisEvidenceCanvas symbol={symbol} />
						<div className="pointer-events-none absolute top-3.5 left-4">
							<div className="font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">
								Evidence graph
							</div>
							<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
								measurement nodes · typed Gonum relationships
							</div>
						</div>
					</div>
					<div className="min-h-0 overflow-y-auto p-3.5">
						<ThesisDetailRail symbol={symbol} />
					</div>
				</div>
			</div>
		</div>
	);
};
