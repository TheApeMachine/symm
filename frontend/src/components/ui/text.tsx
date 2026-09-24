import type { ComponentProps, Ref } from "react";
import { cn } from "@/lib/utils";
import { type MeasureFormat, measure } from "./learning-format";

export type TextProps = Omit<ComponentProps<"span">, "children"> & {
	ref?: Ref<HTMLSpanElement>;
	/*
		The words themselves. A surface drawn from nodes has no way to write
		text between two tags, so the text a node carries is a value it is
		given, exactly like every other thing a node is given.
	*/
	value?: string;
	/*
		How a numeric value is written; the value itself is never altered. A
		value bound from the graph may arrive as a number.
	*/
	format?: MeasureFormat;
};

/*
Text is a run of words.

Every other component here is a shape that words go into. This one is the words,
which is what a graph-authored surface needs in order to label anything at all:
a tab, a legend, a line of prose. It renders a span and nothing else, so it
inherits whatever the thing around it decided about type.
*/
export const Text = ({
	ref,
	value,
	format,
	className,
	...props
}: TextProps) => (
	<span ref={ref} className={cn(className)} {...props}>
		{format === undefined ? value : measure(value, format)}
	</span>
);
