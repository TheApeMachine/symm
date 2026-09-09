import React from "react";
import { Colors } from "#/components/flume/typeBuilders";
import type { Colors as ColorsType } from "#/components/flume/types";
import { Button } from "#/components/ui/button";
import { cn } from "@/lib/utils";

interface ColorPickerProps {
	x: number;
	y: number;
	onColorPicked: (color: ColorsType) => void;
	onRequestClose: () => void;
}

const SWATCH_HEX: Record<ColorsType, string> = {
	grey: "rgb(204, 204, 204)",
	red: "rgb(210, 101, 111)",
	purple: "rgb(159, 101, 210)",
	blue: "rgb(101, 151, 210)",
	green: "rgb(101, 210, 168)",
	orange: "rgb(210, 137, 101)",
	yellow: "rgb(210, 196, 101)",
	pink: "rgb(241, 124, 226)",
};

const ColorPicker = ({
	x,
	y,
	onColorPicked,
	onRequestClose,
}: ColorPickerProps) => {
	const wrapper = React.useRef<HTMLDivElement>(null);

	const testClickOutside = React.useCallback(
		(e: MouseEvent) => {
			if (wrapper.current && !wrapper.current.contains(e.target as Node)) {
				onRequestClose();
				document.removeEventListener("click", testClickOutside);
				document.removeEventListener("contextmenu", testClickOutside);
			}
		},
		[onRequestClose],
	);

	const testEscape = React.useCallback(
		(e: KeyboardEvent) => {
			if (e.key === "Escape") {
				onRequestClose();
				document.removeEventListener("keydown", testEscape);
			}
		},
		[onRequestClose],
	);

	React.useEffect(() => {
		setTimeout(() => {
			document.addEventListener("keydown", testEscape);
			document.addEventListener("click", testClickOutside);
			document.addEventListener("contextmenu", testClickOutside);
		});
		return () => {
			document.removeEventListener("click", testClickOutside);
			document.removeEventListener("contextmenu", testClickOutside);
			document.removeEventListener("keydown", testEscape);
		};
	}, [testClickOutside, testEscape]);

	return (
		<div
			data-flume-component="color-picker"
			ref={wrapper}
			className={cn(
				"fixed z-9999 flex w-[102px] flex-wrap gap-0.5 rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-lg backdrop-blur-sm",
			)}
			style={{
				left: x,
				top: y,
			}}
		>
			{Object.values(Colors).map((colorString) => {
				const color = colorString as ColorsType;
				return (
					<ColorButton
						onSelected={() => {
							onColorPicked(color);
							onRequestClose();
						}}
						color={color}
						key={color}
					/>
				);
			})}
		</div>
	);
};

const ColorButton = ({
	color,
	onSelected,
}: {
	color: ColorsType;
	onSelected: () => void;
}) => (
	<div className="flex shrink-0 items-center justify-center p-px">
		<Button
			type="button"
			data-flume-component="color-button"
			size="icon-sm"
			variant="outline"
			className="size-5 min-h-5 min-w-5 rounded-[3px] border-0 p-0 hover:opacity-90"
			onClick={onSelected}
			style={{ backgroundColor: SWATCH_HEX[color] }}
			data-color={color}
			aria-label={color}
		/>
	</div>
);

export default ColorPicker;
