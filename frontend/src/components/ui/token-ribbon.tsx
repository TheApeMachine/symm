import { cn } from "#/lib/utils";
import { Badge } from "./badge";

export interface TokenRibbonProps {
	/* Each token is the list of sectors it is made of, oldest token first. */
	tokens: number[][];
	/* Slots the sequence fills before it can match. */
	capacity?: number;
	color: (sector: number) => string;
	label?: (sector: number) => string;
	className?: string;
}

/*
TokenRibbon draws a sequence as coloured chips filling a fixed row of slots;
unfilled slots are dashed, so how close the sequence is to complete is a
matter of counting empty boxes.
*/
export const TokenRibbon = ({
	tokens,
	capacity,
	color,
	label,
	className,
}: TokenRibbonProps) => {
	const slots = Math.max(capacity ?? 0, tokens.length);

	return (
		<div className={cn("flex items-center gap-1 flex-wrap", className)}>
			{Array.from({ length: slots }, (_, i) => {
				const token = tokens[i];
				const last = i === tokens.length - 1;
				return (
					<div key={`token-ribbon-${i}`} className="flex items-center gap-1">
						{token ? (
							<div
								className={cn(
									"flex rounded overflow-hidden border",
									last
										? "border-(--f1) shadow-[0_0_8px_var(--acc)]"
										: "border-(--line)",
								)}
							>
								{token.map((s, j) => (
									<Badge
										size="xxs"
										variant="info"
										key={`token-ribbon-badge-${i}-${j}`}
										label={label ? label(s) : s}
									/>
								))}
							</div>
						) : (
							<div className="w-8 h-5 rounded border border-dashed border-(--line)" />
						)}
						{i < slots - 1 && (
							<span className="text-(--f4) text-[10px]">›</span>
						)}
					</div>
				);
			})}
			{slots === 0 && (
				<span className="text-[10px] text-(--f4)">no tokens yet</span>
			)}
		</div>
	);
};
