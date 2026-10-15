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

export type DigramEntry = {
	key: string;
	first: string;
	second: string;
	ruleId: string;
	position: number;
	count: number;
};

export type MicroStepKind =
	| "INITIAL"
	| "APPEND_SYMBOL"
	| "DIGRAM_COLLISION"
	| "CREATE_NEW_RULE"
	| "REUSE_EXISTING_RULE"
	| "DISSOLVE_UNDERUSED_RULE"
	| "INVARIANT_STABLE"
	| "LIVE_UPDATE";

export interface GrammarSnapshot {
	stepIndex: number;
	charIndex: number;
	processedInput: string;
	fullInput: string;
	kind: MicroStepKind;
	title: string;
	explanation: string;
	invariantStatus: {
		digramUniqueness: "NOMINAL" | "VIOLATED" | "REPAIRED";
		ruleUtility: "NOMINAL" | "VIOLATED" | "REPAIRED";
	};
	highlightDigram?: [string, string];
	highlightRuleId?: string;
	dissolvedRuleId?: string;
	dissolvedIntoRuleId?: string;
	rootRule: RuleDefinition;
	rules: RuleDefinition[];
	digrams: DigramEntry[];
	metrics: {
		rawLength: number;
		grammarSymbols: number;
		rootSymbols: number;
		activeRulesCount: number;
		compressionRatio: number;
		surprise?: number;
		activeTokens?: string;
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

export interface PresetSequence {
	id: string;
	name: string;
	sequence: string;
	category: string;
	tagline: string;
	insight: string;
}

export const PRESET_SEQUENCES: PresetSequence[] = [
	{
		id: "market-support-opposition",
		name: "Support vs. Opposition Cycle",
		sequence: "R01 R02 R04 R01 R02 R04",
		category: "Market Dynamic",
		tagline:
			"Concurrent tension between Bid Support (R01), Wall (R02), and Floor Evaporation (R04).",
		insight:
			"When [R01, R02, R04] recurs, Sequitur unifies the multi-signal tension into a single macro-rule R1, reducing sequence entropy.",
	},
	{
		id: "rule-dissolution",
		name: "Rule Dissolution Cascade",
		sequence: "abcdbcabcd",
		category: "Canonical Paper (1997)",
		tagline:
			"Watches R1 form from 'bc' and then dissolve into R2 ('bcd') when a longer repeat arrives.",
		insight:
			"Nevill-Manning & Witten's classic example: 'bc' first forms R1. When 'abcd' repeats later, R1 is absorbed into R2, dropping utility to 1 so it dissolves automatically.",
	},
	{
		id: "breakout-shock",
		name: "Liquidity Breakout Shock",
		sequence: "R02 R04 R02 R04 R08 R02 R04",
		category: "Market Dynamic",
		tagline:
			"Periodic compression interrupted by Mutation Activity Imbalance (R08).",
		insight:
			"Shows how Sequitur maintains stable compression of routine order flow while immediately flagging structural disruptions.",
	},
	{
		id: "abracadabra",
		name: "Abracadabra Cadence",
		sequence: "abracadabra",
		category: "Lexical Pattern",
		tagline:
			"Builds nested rules as 'ab', 'abra', and overlapping vocalic cadence repeat.",
		insight:
			"First 'ab' forms R1. Immediately 'R1 r' forms R2, and 'R2 a' ('abra') coalesces into a macro-capsule at both ends.",
	},
	{
		id: "exponential-run",
		name: "Binary Power Cascade",
		sequence: "aaaaaaaaaaaa",
		category: "Hierarchical Run-Length",
		tagline:
			"Logarithmic compression: 12 identical tokens collapse into nested powers of two.",
		insight:
			"Every pair of 'aa' becomes R1 (length 2), pairs of 'R1 R1' become R2 (length 4), achieving O(log N) grammar footprint.",
	},
	{
		id: "thue-morse",
		name: "Thue-Morse Fractal",
		sequence: "0110100110010110",
		category: "Aperiodic Self-Similar",
		tagline:
			"Cube-free binary sequence that still reveals rich internal digram symmetries.",
		insight:
			"Sequitur discovers recursive complement building blocks even across non-periodic sequences.",
	},
];

// Dark-mode optimized palette with high contrast and translucent glow
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

interface RawRule {
	id: string;
	number: number;
	symbols: string[];
}

/**
 * Generates the complete sequence of micro-step snapshots for an input sequence
 * using the Nevill-Manning & Witten (1997) online Sequitur algorithm.
 */
export function generateSequiturTrace(rawInput: string): GrammarSnapshot[] {
	// Support space-separated tokens (e.g. "R01 R02 R04") or character stream (e.g. "abcdbcabcd")
	const tokens = rawInput.trim().includes(" ")
		? rawInput.trim().split(/\s+/).filter(Boolean)
		: rawInput.replace(/\s+/g, "").split("");

	const snapshots: GrammarSnapshot[] = [];
	let nextRuleNumber = 1;

	const rawRules = new Map<string, RawRule>();
	rawRules.set("S", { id: "S", number: 0, symbols: [] });

	const createSnapshot = (
		charIndex: number,
		kind: MicroStepKind,
		title: string,
		explanation: string,
		invariantStatus: GrammarSnapshot["invariantStatus"],
		extra?: {
			highlightDigram?: [string, string];
			highlightRuleId?: string;
			dissolvedRuleId?: string;
			dissolvedIntoRuleId?: string;
		},
	): GrammarSnapshot => {
		const memoExpansion = new Map<string, string>();
		const memoDepth = new Map<string, number>();

		const isRuleSymbol = (sym: string): boolean => {
			return sym !== "S" && rawRules.has(sym);
		};

		const getExpansion = (sym: string): string => {
			if (!isRuleSymbol(sym)) return sym;
			const cached = memoExpansion.get(sym);
			if (cached !== undefined) return cached;
			const r = rawRules.get(sym);
			if (!r) return sym;
			const exp = r.symbols.map(getExpansion).join(" ");
			memoExpansion.set(sym, exp);
			return exp;
		};

		const getDepth = (sym: string): number => {
			if (!isRuleSymbol(sym)) return 0;
			const cached = memoDepth.get(sym);
			if (cached !== undefined) return cached;
			const r = rawRules.get(sym);
			if (!r || r.symbols.length === 0) return 0;
			const d = 1 + Math.max(...r.symbols.map(getDepth));
			memoDepth.set(sym, d);
			return d;
		};

		const refCounts = new Map<string, number>();
		for (const r of rawRules.values()) {
			for (const s of r.symbols) {
				if (isRuleSymbol(s)) {
					refCounts.set(s, (refCounts.get(s) ?? 0) + 1);
				}
			}
		}

		const ruleIntervals = new Map<string, [number, number][]>();
		for (const r of rawRules.keys()) {
			if (r !== "S") ruleIntervals.set(r, []);
		}

		const collectIntervals = (sym: string, startOffset: number): number => {
			if (isRuleSymbol(sym)) {
				const r = rawRules.get(sym);
				if (!r) return 1;
				let innerOffset = startOffset;
				for (const childSym of r.symbols) {
					innerOffset += collectIntervals(childSym, innerOffset);
				}
				const totalSpan = innerOffset - startOffset;
				const arr = ruleIntervals.get(sym) ?? [];
				arr.push([startOffset, innerOffset - 1]);
				ruleIntervals.set(sym, arr);
				return totalSpan;
			}
			return 1;
		};

		const rootRaw = rawRules.get("S") ?? { id: "S", number: 0, symbols: [] };
		let cursor = 0;
		const rootSymbols: SymbolToken[] = rootRaw.symbols.map((sym, idx) => {
			const len = collectIntervals(sym, cursor);
			const span: [number, number] = [cursor, cursor + len - 1];
			cursor += len;
			const isRule = isRuleSymbol(sym);
			return {
				id: `S-${idx}-${sym}`,
				value: sym,
				isRule,
				ruleId: isRule ? sym : undefined,
				charSpan: span,
			};
		});

		const activeRules: RuleDefinition[] = [];
		const sortedRuleEntries = Array.from(rawRules.values())
			.filter((r) => r.id !== "S")
			.sort((a, b) => a.number - b.number);

		for (const r of sortedRuleEntries) {
			const syms: SymbolToken[] = r.symbols.map((sym, idx) => {
				const isRule = isRuleSymbol(sym);
				return {
					id: `${r.id}-${idx}-${sym}`,
					value: sym,
					isRule,
					ruleId: isRule ? sym : undefined,
				};
			});
			activeRules.push({
				id: r.id,
				number: r.number,
				symbols: syms,
				refCount: refCounts.get(r.id) ?? 0,
				expansion: getExpansion(r.id),
				depth: getDepth(r.id),
				colorIndex: (r.number - 1) % RULE_PALETTE.length,
				intervals: ruleIntervals.get(r.id) ?? [],
			});
		}

		const rootRuleDef: RuleDefinition = {
			id: "S",
			number: 0,
			symbols: rootSymbols,
			refCount: 1,
			expansion: getExpansion("S"),
			depth: getDepth("S"),
			colorIndex: 0,
			intervals: cursor > 0 ? [[0, cursor - 1]] : [],
		};

		const digrams: DigramEntry[] = [];
		const digramCounts = new Map<string, number>();
		const allRuleOrder = [rootRaw, ...sortedRuleEntries];
		for (const r of allRuleOrder) {
			for (let i = 0; i < r.symbols.length - 1; i++) {
				const key = `${r.symbols[i]}|${r.symbols[i + 1]}`;
				digramCounts.set(key, (digramCounts.get(key) ?? 0) + 1);
			}
		}

		for (const r of allRuleOrder) {
			for (let i = 0; i < r.symbols.length - 1; i++) {
				const first = r.symbols[i];
				const second = r.symbols[i + 1];
				const key = `${first}|${second}`;
				digrams.push({
					key,
					first,
					second,
					ruleId: r.id,
					position: i,
					count: digramCounts.get(key) ?? 1,
				});
			}
		}

		const rawLength = Math.max(0, charIndex + 1);
		const rootSymsCount = rootRaw.symbols.length;
		const rulesSymsCount = sortedRuleEntries.reduce(
			(acc, r) => acc + r.symbols.length,
			0,
		);
		const totalGrammarSymbols = rootSymsCount + rulesSymsCount;

		return {
			stepIndex: snapshots.length,
			charIndex,
			processedInput: tokens.slice(0, charIndex + 1).join(" "),
			fullInput: tokens.join(" "),
			kind,
			title,
			explanation,
			invariantStatus,
			highlightDigram: extra?.highlightDigram,
			highlightRuleId: extra?.highlightRuleId,
			dissolvedRuleId: extra?.dissolvedRuleId,
			dissolvedIntoRuleId: extra?.dissolvedIntoRuleId,
			rootRule: rootRuleDef,
			rules: activeRules,
			digrams,
			metrics: {
				rawLength,
				grammarSymbols: totalGrammarSymbols,
				rootSymbols: rootSymsCount,
				activeRulesCount: activeRules.length,
				compressionRatio: rawLength > 0 ? totalGrammarSymbols / rawLength : 1,
			},
		};
	};

	const findDuplicateDigram = (): {
		first: string;
		second: string;
		loc1: { ruleId: string; index: number };
		loc2: { ruleId: string; index: number };
	} | null => {
		const seen = new Map<string, { ruleId: string; index: number }>();
		const rootRule = rawRules.get("S") ?? { id: "S", number: 0, symbols: [] };
		const orderedRules = [
			rootRule,
			...Array.from(rawRules.values())
				.filter((r) => r.id !== "S")
				.sort((a, b) => a.number - b.number),
		];

		for (const r of orderedRules) {
			for (let i = 0; i < r.symbols.length - 1; i++) {
				if (
					i > 0 &&
					r.symbols[i - 1] === r.symbols[i] &&
					r.symbols[i] === r.symbols[i + 1]
				) {
					continue;
				}
				const key = `${r.symbols[i]}|${r.symbols[i + 1]}`;
				const prev = seen.get(key);
				if (prev) {
					if (prev.ruleId === r.id && i === prev.index + 1) continue;
					return {
						first: r.symbols[i],
						second: r.symbols[i + 1],
						loc1: prev,
						loc2: { ruleId: r.id, index: i },
					};
				}
				seen.set(key, { ruleId: r.id, index: i });
			}
		}
		return null;
	};

	const findUnderusedRule = (): {
		underusedRule: RawRule;
		callerRuleId: string;
		callerIndex: number;
	} | null => {
		for (const r of rawRules.values()) {
			if (r.id === "S") continue;
			let count = 0;
			let lastCaller = "";
			let lastIdx = -1;
			for (const other of rawRules.values()) {
				for (let i = 0; i < other.symbols.length; i++) {
					if (other.symbols[i] === r.id) {
						count++;
						lastCaller = other.id;
						lastIdx = i;
					}
				}
			}
			if (count === 1) {
				return {
					underusedRule: r,
					callerRuleId: lastCaller,
					callerIndex: lastIdx,
				};
			}
		}
		return null;
	};

	snapshots.push(
		createSnapshot(
			-1,
			"INITIAL",
			"Initial Empty Grammar",
			"Starting Sequitur state. S is empty; invariants 1 (Digram Uniqueness) and 2 (Rule Utility) hold vacuously.",
			{ digramUniqueness: "NOMINAL", ruleUtility: "NOMINAL" },
		),
	);

	for (let cIdx = 0; cIdx < tokens.length; cIdx++) {
		const ch = tokens[cIdx];
		rawRules.get("S")?.symbols.push(ch);

		snapshots.push(
			createSnapshot(
				cIdx,
				"APPEND_SYMBOL",
				`Append Token '${ch}' to S`,
				`Token '${ch}' appended at position ${cIdx + 1}. Checking digram uniqueness on the new adjacent pair...`,
				{ digramUniqueness: "NOMINAL", ruleUtility: "NOMINAL" },
			),
		);

		let cascadeLimit = 0;
		while (cascadeLimit++ < 30) {
			const dup = findDuplicateDigram();
			if (dup) {
				const { first, second, loc1, loc2 } = dup;

				snapshots.push(
					createSnapshot(
						cIdx,
						"DIGRAM_COLLISION",
						`Digram Collision: [${first}, ${second}]`,
						`Invariant 1 Violated: Digram [${first}, ${second}] occurs multiple times (${loc1.ruleId} and ${loc2.ruleId}). Enforcing uniqueness by rule substitution.`,
						{ digramUniqueness: "VIOLATED", ruleUtility: "NOMINAL" },
						{ highlightDigram: [first, second] },
					),
				);

				let existingRule: RawRule | undefined;
				for (const r of rawRules.values()) {
					if (
						r.id !== "S" &&
						r.symbols.length === 2 &&
						r.symbols[0] === first &&
						r.symbols[1] === second
					) {
						existingRule = r;
						break;
					}
				}

				if (existingRule) {
					const matchingRule = existingRule;
					const targetLoc = loc1.ruleId === matchingRule.id ? loc2 : loc1;
					const targetRule = rawRules.get(targetLoc.ruleId);
					targetRule?.symbols.splice(targetLoc.index, 2, matchingRule.id);

					snapshots.push(
						createSnapshot(
							cIdx,
							"REUSE_EXISTING_RULE",
							`Substitute [${first} ${second}] → ${matchingRule.id}`,
							`Rule ${matchingRule.id} already defines [${first} ${second}]. Replaced duplicate in ${targetRule?.id ?? targetLoc.ruleId} with ${matchingRule.id}.`,
							{ digramUniqueness: "REPAIRED", ruleUtility: "NOMINAL" },
							{
								highlightRuleId: matchingRule.id,
								highlightDigram: [first, second],
							},
						),
					);
				} else {
					const newRuleId = `M${nextRuleNumber++}`;
					rawRules.set(newRuleId, {
						id: newRuleId,
						number: nextRuleNumber - 1,
						symbols: [first, second],
					});

					if (loc1.ruleId === loc2.ruleId) {
						const r = rawRules.get(loc1.ruleId);
						if (r) {
							const high = Math.max(loc1.index, loc2.index);
							const low = Math.min(loc1.index, loc2.index);
							r.symbols.splice(high, 2, newRuleId);
							r.symbols.splice(low, 2, newRuleId);
						}
					} else {
						const r1 = rawRules.get(loc1.ruleId);
						const r2 = rawRules.get(loc2.ruleId);
						if (r1 && r2) {
							r1.symbols.splice(loc1.index, 2, newRuleId);
							r2.symbols.splice(loc2.index, 2, newRuleId);
						}
					}

					snapshots.push(
						createSnapshot(
							cIdx,
							"CREATE_NEW_RULE",
							`Induce New Rule ${newRuleId} -> [${first}, ${second}]`,
							`Created new rule ${newRuleId} and replaced duplicate pair. Digram Uniqueness restored.`,
							{ digramUniqueness: "REPAIRED", ruleUtility: "NOMINAL" },
							{ highlightRuleId: newRuleId, highlightDigram: [first, second] },
						),
					);
				}

				let utilityLoop = 0;
				while (utilityLoop++ < 20) {
					const underused = findUnderusedRule();
					if (!underused) break;

					const { underusedRule, callerRuleId, callerIndex } = underused;
					const callerRule = rawRules.get(callerRuleId);
					if (
						callerRule &&
						callerRule.symbols[callerIndex] === underusedRule.id
					) {
						callerRule.symbols.splice(callerIndex, 1, ...underusedRule.symbols);
					}
					rawRules.delete(underusedRule.id);

					snapshots.push(
						createSnapshot(
							cIdx,
							"DISSOLVE_UNDERUSED_RULE",
							`Dissolve Underused Rule ${underusedRule.id} into ${callerRuleId}`,
							`Invariant 2 (Rule Utility): ${underusedRule.id} dropped to 1 reference. Inlined into ${callerRuleId}.`,
							{
								digramUniqueness: findDuplicateDigram()
									? "VIOLATED"
									: "NOMINAL",
								ruleUtility: "REPAIRED",
							},
							{
								dissolvedRuleId: underusedRule.id,
								dissolvedIntoRuleId: callerRuleId,
								highlightRuleId: callerRuleId,
							},
						),
					);
				}
				continue;
			}
			break;
		}
	}

	return snapshots;
}

/**
 * Builds recursive nested capsule nodes from a GrammarSnapshot for telescoping visualization.
 */
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
 * Converts a backend live SequiturGrammarResponse into a GrammarSnapshot.
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
	extra?: {
		surprise?: number;
		activeTokens?: string;
	},
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

	return {
		stepIndex: 0,
		charIndex: Math.max(0, rawLength - 1),
		processedInput: startList.join(" "),
		fullInput: startList.join(" "),
		kind: "LIVE_UPDATE",
		title: `Live Grammar: ${res.symbol ?? "BTC/USD"}`,
		explanation: `Online hierarchical grammar continuously induced from predictive coding gated regions (${activeRules.length} active rules).`,
		invariantStatus: {
			digramUniqueness: "NOMINAL",
			ruleUtility: "NOMINAL",
		},
		rootRule: rootRuleDef,
		rules: activeRules,
		digrams: [],
		metrics: {
			rawLength,
			grammarSymbols: totalGrammarSymbols,
			rootSymbols: rootCount,
			activeRulesCount: activeRules.length,
			compressionRatio: rawLength > 0 ? totalGrammarSymbols / rawLength : 1,
			surprise: extra?.surprise,
			activeTokens: extra?.activeTokens,
		},
	};
}
