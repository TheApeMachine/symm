import type { MouseEvent, ReactNode } from "react";
import { terminalStore } from "#/collections/terminal";
import {
	isSymbolPair,
	normalizeSymbolPair,
	symbolPairFromText,
} from "#/components/terminal/symbols";

const symbolFromMarkedElement = (
	element: Element,
	boundary: Element,
): string | null => {
	const marked = element.closest("[data-symbol],[data-focus-symbol]");

	if (marked === null || !boundary.contains(marked)) {
		return null;
	}

	const value =
		marked.getAttribute("data-symbol") ??
		marked.getAttribute("data-focus-symbol") ??
		"";

	return isSymbolPair(value) ? normalizeSymbolPair(value) : null;
};

const symbolFromClickTarget = (
	target: EventTarget | null,
	boundary: Element,
): string | null => {
	if (!(target instanceof Element)) {
		return null;
	}

	const marked = symbolFromMarkedElement(target, boundary);

	if (marked !== null) {
		return marked;
	}

	let current: Element | null = target;

	while (current !== null && boundary.contains(current)) {
		const text = current.textContent?.trim() ?? "";

		if (text.length > 0 && text.length <= 120) {
			const symbol = symbolPairFromText(text);

			if (symbol !== null) {
				return symbol;
			}
		}

		if (current === boundary) {
			break;
		}

		current = current.parentElement;
	}

	return null;
};

export const SymbolFocusLayer = ({ children }: { children: ReactNode }) => {
	const onClickCapture = (event: MouseEvent<HTMLDivElement>) => {
		const selection = window.getSelection();

		if (selection !== null && !selection.isCollapsed) {
			return;
		}

		const symbol = symbolFromClickTarget(event.target, event.currentTarget);

		if (symbol === null || terminalStore.state.focusSymbol === symbol) {
			return;
		}

		terminalStore.actions.selectFocusSymbol(symbol);
	};

	return (
		<div
			className="flex min-h-0 flex-1 flex-col"
			onClickCapture={onClickCapture}
		>
			{children}
		</div>
	);
};
