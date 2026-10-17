export type SymbolToken = {
	id: string;
	value: string;
	isRule: boolean;
	ruleId?: string;
	charSpan?: [number, number];
};

export type RuleDefinition = {
	id: string;
	number: number;
	symbols: SymbolToken[];
	refCount: number;
	expansion: string;
	depth: number;
	colorIndex: number;
	intervals: [number, number][];
	action?: string;
};

/*
GrammarSnapshot lays a backend grammar (strategy.SequiturFrame) out for the
tape, strata and rulebook lenses. Its invariants are the backend's Check.
*/
export interface GrammarSnapshot {
	charIndex: number;
	kind: "APPEND_SYMBOL";
	title: string;
	invariantStatus: {
		digramUniqueness: "NOMINAL" | "VIOLATED";
		ruleUtility: "NOMINAL" | "VIOLATED";
	};
	highlightRuleId?: string;
	rootRule: RuleDefinition;
	rules: RuleDefinition[];
	metrics: {
		rawLength: number;
		grammarSymbols: number;
		rootSymbols: number;
		activeRulesCount: number;
		compressionRatio: number;
	};
}

export interface NestedCapsuleNode {
	instanceId: string;
	symbol: string;
	isRule: boolean;
	ruleId?: string;
	colorIndex?: number;
	expansion: string;
	startChar: number;
	endChar: number;
	depth: number;
	children?: NestedCapsuleNode[];
}

export const RULE_PALETTE = [
	{
		name: "Sky",
		bg: "bg-sky-950/40",
		bgSubtle: "bg-sky-950/20",
		border: "border-sky-500/70",
		text: "text-sky-300",
		badgeBg: "bg-sky-600",
		hex: "#38bdf8",
		lightHex: "#0c4a6e",
	},
	{
		name: "Emerald",
		bg: "bg-emerald-950/40",
		bgSubtle: "bg-emerald-950/20",
		border: "border-emerald-500/70",
		text: "text-emerald-300",
		badgeBg: "bg-emerald-600",
		hex: "#34d399",
		lightHex: "#064e3b",
	},
	{
		name: "Amber",
		bg: "bg-amber-950/40",
		bgSubtle: "bg-amber-950/20",
		border: "border-amber-500/70",
		text: "text-amber-300",
		badgeBg: "bg-amber-600",
		hex: "#fbbf24",
		lightHex: "#78350f",
	},
	{
		name: "Rose",
		bg: "bg-rose-950/40",
		bgSubtle: "bg-rose-950/20",
		border: "border-rose-500/70",
		text: "text-rose-300",
		badgeBg: "bg-rose-600",
		hex: "#fb7185",
		lightHex: "#881337",
	},
	{
		name: "Purple",
		bg: "bg-purple-950/40",
		bgSubtle: "bg-purple-950/20",
		border: "border-purple-500/70",
		text: "text-purple-300",
		badgeBg: "bg-purple-600",
		hex: "#c084fc",
		lightHex: "#581c87",
	},
	{
		name: "Teal",
		bg: "bg-teal-950/40",
		bgSubtle: "bg-teal-950/20",
		border: "border-teal-500/70",
		text: "text-teal-300",
		badgeBg: "bg-teal-600",
		hex: "#2dd4bf",
		lightHex: "#134e4a",
	},
	{
		name: "Orange",
		bg: "bg-orange-950/40",
		bgSubtle: "bg-orange-950/20",
		border: "border-orange-500/70",
		text: "text-orange-300",
		badgeBg: "bg-orange-600",
		hex: "#fb923c",
		lightHex: "#7c2d12",
	},
	{
		name: "Indigo",
		bg: "bg-indigo-950/40",
		bgSubtle: "bg-indigo-950/20",
		border: "border-indigo-500/70",
		text: "text-indigo-300",
		badgeBg: "bg-indigo-600",
		hex: "#818cf8",
		lightHex: "#312e81",
	},
];

export function buildNestedCapsules(
	snapshot: GrammarSnapshot,
): NestedCapsuleNode[] {
	const ruleMap = new Map<string, RuleDefinition>();
	for (const r of snapshot.rules) {
		ruleMap.set(r.id, r);
	}

	const buildNode = (
		token: SymbolToken,
		startChar: number,
		depth: number,
		pathPrefix: string,
		visited = new Set<string>(),
	): NestedCapsuleNode => {
		if (
			!token.isRule ||
			!token.ruleId ||
			!ruleMap.has(token.ruleId) ||
			visited.has(token.ruleId) ||
			depth > 8
		) {
			return {
				instanceId: `${pathPrefix}-${token.id}`,
				symbol: token.value,
				isRule: false,
				expansion: token.value,
				startChar,
				endChar: startChar,
				depth,
			};
		}

		const nextVisited = new Set(visited);
		nextVisited.add(token.ruleId);
		const r = ruleMap.get(token.ruleId);
		if (!r) {
			return {
				instanceId: `${pathPrefix}-${token.id}`,
				symbol: token.value,
				isRule: false,
				expansion: token.value,
				startChar,
				endChar: startChar,
				depth,
			};
		}
		let cursor = startChar;
		const children: NestedCapsuleNode[] = r.symbols.map((childTok, idx) => {
			const childNode = buildNode(
				childTok,
				cursor,
				depth + 1,
				`${pathPrefix}-${r.id}_${idx}`,
				nextVisited,
			);
			cursor = childNode.endChar + 1;
			return childNode;
		});

		return {
			instanceId: `${pathPrefix}-${token.id}`,
			symbol: r.id,
			isRule: true,
			ruleId: r.id,
			colorIndex: r.colorIndex,
			expansion: r.expansion,
			startChar,
			endChar: Math.max(startChar, cursor - 1),
			depth,
			children,
		};
	};

	let rootCursor = 0;
	return snapshot.rootRule.symbols.map((tok, idx) => {
		const node = buildNode(tok, rootCursor, 0, `root_${idx}`);
		rootCursor = node.endChar + 1;
		return node;
	});
}

/**
 * Converts a backend grammar (rules and S) into a GrammarSnapshot.
 */
export function grammarResponseToSnapshot(
	res: {
		symbol?: string;
		rules?: Array<{
			id?: number;
			name: string;
			count?: number;
			depth?: number;
			length?: number;
			symbols?: string[];
			terminals?: string[];
			action?: string;
		}>;
		start?: string[];
	},
	violations: string[],
): GrammarSnapshot {
	const rulesList = Array.isArray(res?.rules) ? res.rules : [];
	const startList = Array.isArray(res?.start) ? res.start : [];

	const ruleMap = new Map<string, (typeof rulesList)[0]>();
	for (const r of rulesList) {
		if (r?.name) ruleMap.set(r.name, r);
	}

	const ruleIntervals = new Map<string, [number, number][]>();
	for (const r of rulesList) {
		if (r?.name) ruleIntervals.set(r.name, []);
	}

	const collectIntervals = (
		sym: string,
		startOffset: number,
		visited = new Set<string>(),
	): number => {
		const ruleObj = ruleMap.get(sym);
		if (ruleObj && !visited.has(sym)) {
			visited.add(sym);
			let innerOffset = startOffset;
			for (const childSym of ruleObj.symbols ?? []) {
				innerOffset += collectIntervals(
					childSym,
					innerOffset,
					new Set(visited),
				);
			}
			const totalSpan = Math.max(1, innerOffset - startOffset);
			const arr = ruleIntervals.get(sym) ?? [];
			arr.push([startOffset, innerOffset - 1]);
			ruleIntervals.set(sym, arr);
			return totalSpan;
		}
		return 1;
	};

	let cursor = 0;
	const rootSymbols: SymbolToken[] = startList.map((s, idx) => {
		const symStr = typeof s === "string" ? s : String(s);
		const spanLen = collectIntervals(symStr, cursor);
		const span: [number, number] = [cursor, cursor + spanLen - 1];
		cursor += spanLen;
		const isRule = ruleMap.has(symStr);
		return {
			id: `S-${idx}-${symStr}`,
			value: symStr,
			isRule,
			ruleId: isRule ? symStr : undefined,
			charSpan: span,
		};
	});

	const activeRules: RuleDefinition[] = rulesList.map((r, rIdx) => {
		const ruleId = r.name || `M${r.id ?? rIdx + 1}`;
		const syms: SymbolToken[] = (r.symbols ?? []).map((s, sIdx) => {
			const symStr = typeof s === "string" ? s : String(s);
			const isSubRule = ruleMap.has(symStr);
			return {
				id: `${ruleId}-${sIdx}-${symStr}`,
				value: symStr,
				isRule: isSubRule,
				ruleId: isSubRule ? symStr : undefined,
			};
		});

		const numId = typeof r.id === "number" ? r.id : rIdx + 1;

		return {
			id: ruleId,
			number: numId,
			symbols: syms,
			refCount: r.count ?? 1,
			expansion: Array.isArray(r.terminals) ? r.terminals.join(" ") : "",
			depth: r.depth ?? 1,
			colorIndex: Math.max(0, (numId - 1) % RULE_PALETTE.length),
			intervals: ruleIntervals.get(ruleId) ?? [],
			action: r.action,
		};
	});

	const rootRuleDef: RuleDefinition = {
		id: "S",
		number: 0,
		symbols: rootSymbols,
		refCount: 1,
		expansion: rootSymbols.map((s) => s.value).join(" "),
		depth: 0,
		colorIndex: 0,
		intervals: cursor > 0 ? [[0, cursor - 1]] : [],
	};

	const rawLength = cursor;
	const rootCount = rootSymbols.length;
	const rulesCount = activeRules.reduce((acc, r) => acc + r.symbols.length, 0);
	const totalGrammarSymbols = rootCount + rulesCount;

	const violated = (kind: string) =>
		violations.some((v) => v.startsWith(kind)) ? "VIOLATED" : "NOMINAL";

	return {
		charIndex: Math.max(0, rawLength - 1),
		kind: "APPEND_SYMBOL",
		title: `Live Grammar: ${res.symbol ?? ""}`,
		invariantStatus: {
			digramUniqueness: violated("digram uniqueness"),
			ruleUtility: violated("rule utility"),
		},
		rootRule: rootRuleDef,
		rules: activeRules,
		metrics: {
			rawLength,
			grammarSymbols: totalGrammarSymbols,
			rootSymbols: rootCount,
			activeRulesCount: activeRules.length,
			compressionRatio: rawLength > 0 ? totalGrammarSymbols / rawLength : 1,
		},
	};
}

/**
 * frameToSnapshot lays out a live SequiturFrame for the tape and strata
 * lenses: the last `window` symbols of R0 (a live R0 grows without bound),
 * every rule, the invariants as the backend checked them, and the rule this
 * frame completed highlighted.
 */
export function frameToSnapshot(
	frame: {
		symbol: string;
		rules: Parameters<typeof grammarResponseToSnapshot>[0]["rules"];
		start: string[];
		violations: string[];
		completed: string;
	},
	window: number,
): GrammarSnapshot {
	const snapshot = grammarResponseToSnapshot(
		{ symbol: frame.symbol, rules: frame.rules, start: frame.start.slice(-window) },
		frame.violations,
	);

	return { ...snapshot, highlightRuleId: frame.completed || undefined };
}
