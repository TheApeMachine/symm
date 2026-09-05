import type { MouseEvent, ReactNode } from "react";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";

const symbolExact =
	/^[A-Za-z0-9][A-Za-z0-9._-]{0,19}\/(?:USD|EUR|USDT|USDC|BTC|ETH|GBP|CAD|AUD|JPY|CHF)$/i;
const symbolText =
	/\b[A-Za-z0-9][A-Za-z0-9._-]{0,19}\/(?:USD|EUR|USDT|USDC|BTC|ETH|GBP|CAD|AUD|JPY|CHF)\b/gi;

export const SymbolFocusLayer = ({ children }: { children: ReactNode }) => {
	const onClickCapture = (event: MouseEvent<HTMLDivElement>) => {
		const selection = window.getSelection();

		if (selection !== null && !selection.isCollapsed) {
			return;
		}

		if (!(event.target instanceof Element)) {
			return;
		}

		let symbol = "";
		const marked = event.target.closest("[data-symbol],[data-focus-symbol]");

		if (marked !== null && event.currentTarget.contains(marked)) {
			const value = (
				marked.getAttribute("data-symbol") ??
				marked.getAttribute("data-focus-symbol") ??
				""
			)
				.trim()
				.toUpperCase();

			if (symbolExact.test(value)) {
				symbol = value;
			}
		}

		let current: Element | null = event.target;

		while (
			symbol === "" &&
			current !== null &&
			event.currentTarget.contains(current)
		) {
			const text = current.textContent?.trim() ?? "";

			if (text.length > 0 && text.length <= 120) {
				const matches = text.match(symbolText) ?? [];
				const symbols = [
					...new Set(matches.map((value) => value.toUpperCase())),
				].filter((value) => symbolExact.test(value));

				if (symbols.length === 1) {
					symbol = String(symbols[0]);
				}
			}

			if (current === event.currentTarget) {
				break;
			}

			current = current.parentElement;
		}

		if (symbol === "" || appStore.state.focusSymbol === symbol) {
			return;
		}

		appStore.actions.updateFocusSymbol(symbol);
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
