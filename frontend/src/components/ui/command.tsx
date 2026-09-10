"use client";

import {
	type ComponentProps,
	createContext,
	type ReactNode,
	useContext,
	useMemo,
	useState,
} from "react";
import { cn } from "@/lib/utils";
import { Input } from "./input";

/*
Command is a filterable list of things to pick: a palette, a context menu, an
autocomplete.

The filtering lives here rather than in each caller because the awkward part is
not the predicate — the caller supplies that — but keeping the list, the empty
state, and any groups agreeing on one filtered result. A group that filtered
its own rows would keep its heading after its last row disappeared, and an
empty state that counted the unfiltered list would never show.

So one filtered result is computed at the top and read from context by every
part. A group whose rows all fail the filter is dropped whole, and `mode`
decides whether there is a filter at all: a menu that is short enough to read
does not need a search box in front of it.

This is a primitive, not a palette. It brings no floating behaviour, no
keyboard trapping, and no opinion about where it is positioned — that belongs
to whatever opens it.
*/

/*
Grouped rows are recognized by shape rather than declared: an item that carries
its own items is a group of them.
*/
type Grouped = { items: readonly unknown[] };

const grouped = (item: unknown): item is Grouped =>
	typeof item === "object" &&
	item !== null &&
	Array.isArray((item as Grouped).items);

export type CommandFilter = (item: unknown, query: string) => boolean;

type CommandState = {
	query: string;
	setQuery: (query: string) => void;
	filtered: readonly unknown[];
	filtering: boolean;
};

const CommandContext = createContext<CommandState | null>(null);

/*
Collection is the scope a rendered row belongs to: the whole filtered result,
or one group's surviving rows.
*/
const CollectionContext = createContext<readonly unknown[] | null>(null);

const useCommand = (part: string): CommandState => {
	const state = useContext(CommandContext);

	if (!state) {
		throw new Error(`${part} must be rendered inside a Command`);
	}

	return state;
};

/*
useFilteredItems returns the rows the caller should render right now — a
group's rows inside a CommandGroup, and the whole filtered result outside one.
*/
export const useFilteredItems = <T,>(): T[] => {
	const scope = useContext(CollectionContext);
	const { filtered } = useCommand("useFilteredItems");

	return (scope ?? filtered) as T[];
};

/*
keep applies the filter to one row, descending into a group so the group
survives exactly as long as one of its rows does.
*/
const keep = (
	item: unknown,
	query: string,
	filter: CommandFilter,
): unknown | undefined => {
	if (!grouped(item)) {
		return filter(item, query) ? item : undefined;
	}

	const rows = item.items.filter((row) => filter(row, query));

	if (rows.length === 0) {
		return undefined;
	}

	return { ...item, items: rows };
};

export type CommandProps = Omit<ComponentProps<"div">, "children"> & {
	items: readonly unknown[];
	filter?: CommandFilter;
	mode?: "list" | "none";
	children: ReactNode;
};

export const Command = ({
	ref,
	items,
	filter,
	mode = "list",
	className,
	children,
	...props
}: CommandProps) => {
	const [query, setQuery] = useState("");
	const filtering = mode !== "none" && filter !== undefined;

	const filtered = useMemo(() => {
		if (!filtering || !filter || query.trim() === "") {
			return items;
		}

		return items
			.map((item) => keep(item, query, filter))
			.filter((item) => item !== undefined);
	}, [items, filter, filtering, query]);

	const state = useMemo(
		() => ({ query, setQuery, filtered, filtering }),
		[query, filtered, filtering],
	);

	return (
		<CommandContext.Provider value={state}>
			<div
				ref={ref}
				className={cn("flex min-h-0 min-w-0 flex-col", className)}
				{...props}
			>
				{children}
			</div>
		</CommandContext.Provider>
	);
};

/*
CommandPanel is the plate a Command is served on.
*/
export const CommandPanel = ({
	ref,
	className,
	...props
}: ComponentProps<"div">) => (
	<div
		ref={ref}
		className={cn(
			"flex min-h-0 min-w-0 flex-col overflow-hidden rounded-[4px]",
			/*
				A panel is often portaled to the document body, outside whatever
				set the page's foreground, so it states its own rather than
				inheriting the browser's black onto a dark plate.
			*/
			"border border-(--line2) bg-(--surface) text-(--f2)",
			"shadow-[0_12px_32px_-12px_rgb(0_0_0/0.8)]",
			className,
		)}
		{...props}
	/>
);

export const CommandInput = ({
	ref,
	className,
	...props
}: Omit<ComponentProps<typeof Input>, "value" | "onChange">) => {
	const { query, setQuery } = useCommand("CommandInput");

	return (
		<div className="shrink-0 border-(--line) border-b px-2.5 py-1.5">
			<Input
				ref={ref}
				value={query}
				onChange={(event) => setQuery(event.target.value)}
				className={cn("w-full", className)}
				{...props}
			/>
		</div>
	);
};

export const CommandList = ({
	ref,
	className,
	...props
}: ComponentProps<"div">) => (
	<div
		ref={ref}
		role="listbox"
		className={cn("min-h-0 min-w-0 overflow-y-auto p-1", className)}
		{...props}
	/>
);

export type CommandGroupProps = ComponentProps<"div"> & {
	items: readonly unknown[];
};

export const CommandGroup = ({
	ref,
	items,
	className,
	children,
	...props
}: CommandGroupProps) => (
	<CollectionContext.Provider value={items}>
		<div ref={ref} className={cn("min-w-0 py-0.5", className)} {...props}>
			{children}
		</div>
	</CollectionContext.Provider>
);

export const CommandGroupLabel = ({
	ref,
	className,
	...props
}: ComponentProps<"div">) => (
	<div
		ref={ref}
		className={cn(
			"px-2 py-1 text-[9px] text-(--f4) uppercase tracking-[0.14em]",
			className,
		)}
		{...props}
	/>
);

/*
CommandCollection renders the rows in scope. Its child is the renderer rather
than the rows, so a caller writes one row and the collection decides how many
of them there are.
*/
export type CommandCollectionProps<T> = {
	children: (item: T, index: number) => ReactNode;
};

export const CommandCollection = <T,>({
	children,
}: CommandCollectionProps<T>) => {
	const items = useFilteredItems<T>();

	return <>{items.map((item, index) => children(item, index))}</>;
};

export type CommandItemProps = Omit<ComponentProps<"button">, "value"> & {
	value?: unknown;
};

export const CommandItem = ({
	ref,
	value,
	className,
	children,
	...props
}: CommandItemProps) => (
	<button
		ref={ref}
		type="button"
		role="option"
		aria-selected={false}
		className={cn(
			"flex w-full min-w-0 items-center gap-2 rounded-[3px] px-2 py-1.5",
			"text-left text-[11px] text-(--f2) transition-colors",
			"hover:bg-(--raised) hover:text-(--f1)",
			"focus-visible:bg-(--raised) focus-visible:text-(--f1) focus-visible:outline-none",
			className,
		)}
		{...props}
	>
		{children}
	</button>
);

/*
CommandEmpty renders only when the filter left nothing, so a caller can state
the empty case beside the list rather than branching around it.
*/
export const CommandEmpty = ({
	ref,
	className,
	...props
}: ComponentProps<"div">) => {
	const { filtered } = useCommand("CommandEmpty");

	if (filtered.length > 0) {
		return null;
	}

	return (
		<div
			ref={ref}
			className={cn("px-2 py-3 text-center text-[11px] text-(--f4)", className)}
			{...props}
		/>
	);
};
