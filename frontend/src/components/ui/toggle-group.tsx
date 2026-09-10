"use client";

import {
	type ComponentProps,
	createContext,
	type ReactNode,
	useContext,
} from "react";
import { cn } from "@/lib/utils";

/*
ToggleGroup is a row of mutually exclusive options, one of which is always on.

It is a segmented control rather than a set of buttons: the choice is the
state, so the group owns the value and each item only says which value it
stands for. Items render as radios in a radiogroup so arrow keys move between
them, which is what a keyboard user expects of a control where exactly one
option holds at a time.
*/

type ToggleGroupState<T extends string = string> = {
	value: T;
	onValueChange: (value: T) => void;
	name: string;
};

const ToggleGroupContext = createContext<ToggleGroupState | null>(null);

export type ToggleGroupProps<T extends string = string> = Omit<
	ComponentProps<"div">,
	"onChange" | "defaultValue"
> & {
	value: T;
	onValueChange: (value: T) => void;
	name: string;
	children: ReactNode;
};

export const ToggleGroup = <T extends string = string>({
	ref,
	value,
	onValueChange,
	name,
	className,
	children,
	...props
}: ToggleGroupProps<T>) => (
	<ToggleGroupContext.Provider
		value={{
			value,
			onValueChange: onValueChange as (next: string) => void,
			name,
		}}
	>
		<div
			ref={ref}
			role="radiogroup"
			className={cn(
				"inline-flex items-center gap-px rounded-[4px] border border-(--line) bg-(--sunken) p-px",
				className,
			)}
			{...props}
		>
			{children}
		</div>
	</ToggleGroupContext.Provider>
);

export type ToggleGroupItemProps = Omit<ComponentProps<"label">, "onChange"> & {
	value: string;
	children: ReactNode;
};

export const ToggleGroupItem = ({
	ref,
	value,
	className,
	children,
	...props
}: ToggleGroupItemProps) => {
	const group = useContext(ToggleGroupContext);

	if (!group) {
		throw new Error("ToggleGroupItem must be rendered inside a ToggleGroup");
	}

	const selected = group.value === value;

	return (
		<label
			ref={ref}
			className={cn(
				"relative inline-flex cursor-pointer select-none items-center gap-1.5",
				"rounded-[3px] px-2 py-1 text-[10.5px] transition-colors",
				"has-[:focus-visible]:ring-1 has-[:focus-visible]:ring-(--acc)",
				selected
					? "bg-(--raised) text-(--f1)"
					: "text-(--f4) hover:text-(--f2)",
				className,
			)}
			{...props}
		>
			<input
				type="radio"
				name={group.name}
				value={value}
				checked={selected}
				onChange={() => group.onValueChange(value)}
				className="absolute size-0 opacity-0"
			/>
			{children}
		</label>
	);
};
