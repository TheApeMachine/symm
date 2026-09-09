"use client";

import { Autocomplete } from "@base-ui/react/autocomplete";
import React from "react";
import type { SelectOption } from "#/components/flume/types";
import {
	Command,
	CommandCollection,
	CommandEmpty,
	CommandGroup,
	CommandGroupLabel,
	CommandInput,
	CommandItem,
	CommandList,
	CommandPanel,
} from "#/components/ui/command";
import { cn } from "@/lib/utils";

interface ContextMenuProps {
	x: number;
	y: number;
	options: SelectOption[];
	onRequestClose: () => void;
	onOptionSelected: (option: SelectOption) => void;
	label?: string;
	hideHeader?: boolean;
	hideFilter?: boolean;
	emptyText?: string;
}

function buildCommandRootItems(options: SelectOption[]): {
	rootItems:
		| SelectOption[]
		| Array<{ label: string; items: readonly SelectOption[] }>;
	grouped: boolean;
} {
	if (options.length === 0) {
		return { rootItems: [], grouped: false };
	}

	const hasCategory = options.some((o) => Boolean(o.category?.trim()));
	if (!hasCategory) {
		return { rootItems: options, grouped: false };
	}

	const byCat = new Map<string, SelectOption[]>();
	for (const opt of options) {
		const catLabel = opt.category?.trim() || "Other";
		const bucket = byCat.get(catLabel) ?? [];
		bucket.push(opt);
		byCat.set(catLabel, bucket);
	}

	const labels = [...byCat.keys()].sort((a, b) => {
		if (a === "Other") return 1;
		if (b === "Other") return -1;
		return a.localeCompare(b);
	});

	const groups = labels.map((label) => ({
		label,
		items: byCat.get(label) as readonly SelectOption[],
	}));

	return { rootItems: groups, grouped: true };
}

const filterOption = (item: SelectOption, query: string): boolean => {
	const q = query.trim().toLowerCase();
	if (!q) return true;
	return (
		item.label.toLowerCase().includes(q) ||
		(item.description?.toLowerCase().includes(q) ?? false)
	);
};

type FilteredGroup = { label: string; items: readonly SelectOption[] };

function ContextMenuFilteredBody({
	grouped,
	onPick,
	optionsLength,
	emptyText,
}: {
	grouped: boolean;
	onPick: (option: SelectOption) => void;
	optionsLength: number;
	emptyText: string;
}) {
	const filteredItems = Autocomplete.useFilteredItems();

	const renderItem = (item: SelectOption) => (
		<CommandItem key={item.value} value={item} onClick={() => onPick(item)}>
			<div className="flex min-w-0 flex-col gap-0.5 text-left">
				<span>{item.label}</span>
				{item.description ? (
					<span className="text-muted-foreground text-xs">
						{item.description}
					</span>
				) : null}
			</div>
		</CommandItem>
	);

	return (
		<div>
			{grouped ? (
				(filteredItems as FilteredGroup[]).map((group) => (
					<CommandGroup items={[...group.items]} key={group.label}>
						<CommandGroupLabel>{group.label}</CommandGroupLabel>
						<CommandCollection>{renderItem}</CommandCollection>
					</CommandGroup>
				))
			) : (
				<CommandCollection>{renderItem}</CommandCollection>
			)}
			<CommandEmpty data-flume-component="ctx-menu-empty">
				{optionsLength === 0 ? emptyText : "No matching options."}
			</CommandEmpty>
		</div>
	);
}

const ContextMenu = ({
	x,
	y,
	options = [],
	onRequestClose,
	onOptionSelected,
	label,
	hideHeader,
	hideFilter,
	emptyText,
}: ContextMenuProps) => {
	const menuWrapper = React.useRef<HTMLDivElement>(null);
	const { rootItems, grouped } = React.useMemo(
		() => buildCommandRootItems(options),
		[options],
	);

	const showFilter = !hideFilter && options.length > 0;
	const commandMode = showFilter ? "list" : "none";

	const emptyMessage = emptyText ?? "No options.";

	const handleOptionSelected = (option: SelectOption) => {
		onOptionSelected(option);
		onRequestClose();
	};

	const onRequestCloseRef = React.useRef(onRequestClose);
	React.useEffect(() => {
		onRequestCloseRef.current = onRequestClose;
	});

	React.useEffect(() => {
		let cancelled = false;

		function teardownListeners() {
			document.removeEventListener("click", onOutsidePointer, {
				capture: true,
			});
			document.removeEventListener("contextmenu", onOutsidePointer, {
				capture: true,
			});
			document.removeEventListener("keydown", onEscape, {
				capture: true,
			});
		}

		function onOutsidePointer(event: Event) {
			const e = event as MouseEvent;
			if (
				menuWrapper.current &&
				!menuWrapper.current.contains(e.target as Element)
			) {
				teardownListeners();
				onRequestCloseRef.current();
			}
		}

		function onEscape(event: Event) {
			const e = event as KeyboardEvent;
			if (e.key === "Escape" || e.key === "Esc" || e.keyCode === 27) {
				teardownListeners();
				onRequestCloseRef.current();
			}
		}

		const timeoutId = window.setTimeout(() => {
			if (cancelled) {
				return;
			}
			document.addEventListener("keydown", onEscape, {
				capture: true,
			});
			document.addEventListener("click", onOutsidePointer, {
				capture: true,
			});
			document.addEventListener("contextmenu", onOutsidePointer, {
				capture: true,
			});
		}, 0);

		return () => {
			cancelled = true;
			window.clearTimeout(timeoutId);
			teardownListeners();
		};
	}, []);

	React.useEffect(() => {
		if (!showFilter && hideHeader) {
			menuWrapper.current?.focus({ preventScroll: true });
		}
	}, [hideHeader, showFilter]);

	const showHeader =
		!hideHeader && (Boolean(label?.trim()) || options.length > 0);

	const maxListHeight = Math.min(
		300,
		Math.max(48, window.innerHeight - y - 72),
	);

	return (
		<div
			data-flume-component="ctx-menu"
			className={cn(
				"fixed z-9999 w-[min(calc(100vw-24px),320px)] max-w-[90vw] outline-none",
			)}
			style={{
				left: x,
				top: y,
			}}
			ref={menuWrapper}
			tabIndex={!showFilter && hideHeader ? 0 : -1}
		>
			<CommandPanel className="flex max-h-[min(420px,calc(100vh-48px))] flex-col overflow-hidden rounded-xl shadow-lg">
				{showHeader ? (
					<div
						className="border-border border-b px-3 py-2"
						data-flume-component="ctx-menu-header"
					>
						{label ? (
							<span
								className="text-base font-semibold leading-tight"
								data-flume-component="ctx-menu-title"
							>
								{label}
							</span>
						) : null}
					</div>
				) : null}
				<Command
					filter={
						showFilter
							? (item, query) => filterOption(item as SelectOption, query)
							: undefined
					}
					items={rootItems}
					mode={commandMode}
				>
					{showFilter ? (
						<CommandInput
							aria-label="Filter options"
							data-flume-component="ctx-menu-input"
							placeholder="Filter options"
						/>
					) : null}
					<CommandList
						className="min-h-0 flex-1 overflow-y-auto"
						style={{ maxHeight: maxListHeight }}
					>
						<ContextMenuFilteredBody
							emptyText={emptyMessage}
							grouped={grouped}
							onPick={handleOptionSelected}
							optionsLength={options.length}
						/>
					</CommandList>
				</Command>
			</CommandPanel>
		</div>
	);
};

export default ContextMenu;
