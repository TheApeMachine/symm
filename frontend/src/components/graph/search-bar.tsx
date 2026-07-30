"use client";

import { SearchIcon, XIcon } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Input } from "#/components/ui/input";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import type { Graph } from "./core/graph";
import { searchNodes } from "./graph-helpers";

/*
SearchBar lets the user type to find nodes in the active graph by tail
name or full path. Matches are ranked (see searchNodes) and clicking
one fires onPick, which the inspector turns into selection + camera
focus.
*/
export const NodeSearchBar = ({
	graph,
	onPick,
}: {
	graph: Graph | undefined;
	onPick: (name: string) => void;
}) => {
	const [query, setQuery] = useState("");
	const [open, setOpen] = useState(false);
	const inputRef = useRef<HTMLInputElement | null>(null);
	const wrapperRef = useRef<HTMLDivElement | null>(null);

	const matches = useMemo(
		() => (graph ? searchNodes(graph, query, 20) : []),
		[graph, query],
	);

	useEffect(() => {
		if (!open) return;

		const handler = (event: MouseEvent) => {
			if (!wrapperRef.current?.contains(event.target as Node)) {
				setOpen(false);
			}
		};

		document.addEventListener("mousedown", handler);
		return () => document.removeEventListener("mousedown", handler);
	}, [open]);

	const choose = (name: string) => {
		onPick(name);
		setQuery("");
		setOpen(false);
	};

	return (
		<div className="relative w-full max-w-sm" ref={wrapperRef}>
			<Flex.Row className="items-center gap-1.5 rounded-md border border-border bg-background px-2">
				<SearchIcon
					aria-hidden
					className="size-3.5 shrink-0 text-muted-foreground"
				/>
				<Input
					className="h-7 border-0 bg-transparent px-0 shadow-none focus-visible:ring-0"
					onChange={(event) => {
						setQuery(event.target.value);
						setOpen(true);
					}}
					onFocus={() => setOpen(true)}
					onKeyDown={(event) => {
						if (event.key === "Enter" && matches[0]) choose(matches[0]);
						if (event.key === "Escape") {
							setQuery("");
							setOpen(false);
						}
					}}
					placeholder="Search nodes…"
					ref={inputRef}
					value={query}
				/>
				{query.length > 0 ? (
					<Button
						aria-label="Clear search"
						className="size-5 shrink-0"
						onClick={() => {
							setQuery("");
							setOpen(false);
							inputRef.current?.focus();
						}}
						size="icon-xs"
						type="button"
						variant="ghost"
					>
						<XIcon className="size-3" />
					</Button>
				) : null}
			</Flex.Row>

			{open && query.length > 0 ? (
				<div className="absolute left-0 top-full z-50 mt-1 max-h-72 w-full overflow-auto rounded-md border border-border bg-popover py-1 shadow-lg">
					{matches.length === 0 ? (
						<Typography.Span
							className="block px-3 py-2 text-xs"
							variant="muted"
						>
							No matches.
						</Typography.Span>
					) : (
						matches.map((name) => {
							const tail = name.split(".").at(-1) ?? name;

							return (
								<button
									className={cn(
										"flex w-full items-baseline justify-between gap-3 px-3 py-1.5 text-left text-xs hover:bg-accent",
									)}
									key={name}
									onClick={() => choose(name)}
									type="button"
								>
									<Typography.Span className="truncate font-mono">
										{tail}
									</Typography.Span>
									<Typography.Span
										className="truncate font-mono text-[10px]"
										variant="muted"
									>
										{name}
									</Typography.Span>
								</button>
							);
						})
					)}
				</div>
			) : null}
		</div>
	);
};
