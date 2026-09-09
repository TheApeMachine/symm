import { ChevronDown, XIcon } from "lucide-react";
import React from "react";
import { createPortal } from "react-dom";
import type { SelectOption } from "#/components/flume/types";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { cn } from "@/lib/utils";
import ContextMenu from "../ContextMenu/ContextMenu";

const MAX_LABEL_LENGTH = 50;

interface SelectProps {
	allowMultiple?: boolean;
	data: string | string[];
	onChange: (data: string | string[]) => void;
	options: SelectOption[];
	placeholder?: string;
}

const Select = ({
	options = [],
	placeholder = "[Select an option]",
	onChange,
	data,
	allowMultiple = false,
}: SelectProps) => {
	const [drawerOpen, setDrawerOpen] = React.useState(false);
	const [drawerCoordinates, setDrawerCoordinates] = React.useState({
		x: 0,
		y: 0,
	});
	const wrapper = React.useRef<HTMLButtonElement>(null);

	const closeDrawer = () => {
		setDrawerOpen(false);
	};

	const openDrawer = () => {
		if (!drawerOpen) {
			const wrapperRect = wrapper.current?.getBoundingClientRect();
			if (wrapperRect) {
				setDrawerCoordinates({
					x: wrapperRect.x,
					y: wrapperRect.y + wrapperRect.height,
				});
				setDrawerOpen(true);
			}
		}
	};

	const handleOptionSelected = (option: SelectOption) => {
		if (allowMultiple && Array.isArray(data)) {
			onChange([...data, option.value]);
		} else {
			onChange(option.value);
		}
	};

	const handleOptionDeleted = (optionIndex: number) => {
		if (!Array.isArray(data)) return;
		onChange([...data.slice(0, optionIndex), ...data.slice(optionIndex + 1)]);
	};

	const getFilteredOptions = () =>
		allowMultiple
			? options.filter((opt) => !data.includes(opt.value))
			: options;

	const selectedOption = React.useMemo(() => {
		if (typeof data !== "string") return undefined;
		const option = options.find((o) => o.value === data);
		if (option) {
			return {
				...option,
				label:
					option.label.length > MAX_LABEL_LENGTH
						? `${option.label.slice(0, MAX_LABEL_LENGTH)}...`
						: option.label,
			};
		}

		return undefined;
	}, [options, data]);

	const portalContainer =
		typeof document !== "undefined" ? document.body : null;

	const triggerButtonClass =
		"w-full justify-between gap-2 font-normal text-left";

	return (
		<div className="flex w-full min-w-0 flex-col">
			{allowMultiple && typeof data !== "string" ? (
				data.length ? (
					<div className="mb-2 flex flex-col gap-1">
						{data.map((val, i) => {
							const optLabel =
								options.find((opt) => opt.value === val)?.label ?? "";
							return (
								<OptionChip
									key={val}
									onRequestDelete={() => handleOptionDeleted(i)}
								>
									{optLabel}
								</OptionChip>
							);
						})}
					</div>
				) : null
			) : data ? (
				<SelectedOption
					wrapperRef={wrapper}
					option={selectedOption}
					onClick={openDrawer}
					expanded={drawerOpen}
				/>
			) : null}
			{(allowMultiple || !data) && (
				<Button
					ref={wrapper}
					variant="outline"
					type="button"
					size="sm"
					onClick={openDrawer}
					aria-haspopup="listbox"
					aria-expanded={drawerOpen}
					aria-label={placeholder}
					className={triggerButtonClass}
				>
					<span className="min-w-0 flex-1 truncate">{placeholder}</span>
					<ChevronDown className="size-4 shrink-0 opacity-80" aria-hidden />
				</Button>
			)}
			{drawerOpen && portalContainer
				? createPortal(
						<ContextMenu
							x={drawerCoordinates.x}
							y={drawerCoordinates.y}
							emptyText="There are no options"
							options={getFilteredOptions()}
							onOptionSelected={handleOptionSelected}
							onRequestClose={closeDrawer}
						/>,
						portalContainer,
					)
				: null}
		</div>
	);
};

export default Select;

interface SelectedOptionProps {
	option?: SelectOption;
	wrapperRef: React.RefObject<HTMLButtonElement | null>;
	onClick: React.MouseEventHandler;
	expanded: boolean;
}

const SelectedOption = ({
	option: { label, description } = {
		label: "",
		description: "",
		value: "",
	},
	wrapperRef,
	onClick,
	expanded,
}: SelectedOptionProps) => (
	<Button
		ref={wrapperRef}
		className={cn(
			"mb-1 h-auto w-full shrink-0 items-start gap-3 px-3 py-2 font-normal whitespace-normal",
		)}
		data-flume-component="select"
		type="button"
		variant="outline"
		size="sm"
		onClick={onClick}
		aria-haspopup="listbox"
		aria-expanded={expanded}
	>
		<div className="flex min-w-0 flex-1 flex-col items-start gap-0.5 text-left">
			<span className="w-full truncate" data-flume-component="select-label">
				{label}
			</span>
			{description ? (
				<span
					className="w-full text-left text-muted-foreground text-xs italic"
					data-flume-component="select-desc"
				>
					{description}
				</span>
			) : null}
		</div>
		<ChevronDown className="mt-1 size-4 shrink-0 opacity-80" aria-hidden />
	</Button>
);

interface OptionChipProps {
	children: React.ReactNode;
	onRequestDelete: () => void;
}

const OptionChip = ({ children, onRequestDelete }: OptionChipProps) => (
	<div className="flex w-full min-w-0 items-center gap-2">
		<Badge
			variant="outline"
			size="lg"
			className="min-w-0 flex-1 justify-start gap-2 font-normal"
		>
			<span className="truncate">{children}</span>
		</Badge>
		<Button
			className="size-8 shrink-0 text-muted-foreground hover:text-destructive-foreground"
			type="button"
			size="icon-sm"
			variant="ghost"
			aria-label="Remove"
			onMouseDown={(e) => {
				e.stopPropagation();
			}}
			onClick={onRequestDelete}
		>
			<XIcon className="size-4" />
		</Button>
	</div>
);
