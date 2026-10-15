export type SymbolToken = {
  id: string; // unique instance id for smooth layout animations
  value: string; // e.g., 'a', 'b', or 'R1', 'R2'
  isRule: boolean;
  ruleId?: string; // 'R1', 'R2', etc.
  charSpan?: [number, number]; // inclusive [startIdx, endIdx] in the raw input string (for S tokens)
};

export type RuleDefinition = {
  id: string; // 'S' or 'R1', 'R2', ...
  number: number; // 0 for S, 1 for R1, etc.
  symbols: SymbolToken[];
  refCount: number; // number of times referenced across S and other rules
  expansion: string; // full terminal string expansion
  depth: number; // hierarchy level (1 for rules of terminals, 2+ for rules referencing rules)
  colorIndex: number; // deterministic palette index
  intervals: [number, number][]; // all [start, end] inclusive character spans in S where this rule occurs
};

export type DigramEntry = {
  key: string; // e.g. "a|b" or "R1|c"
  first: string;
  second: string;
  ruleId: string; // which rule ('S', 'R1', ...) holds this digram
  position: number; // index in that rule's symbols array
  count: number; // 1 when valid, 2 when collision is detected
};

export type MicroStepKind =
  | 'INITIAL'
  | 'APPEND_SYMBOL'
  | 'DIGRAM_COLLISION'
  | 'CREATE_NEW_RULE'
  | 'REUSE_EXISTING_RULE'
  | 'DISSOLVE_UNDERUSED_RULE'
  | 'INVARIANT_STABLE';

export interface GrammarSnapshot {
  stepIndex: number;
  charIndex: number; // index of character being processed (-1 for initial)
  processedInput: string;
  fullInput: string;
  kind: MicroStepKind;
  title: string;
  explanation: string;
  invariantStatus: {
    digramUniqueness: 'NOMINAL' | 'VIOLATED' | 'REPAIRED';
    ruleUtility: 'NOMINAL' | 'VIOLATED' | 'REPAIRED';
  };
  highlightDigram?: [string, string];
  highlightRuleId?: string;
  dissolvedRuleId?: string;
   dissolvedIntoRuleId?: string;
  rootRule: RuleDefinition;
  rules: RuleDefinition[]; // R1, R2, ...
  digrams: DigramEntry[];
  metrics: {
    rawLength: number;
    grammarSymbols: number; // total symbols in S + all active rules (or S + sum(|R_i|))
    rootSymbols: number;
    activeRulesCount: number;
    compressionRatio: number; // grammarSymbols / rawLength
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
    id: 'rule-dissolution',
    name: 'Rule Dissolution Cascade',
    sequence: 'abcdbcabcd',
    category: 'Canonical Paper (1997)',
    tagline: 'Watches R1 form from "bc" and then dissolve into R2 ("bcd") when a longer repeat arrives.',
    insight:
      'Nevill-Manning & Witten’s classic example: "bc" first forms R1. When "abcd" repeats later, R1 ("bc") is absorbed into R2 ("bcd") and R3 ("abcd"), dropping R1’s utility to 1 so it dissolves automatically.'
  },
  {
    id: 'abracadabra',
    name: 'Abracadabra Motif',
    sequence: 'abracadabra',
    category: 'Lexical Pattern',
    tagline: 'Builds nested rules as "ab", "abra", and overlapping vocalic cadence repeat.',
    insight:
      'First "ab" forms R1 at index 7. Immediately "R1 r" forms R2, and "R2 a" ("abra") coalesces into a 4-character macro-capsule at both ends of the word.'
  },
  {
    id: 'exponential-run',
    name: 'Binary Power Cascade',
    sequence: 'aaaaaaaaaaaa',
    category: 'Hierarchical Run-Length',
    tagline: 'Demonstrates logarithmic compression: 12 identical characters collapse into nested powers of two.',
    insight:
      'Every pair of "aa" becomes R1 (length 2), pairs of "R1 R1" become R2 (length 4), and "R2 R2" becomes R3 (length 8), achieving O(log N) grammar size.'
  },
  {
    id: 'dna-tandem',
    name: 'CRISPR Tandem Repeat',
    sequence: 'ACGTACGTACGTACGT',
    category: 'Genomic Sequence',
    tagline: 'Coalesces 4-base nucleotide codons into multi-layer chromosomal repeat capsules.',
    insight:
      'Notice how R1 ("AC"), R2 ("ACGT") and R3 ("ACGTACGT") form a clean 3-stratum laminar stack where 16 bases compress into two top-level R3 tokens.'
  },
  {
    id: 'thue-morse',
    name: 'Thue-Morse Fractal',
    sequence: '0110100110010110',
    category: 'Aperiodic Self-Similar',
    tagline: 'Cube-free binary sequence that still reveals rich internal digram symmetries.',
    insight:
      'Even though the Thue-Morse sequence never repeats three identical blocks in a row, Sequitur discovers its recursive complement building blocks.'
  },
  {
    id: 'chorus-verse',
    name: 'Rondo Phrase Structure',
    sequence: 'XYZXWXYZYXWZXYZX',
    category: 'Structural Cadence',
    tagline: 'Uncovers recurring phrases and sub-motifs inside an interwoven sequence.',
    insight:
      'Sequitur acts as an online phrase-structure discovery engine, isolating "XYZ" and "XYZX" without prior knowledge of motif boundaries.'
  }
];

// Curated high-contrast accessible rule palette (pairs light surface, border, and high-contrast text)
export const RULE_PALETTE = [
  {
    name: 'Azure',
    bg: 'bg-[#E0F2FE]',
    bgSubtle: 'bg-[#F0F9FF]',
    border: 'border-[#0284C7]',
    text: 'text-[#0369A1]',
    badgeBg: 'bg-[#0284C7]',
    hex: '#0284C7',
    lightHex: '#E0F2FE'
  },
  {
    name: 'Emerald',
    bg: 'bg-[#D1FAE5]',
    bgSubtle: 'bg-[#ECFDF5]',
    border: 'border-[#059669]',
    text: 'text-[#047857]',
    badgeBg: 'bg-[#059669]',
    hex: '#059669',
    lightHex: '#D1FAE5'
  },
  {
    name: 'Amber',
    bg: 'bg-[#FEF3C7]',
    bgSubtle: 'bg-[#FFFBEB]',
    border: 'border-[#D97706]',
    text: 'text-[#B45309]',
    badgeBg: 'bg-[#D97706]',
    hex: '#D97706',
    lightHex: '#FEF3C7'
  },
  {
    name: 'Rose',
    bg: 'bg-[#FFE4E6]',
    bgSubtle: 'bg-[#FFF1F2]',
    border: 'border-[#E11D48]',
    text: 'text-[#BE123C]',
    badgeBg: 'bg-[#E11D48]',
    hex: '#E11D48',
    lightHex: '#FFE4E6'
  },
  {
    name: 'Teal',
    bg: 'bg-[#CCFBF1]',
    bgSubtle: 'bg-[#F0FDFA]',
    border: 'border-[#0D9488]',
    text: 'text-[#0F766E]',
    badgeBg: 'bg-[#0D9488]',
    hex: '#0D9488',
    lightHex: '#CCFBF1'
  },
  {
    name: 'Indigo',
    bg: 'bg-[#E0E7FF]',
    bgSubtle: 'bg-[#EEF2FF]',
    border: 'border-[#4F46E5]',
    text: 'text-[#4338CA]',
    badgeBg: 'bg-[#4F46E5]',
    hex: '#4F46E5',
    lightHex: '#E0E7FF'
  },
  {
    name: 'Orange',
    bg: 'bg-[#FFEDD5]',
    bgSubtle: 'bg-[#FFF7ED]',
    border: 'border-[#EA580C]',
    text: 'text-[#C2410C]',
    badgeBg: 'bg-[#EA580C]',
    hex: '#EA580C',
    lightHex: '#FFEDD5'
  },
  {
    name: 'Cyan',
    bg: 'bg-[#CFFAFE]',
    bgSubtle: 'bg-[#ECFEFF]',
    border: 'border-[#0891B2]',
    text: 'text-[#0E7490]',
    badgeBg: 'bg-[#0891B2]',
    hex: '#0891B2',
    lightHex: '#CFFAFE'
  }
];

interface RawRule {
  id: string;
  number: number;
  symbols: string[]; // e.g. ['a', 'b', 'R1']
}

/**
 * Generates the complete sequence of micro-step snapshots for a given input string
 * using the exact Nevill-Manning & Witten (1997) Sequitur algorithm.
 */
export function generateSequiturTrace(input: string): GrammarSnapshot[] {
  const snapshots: GrammarSnapshot[] = [];
  let nextRuleNumber = 1;
  let tokenCounter = 1;

  // Map from ruleId ('S', 'R1', ...) to RawRule
  const rawRules = new Map<string, RawRule>();
  rawRules.set('S', { id: 'S', number: 0, symbols: [] });

  const createSnapshot = (
    charIndex: number,
    kind: MicroStepKind,
    title: string,
    explanation: string,
    invariantStatus: GrammarSnapshot['invariantStatus'],
    extra?: {
      highlightDigram?: [string, string];
      highlightRuleId?: string;
      dissolvedRuleId?: string;
      dissolvedIntoRuleId?: string;
    }
  ): GrammarSnapshot => {
    // Compute expansions & depths for all rules
    const memoExpansion = new Map<string, string>();
    const memoDepth = new Map<string, number>();

    const getExpansion = (sym: string): string => {
      if (!sym.startsWith('R') && sym !== 'S') return sym;
      if (memoExpansion.has(sym)) return memoExpansion.get(sym)!;
      const r = rawRules.get(sym);
      if (!r) return sym;
      const exp = r.symbols.map(getExpansion).join('');
      memoExpansion.set(sym, exp);
      return exp;
    };

    const getDepth = (sym: string): number => {
      if (!sym.startsWith('R') && sym !== 'S') return 0;
      if (memoDepth.has(sym)) return memoDepth.get(sym)!;
      const r = rawRules.get(sym);
      if (!r || r.symbols.length === 0) return 0;
      const d = 1 + Math.max(...r.symbols.map(getDepth));
      memoDepth.set(sym, d);
      return d;
    };

    // Count references across S and all active rules
    const refCounts = new Map<string, number>();
    for (const r of rawRules.values()) {
      for (const s of r.symbols) {
        if (s.startsWith('R')) {
          refCounts.set(s, (refCounts.get(s) ?? 0) + 1);
        }
      }
    }

    // Compute exact character spans in S and all intervals for every rule
    const ruleIntervals = new Map<string, [number, number][]>();
    for (const r of rawRules.keys()) {
      if (r !== 'S') ruleIntervals.set(r, []);
    }

    const collectIntervals = (sym: string, startOffset: number): number => {
      const expLen = getExpansion(sym).length;
      if (sym.startsWith('R') && rawRules.has(sym)) {
        const arr = ruleIntervals.get(sym) ?? [];
        arr.push([startOffset, startOffset + expLen - 1]);
        ruleIntervals.set(sym, arr);

        const r = rawRules.get(sym)!;
        let innerOffset = startOffset;
        for (const childSym of r.symbols) {
          innerOffset += collectIntervals(childSym, innerOffset);
        }
      }
      return expLen;
    };

    const rootRaw = rawRules.get('S')!;
    let cursor = 0;
    const rootSymbols: SymbolToken[] = rootRaw.symbols.map((sym, idx) => {
      const len = collectIntervals(sym, cursor);
      const span: [number, number] = [cursor, cursor + len - 1];
      cursor += len;
      return {
        id: `S-${idx}-${sym}`,
        value: sym,
        isRule: sym.startsWith('R'),
        ruleId: sym.startsWith('R') ? sym : undefined,
        charSpan: span
      };
    });

    const activeRules: RuleDefinition[] = [];
    const sortedRuleEntries = Array.from(rawRules.values())
      .filter((r) => r.id !== 'S')
      .sort((a, b) => a.number - b.number);

    for (const r of sortedRuleEntries) {
      const syms: SymbolToken[] = r.symbols.map((sym, idx) => ({
        id: `${r.id}-${idx}-${sym}`,
        value: sym,
        isRule: sym.startsWith('R'),
        ruleId: sym.startsWith('R') ? sym : undefined
      }));
      activeRules.push({
        id: r.id,
        number: r.number,
        symbols: syms,
        refCount: refCounts.get(r.id) ?? 0,
        expansion: getExpansion(r.id),
        depth: getDepth(r.id),
        colorIndex: (r.number - 1) % RULE_PALETTE.length,
        intervals: ruleIntervals.get(r.id) ?? []
      });
    }

    const rootRuleDef: RuleDefinition = {
      id: 'S',
      number: 0,
      symbols: rootSymbols,
      refCount: 1,
      expansion: getExpansion('S'),
      depth: getDepth('S'),
      colorIndex: 0,
      intervals: cursor > 0 ? [[0, cursor - 1]] : []
    };

    // Build digram table for inspection
    const digrams: DigramEntry[] = [];
    const digramCounts = new Map<string, number>();

    const allRuleOrder = [rootRaw, ...sortedRuleEntries];
    for (const r of allRuleOrder) {
      for (let i = 0; i < r.symbols.length - 1; i++) {
        const key = `${r.symbols[i]}|${r.symbols[i + 1]}`;
        // Check for overlapping identical pair at i-1 in the same rule
        if (
          i > 0 &&
          r.symbols[i - 1] === r.symbols[i] &&
          r.symbols[i] === r.symbols[i + 1]
        ) {
          // Only count non-overlapping occurrence if i-1 was already counted and i-2 wasn't
          // Wait: in 'aaa', index 0 is (a,a) and index 1 is overlapping (a,a).
          // In 'aaaa', index 0 is (a,a), index 1 overlaps, index 2 is non-overlapping (a,a).
        }
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
          count: digramCounts.get(key) ?? 1
        });
      }
    }

    const rawLength = Math.max(0, charIndex + 1);
    const rootSymsCount = rootRaw.symbols.length;
    const rulesSymsCount = sortedRuleEntries.reduce((acc, r) => acc + r.symbols.length, 0);
    const totalGrammarSymbols = rootSymsCount + rulesSymsCount;

    return {
      stepIndex: snapshots.length,
      charIndex,
      processedInput: input.slice(0, charIndex + 1),
      fullInput: input,
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
        compressionRatio: rawLength > 0 ? totalGrammarSymbols / rawLength : 1
      }
    };
  };

  // Helper: Find any non-overlapping duplicate digram across all rules (S first, then R1, R2...)
  const findDuplicateDigram = (): {
    first: string;
    second: string;
    loc1: { ruleId: string; index: number };
    loc2: { ruleId: string; index: number };
  } | null => {
    const seen = new Map<string, { ruleId: string; index: number }>();
    const orderedRules = [
      rawRules.get('S')!,
      ...Array.from(rawRules.values())
        .filter((r) => r.id !== 'S')
        .sort((a, b) => a.number - b.number)
    ];

    for (const r of orderedRules) {
      for (let i = 0; i < r.symbols.length - 1; i++) {
        const first = r.symbols[i];
        const second = r.symbols[i + 1];
        const key = `${first}|${second}`;
        const prev = seen.get(key);
        if (prev) {
          // Ensure non-overlapping if in the same rule
          if (prev.ruleId === r.id && Math.abs(prev.index - i) < 2) {
            continue;
          }
          return {
            first,
            second,
            loc1: prev,
            loc2: { ruleId: r.id, index: i }
          };
        } else {
          seen.set(key, { ruleId: r.id, index: i });
        }
      }
    }
    return null;
  };

  // Helper: Find any rule whose reference count is < 2
  const findUnderusedRule = (): {
    underusedRule: RawRule;
    callerRuleId: string;
    callerIndex: number;
  } | null => {
    const counts = new Map<string, { count: number; callerRuleId: string; callerIndex: number }>();
    for (const r of rawRules.values()) {
      if (r.id !== 'S') {
        counts.set(r.id, { count: 0, callerRuleId: 'S', callerIndex: 0 });
      }
    }

    for (const r of rawRules.values()) {
      for (let i = 0; i < r.symbols.length; i++) {
        const sym = r.symbols[i];
        if (sym.startsWith('R') && counts.has(sym)) {
          const info = counts.get(sym)!;
          info.count += 1;
          info.callerRuleId = r.id;
          info.callerIndex = i;
        }
      }
    }

    for (const [ruleId, info] of counts.entries()) {
      if (info.count < 2) {
        return {
          underusedRule: rawRules.get(ruleId)!,
          callerRuleId: info.callerRuleId,
          callerIndex: info.callerIndex
        };
      }
    }
    return null;
  };

  // Initial empty state
  snapshots.push(
    createSnapshot(
      -1,
      'INITIAL',
      'Grammar Initialized',
      'Root sequence S is empty. Append symbols or press Play to watch Sequitur maintain Digram Uniqueness and Rule Utility in real time.',
      { digramUniqueness: 'NOMINAL', ruleUtility: 'NOMINAL' }
    )
  );

  for (let cIdx = 0; cIdx < input.length; cIdx++) {
    const ch = input[cIdx];
    const S = rawRules.get('S')!;
    S.symbols.push(ch);
    tokenCounter++;

    const trailingDigram: [string, string] | undefined =
      S.symbols.length >= 2
        ? [S.symbols[S.symbols.length - 2], S.symbols[S.symbols.length - 1]]
        : undefined;

    // Check immediately if appending caused a collision
    const immediateCollision = findDuplicateDigram();

    snapshots.push(
      createSnapshot(
        cIdx,
        'APPEND_SYMBOL',
        `Append '${ch}' to Root Sequence S`,
        trailingDigram
          ? `Appended terminal '${ch}' at index ${cIdx + 1}, forming trailing digram [${trailingDigram[0]} ${trailingDigram[1]}].`
          : `Appended first terminal '${ch}' to S.`,
        {
          digramUniqueness: immediateCollision ? 'VIOLATED' : 'NOMINAL',
          ruleUtility: 'NOMINAL'
        },
        {
          highlightDigram: trailingDigram
        }
      )
    );

    // Loop to resolve all digram collisions and utility violations
    let safety = 0;
    while (safety++ < 100) {
      const collision = findDuplicateDigram();
      if (collision) {
        const { first, second, loc1, loc2 } = collision;

        // Snapshot highlighting the collision before replacement
        snapshots.push(
          createSnapshot(
            cIdx,
            'DIGRAM_COLLISION',
            `Digram Collision: [${first} ${second}]`,
            `Invariant 1 (Digram Uniqueness) violated: pair [${first} ${second}] appears in ${loc1.ruleId} (pos ${loc1.index + 1}) and ${loc2.ruleId} (pos ${loc2.index + 1}).`,
            {
              digramUniqueness: 'VIOLATED',
              ruleUtility: 'NOMINAL'
            },
            {
              highlightDigram: [first, second]
            }
          )
        );

        // Check if an existing rule already has body exactly [first, second]
        let matchingRule: RawRule | undefined;
        for (const r of rawRules.values()) {
          if (
            r.id !== 'S' &&
            r.symbols.length === 2 &&
            r.symbols[0] === first &&
            r.symbols[1] === second
          ) {
            matchingRule = r;
            break;
          }
        }

        if (matchingRule) {
          // Replace the other occurrence with matchingRule.id
          const targetLoc = loc1.ruleId === matchingRule.id ? loc2 : loc1;
          const targetRule = rawRules.get(targetLoc.ruleId)!;
          targetRule.symbols.splice(targetLoc.index, 2, matchingRule.id);

          const utilityAfter = findUnderusedRule();
          snapshots.push(
            createSnapshot(
              cIdx,
              'REUSE_EXISTING_RULE',
              `Substitute [${first} ${second}] → ${matchingRule.id}`,
              `Rule ${matchingRule.id} already defines [${first} ${second}]. Replaced duplicate in ${targetRule.id} with ${matchingRule.id}, increasing ${matchingRule.id} utility.`,
              {
                digramUniqueness: 'REPAIRED',
                ruleUtility: utilityAfter ? 'VIOLATED' : 'NOMINAL'
              },
              {
                highlightRuleId: matchingRule.id,
                highlightDigram: [first, second]
              }
            )
          );
        } else {
          // Mint a new rule Rn -> first second
          const newRuleId = `R${nextRuleNumber}`;
          const newRule: RawRule = {
            id: newRuleId,
            number: nextRuleNumber,
            symbols: [first, second]
          };
          nextRuleNumber++;
          rawRules.set(newRuleId, newRule);

          // Replace in both locations (if same rule, replace higher index first so lower index stays valid)
          if (loc1.ruleId === loc2.ruleId) {
            const r = rawRules.get(loc1.ruleId)!;
            const high = Math.max(loc1.index, loc2.index);
            const low = Math.min(loc1.index, loc2.index);
            r.symbols.splice(high, 2, newRuleId);
            r.symbols.splice(low, 2, newRuleId);
          } else {
            const r1 = rawRules.get(loc1.ruleId)!;
            const r2 = rawRules.get(loc2.ruleId)!;
            r1.symbols.splice(loc1.index, 2, newRuleId);
            r2.symbols.splice(loc2.index, 2, newRuleId);
          }

          const utilityAfter = findUnderusedRule();
          snapshots.push(
            createSnapshot(
              cIdx,
              'CREATE_NEW_RULE',
              `Mint Rule ${newRuleId} → ${first} ${second}`,
              `Created ${newRuleId} → ${first} ${second} and substituted both occurrences in ${loc1.ruleId} and ${loc2.ruleId}.`,
              {
                digramUniqueness: 'REPAIRED',
                ruleUtility: utilityAfter ? 'VIOLATED' : 'NOMINAL'
              },
              {
                highlightRuleId: newRuleId,
                highlightDigram: [first, second]
              }
            )
          );
        }

        // Immediately check if any rule became underused (utility < 2)
        let utilityLoop = 0;
        while (utilityLoop++ < 20) {
          const underused = findUnderusedRule();
          if (!underused) break;

          const { underusedRule, callerRuleId, callerIndex } = underused;
          const callerRule = rawRules.get(callerRuleId);
          if (callerRule && callerRule.symbols[callerIndex] === underusedRule.id) {
            callerRule.symbols.splice(callerIndex, 1, ...underusedRule.symbols);
          }
          rawRules.delete(underusedRule.id);

          snapshots.push(
            createSnapshot(
              cIdx,
              'DISSOLVE_UNDERUSED_RULE',
              `Dissolve Underused ${underusedRule.id} into ${callerRuleId}`,
              `Invariant 2 (Rule Utility): ${underusedRule.id} dropped to 1 reference after substitution. Inlined [${underusedRule.symbols.join(' ')}] directly into ${callerRuleId}.`,
              {
                digramUniqueness: findDuplicateDigram() ? 'VIOLATED' : 'NOMINAL',
                ruleUtility: 'REPAIRED'
              },
              {
                dissolvedRuleId: underusedRule.id,
                dissolvedIntoRuleId: callerRuleId,
                highlightRuleId: callerRuleId
              }
            )
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
 * Builds a recursive non-tree NestedCapsuleNode list for S or any rule
 * up to a controllable maxUnfurlDepth so users can interactively telescope
 * capsules open in-place without a vertical node-link tree.
 */
export function buildNestedCapsules(
  snapshot: GrammarSnapshot
): NestedCapsuleNode[] {
  const ruleMap = new Map<string, RuleDefinition>();
  for (const r of snapshot.rules) {
    ruleMap.set(r.id, r);
  }

  const buildNode = (
    token: SymbolToken,
    startChar: number,
    depth: number,
    pathPrefix: string
  ): NestedCapsuleNode => {
    if (!token.isRule || !token.ruleId || !ruleMap.has(token.ruleId)) {
      return {
        instanceId: `${pathPrefix}-${token.id}`,
        symbol: token.value,
        isRule: false,
        expansion: token.value,
        startChar,
        endChar: startChar,
        depth
      };
    }

    const r = ruleMap.get(token.ruleId)!;
    let cursor = startChar;
    const children: NestedCapsuleNode[] = r.symbols.map((childTok, idx) => {
      const childNode = buildNode(childTok, cursor, depth + 1, `${pathPrefix}-${r.id}_${idx}`);
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
      endChar: startChar + r.expansion.length - 1,
      depth,
      children
    };
  };

  let rootCursor = 0;
  return snapshot.rootRule.symbols.map((tok, idx) => {
    const node = buildNode(tok, rootCursor, 0, `root_${idx}`);
    rootCursor = node.endChar + 1;
    return node;
  });
}
