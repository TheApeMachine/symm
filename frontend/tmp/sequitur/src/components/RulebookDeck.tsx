import React, { useState } from 'react';
import { GrammarSnapshot, RULE_PALETTE } from '../engine/sequitur';

interface RulebookDeckProps {
  snapshot: GrammarSnapshot;
  hoveredRuleId: string | null;
  onHoverRule: (ruleId: string | null) => void;
  hoveredDigramKey: string | null;
  onHoverDigram: (key: string | null) => void;
}

export const RulebookDeck: React.FC<RulebookDeckProps> = ({
  snapshot,
  hoveredRuleId,
  onHoverRule,
  hoveredDigramKey,
  onHoverDigram
}) => {
  const [digramFilter, setDigramFilter] = useState<'all' | 'root' | 'rules'>('all');

  const filteredDigrams = snapshot.digrams.filter((d) => {
    if (digramFilter === 'root') return d.ruleId === 'S';
    if (digramFilter === 'rules') return d.ruleId !== 'S';
    return true;
  });

  return (
    <div className="flex flex-col gap-5">
      {/* Section 1: Invariant Status & Live Rulebook */}
      <section
        id="invariant-rules"
        className="bg-white border border-slate-200 rounded-xl p-5 shadow-2xs"
      >
        <div className="flex items-center justify-between gap-2 pb-3.5 border-b border-slate-100">
          <div>
            <div className="flex items-center gap-1.5 text-xs text-slate-500 font-medium">
              <span>03. Grammar Production Deck</span>
              <span aria-hidden="true">·</span>
              <span className="font-mono tabular-nums">
                {snapshot.rules.length} active rules
              </span>
            </div>
            <h2 className="text-base font-semibold text-slate-900 mt-0.5">
              Live Rulebook &amp; Utility Invariant
            </h2>
          </div>

          <div className="text-right font-mono text-xs tabular-nums">
            <span className="text-slate-500">Ratio: </span>
            <span className="font-semibold text-slate-900">
              {(snapshot.metrics.compressionRatio * 100).toFixed(0)}%
            </span>
          </div>
        </div>

        {/* Dual-Coded Invariant Status Strip (Never color alone: explicit glyph + label) */}
        <div className="grid grid-cols-2 gap-2.5 mt-3.5">
          <div
            className={`p-2.5 rounded-lg border text-xs ${
              snapshot.invariantStatus.digramUniqueness === 'VIOLATED'
                ? 'bg-[#FEF2F2] border-[#FECACA] text-[#991B1B]'
                : snapshot.invariantStatus.digramUniqueness === 'REPAIRED'
                ? 'bg-[#F0F9FF] border-[#BAE6FD] text-[#0369A1]'
                : 'bg-[#ECFDF5] border-[#A7F3D0] text-[#065F46]'
            }`}
          >
            <div className="text-[11px] opacity-80 font-medium">
              Invariant 1: Digram Uniqueness
            </div>
            <div className="font-mono font-bold mt-0.5 flex items-center gap-1.5">
              {snapshot.invariantStatus.digramUniqueness === 'VIOLATED' ? (
                <span>▲ DUPLICATE PAIR</span>
              ) : snapshot.invariantStatus.digramUniqueness === 'REPAIRED' ? (
                <span>◆ FUSED TO RULE</span>
              ) : (
                <span>● NOMINAL (≤ 1×)</span>
              )}
            </div>
          </div>

          <div
            className={`p-2.5 rounded-lg border text-xs ${
              snapshot.invariantStatus.ruleUtility === 'VIOLATED'
                ? 'bg-[#FFFBEB] border-[#FDE68A] text-[#92400E]'
                : snapshot.invariantStatus.ruleUtility === 'REPAIRED'
                ? 'bg-[#F0F9FF] border-[#BAE6FD] text-[#0369A1]'
                : 'bg-[#ECFDF5] border-[#A7F3D0] text-[#065F46]'
            }`}
          >
            <div className="text-[11px] opacity-80 font-medium">
              Invariant 2: Rule Utility
            </div>
            <div className="font-mono font-bold mt-0.5 flex items-center gap-1.5">
              {snapshot.invariantStatus.ruleUtility === 'VIOLATED' ? (
                <span>▲ REF COUNT &lt; 2</span>
              ) : snapshot.invariantStatus.ruleUtility === 'REPAIRED' ? (
                <span>◆ RULE DISSOLVED</span>
              ) : (
                <span>● NOMINAL (≥ 2×)</span>
              )}
            </div>
          </div>
        </div>

        {/* Dissolved Rule Callout if a rule just dissolved on this micro-step */}
        {snapshot.dissolvedRuleId && (
          <div className="mt-3 px-3 py-2 rounded-lg bg-[#FFFBEB] border border-[#FDE68A] text-xs text-[#92400E] font-mono">
            ▲ {snapshot.dissolvedRuleId} dropped to 1× utility and was inlined directly into{' '}
            {snapshot.dissolvedIntoRuleId}.
          </div>
        )}

        {/* Active Production Rules List */}
        <div className="mt-4 space-y-2.5">
          {/* Root S Summary Row */}
          <div className="p-3 rounded-lg bg-slate-50 border border-slate-200/80 flex items-center justify-between gap-3">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-mono text-xs font-bold text-slate-900">S →</span>
                <div className="flex flex-wrap items-center gap-1 font-mono text-xs">
                  {snapshot.rootRule.symbols.length === 0 ? (
                    <span className="text-slate-400 italic">ε (empty)</span>
                  ) : (
                    snapshot.rootRule.symbols.map((tok) => {
                      const r = tok.ruleId
                        ? snapshot.rules.find((x) => x.id === tok.ruleId)
                        : undefined;
                      const pal = r ? RULE_PALETTE[r.colorIndex] : null;
                      return (
                        <span
                          key={tok.id}
                          onMouseEnter={() => tok.ruleId && onHoverRule(tok.ruleId)}
                          onMouseLeave={() => onHoverRule(null)}
                          className={`px-1.5 py-0.5 rounded border text-xs ${
                            pal
                              ? `${pal.bg} ${pal.border} ${pal.text} font-bold cursor-pointer`
                              : 'bg-white border-slate-200 text-slate-700'
                          }`}
                        >
                          {tok.value}
                        </span>
                      );
                    })
                  )}
                </div>
              </div>
            </div>
            <span className="font-mono text-[11px] text-slate-500 whitespace-nowrap shrink-0 tabular-nums">
              Root Stream
            </span>
          </div>

          {snapshot.rules.length === 0 ? (
            <div className="py-6 text-center text-xs text-slate-400 font-mono">
              No active sub-rules yet. When any digram repeats, R1 will appear here.
            </div>
          ) : (
            snapshot.rules.map((rule) => {
              const palette = RULE_PALETTE[rule.colorIndex];
              const isHighlighted =
                hoveredRuleId === rule.id || snapshot.highlightRuleId === rule.id;
              const isUnderused = rule.refCount < 2;

              return (
                <div
                  key={rule.id}
                  onMouseEnter={() => onHoverRule(rule.id)}
                  onMouseLeave={() => onHoverRule(null)}
                  className={`p-3 rounded-lg border transition-all cursor-pointer ${
                    palette.bgSubtle
                  } ${palette.border} ${
                    isHighlighted ? 'ring-2 ring-[#0284C7] shadow-xs' : ''
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <span
                        className={`font-mono text-xs font-bold px-1.5 py-0.5 rounded ${palette.bg} ${palette.text} border ${palette.border}`}
                      >
                        {rule.id}
                      </span>
                      <span className="font-mono text-xs text-slate-400">→</span>
                      <div className="flex items-center gap-1 font-mono text-xs">
                        {rule.symbols.map((sym) => {
                          const childRule = sym.ruleId
                            ? snapshot.rules.find((r) => r.id === sym.ruleId)
                            : undefined;
                          const childPal = childRule
                            ? RULE_PALETTE[childRule.colorIndex]
                            : null;
                          return (
                            <span
                              key={sym.id}
                              className={`px-1.5 py-0.5 rounded border font-semibold ${
                                childPal
                                  ? `${childPal.bg} ${childPal.border} ${childPal.text}`
                                  : 'bg-white border-slate-200 text-slate-800'
                              }`}
                            >
                              {sym.value}
                            </span>
                          );
                        })}
                      </div>
                    </div>

                    {/* Utility Reference Counter */}
                    <div
                      className={`font-mono text-xs font-semibold tabular-nums flex items-center gap-1 ${
                        isUnderused ? 'text-[#D97706]' : 'text-[#059669]'
                      }`}
                      title={`Rule ${rule.id} is referenced ${rule.refCount} times across the grammar (minimum 2 required by Rule Utility)`}
                    >
                      <span>{isUnderused ? '▲' : '●'}</span>
                      <span>Utility: {rule.refCount}×</span>
                    </div>
                  </div>

                  {/* Expansion & Span Metadata Row (Zero-pill clean text with · separators) */}
                  <div className="mt-2 pt-2 border-t border-slate-200/60 flex items-center justify-between gap-2 text-xs text-slate-600 font-mono tabular-nums">
                    <span>
                      Expands to: <strong className="text-slate-900">“{rule.expansion}”</strong> (len{' '}
                      {rule.expansion.length})
                    </span>
                    <span className="text-[11px] text-slate-500">
                      Spans:{' '}
                      {rule.intervals
                        .map(([s, e]) => `[${s + 1}..${e + 1}]`)
                        .join(' · ')}
                    </span>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </section>

      {/* Section 2: Live Digram Index Radar */}
      <section
        id="digram-radar"
        className="bg-white border border-slate-200 rounded-xl p-5 shadow-2xs"
      >
        <div className="flex flex-wrap items-center justify-between gap-2 pb-3.5 border-b border-slate-100">
          <div>
            <div className="flex items-center gap-1.5 text-xs text-slate-500 font-medium">
              <span>04. Digram Index Table</span>
              <span aria-hidden="true">·</span>
              <span className="font-mono tabular-nums">
                {snapshot.digrams.length} adjacent pairs
              </span>
            </div>
            <h2 className="text-base font-semibold text-slate-900 mt-0.5">
              Live Digram Uniqueness Index
            </h2>
          </div>

          {/* Interactive Filter Controls (Functional buttons) */}
          <div className="flex items-center gap-1 p-1 bg-slate-100 rounded-lg">
            <button
              type="button"
              onClick={() => setDigramFilter('all')}
              className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors whitespace-nowrap shrink-0 ${
                digramFilter === 'all'
                  ? 'bg-white text-slate-900 shadow-2xs'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              All ({snapshot.digrams.length})
            </button>
            <button
              type="button"
              onClick={() => setDigramFilter('root')}
              className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors whitespace-nowrap shrink-0 ${
                digramFilter === 'root'
                  ? 'bg-white text-slate-900 shadow-2xs'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              In S
            </button>
            <button
              type="button"
              onClick={() => setDigramFilter('rules')}
              className={`px-2.5 py-1 text-xs font-medium rounded-md transition-colors whitespace-nowrap shrink-0 ${
                digramFilter === 'rules'
                  ? 'bg-white text-slate-900 shadow-2xs'
                  : 'text-slate-600 hover:text-slate-900'
              }`}
            >
              In Rules
            </button>
          </div>
        </div>

        <p className="mt-2.5 text-xs text-slate-500 leading-relaxed">
          Sequitur maintains a hash index of every adjacent symbol pair{' '}
          <code className="font-mono text-slate-700">[A B]</code>. Whenever a second
          non-overlapping pair appears, both coalesce into a grammar rule.
        </p>

        {filteredDigrams.length === 0 ? (
          <div className="py-6 text-center text-xs text-slate-400 font-mono">
            No digrams in this view yet.
          </div>
        ) : (
          <div className="mt-3 grid grid-cols-2 sm:grid-cols-3 gap-2 max-h-[240px] overflow-y-auto pr-1">
            {filteredDigrams.map((d, idx) => {
              const isCollision =
                d.count > 1 ||
                (snapshot.highlightDigram &&
                  snapshot.highlightDigram[0] === d.first &&
                  snapshot.highlightDigram[1] === d.second);
              const isHovered = hoveredDigramKey === d.key;

              return (
                <button
                  key={`${d.ruleId}-${d.position}-${idx}`}
                  type="button"
                  onMouseEnter={() => onHoverDigram(d.key)}
                  onMouseLeave={() => onHoverDigram(null)}
                  className={`p-2 rounded-lg border text-left font-mono transition-colors flex items-center justify-between gap-1.5 ${
                    isCollision
                      ? 'bg-[#FEF2F2] border-[#DC2626] text-[#991B1B] font-bold'
                      : isHovered
                      ? 'bg-[#E0F2FE] border-[#0284C7] text-[#0369A1]'
                      : 'bg-slate-50/80 border-slate-200/80 text-slate-800 hover:border-slate-300'
                  }`}
                >
                  <div className="flex items-center gap-1 text-xs font-semibold truncate">
                    <span>{d.first}</span>
                    <span className="opacity-40">·</span>
                    <span>{d.second}</span>
                  </div>
                  <div className="text-[10px] opacity-75 whitespace-nowrap shrink-0 tabular-nums">
                    {d.ruleId}@{d.position + 1}
                    {d.count > 1 ? ' ▲2×' : ''}
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
};
