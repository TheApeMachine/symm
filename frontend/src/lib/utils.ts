import type { ClassValue } from "clsx";
import { clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export const cn = (...inputs: ClassValue[]): string => {
	return twMerge(clsx(inputs));
};

export const formatPnl = (value: number): string => {
	const sign = value >= 0 ? "+" : "";
	return `${sign}€${value.toFixed(4)}`;
};

export const formatEur = (value: number): string => {
	return `€${value.toFixed(2)}`;
};

export const pnlTone = (value: number | undefined): string => {
	if (value === undefined) return "";
	if (value > 0) return "text-(--dash-up)";
	if (value < 0) return "text-(--dash-down)";
	return "text-(--dash-muted)";
};
