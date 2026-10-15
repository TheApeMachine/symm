/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import React, { useState, useEffect, useMemo } from 'react';
import {
  PRESET_SEQUENCES,
  PresetSequence,
  generateSequiturTrace,
  MicroStepKind
} from './engine/sequitur';
import { TelescopingTape } from './components/TelescopingTape';
import { LaminarStrata } from './components/LaminarStrata';
import { RulebookDeck } from './components/RulebookDeck';
import {
  Play,
  Pause,
  SkipBack,
  SkipForward,
  RotateCcw,
  FastForward,
  Plus
} from 'lucide-react';

export default function App() {
  const [selectedPresetId, setSelectedPresetId] = useState<string>(
    PRESET_SEQUENCES[0].id
  );
  const [inputSequence, setInputSequence] = useState<string>(
    PRESET_SEQUENCES[0].sequence
  );
  const [stepIndex, setStepIndex] = useState<number>(0);
  const [isPlaying, setIsPlaying] = useState<boolean>(false);
  const [playbackSpeed, setPlaybackSpeed] = useState<number>(1.25); // steps per second

  // Shared synchronized hover states across all non-tree lenses
  const [hoveredRuleId, setHoveredRuleId] = useState<string | null>(null);
  const [hoveredCharIndex, setHoveredCharIndex] = useState<number | null>(null);
  const [hoveredDigramKey, setHoveredDigramKey] = useState<string | null>(null);

  // Compute full micro-step trace whenever inputSequence changes
  const trace = useMemo(() => {
    const cleaned = inputSequence.replace(/\s+/g, '').slice(0, 36);
    return generateSequiturTrace(cleaned);
  }, [inputSequence]);

  // Start on the final completed step on initial load so the user immediately sees
  // a rich populated grammar, while allowing instant scrubbing or replay
  useEffect(() => {
    setStepIndex(Math.max(0, trace.length - 1));
  }, []);

  // Clamp stepIndex if trace length shrinks
  const safeStepIndex = Math.min(stepIndex, Math.max(0, trace.length - 1));
  const currentSnapshot = trace[safeStepIndex] ?? trace[0];

  // Playback timer
  useEffect(() => {
    if (!isPlaying) return;
    if (safeStepIndex >= trace.length - 1) {
      setIsPlaying(false);
      return;
    }

    const intervalMs = Math.max(180, Math.round(900 / playbackSpeed));
    const timer = window.setInterval(() => {
      setStepIndex((prev) => {
        if (prev >= trace.length - 1) {
          setIsPlaying(false);
          return prev;
        }
        return prev + 1;
      });
    }, intervalMs);

    return () => window.clearInterval(timer);
  }, [isPlaying, safeStepIndex, trace.length, playbackSpeed]);

  const activePreset: PresetSequence | undefined = PRESET_SEQUENCES.find(
    (p) => p.id === selectedPresetId
  );

  const handleSelectPreset = (preset: PresetSequence) => {
    setIsPlaying(false);
    setSelectedPresetId(preset.id);
    setInputSequence(preset.sequence);
    const nextTrace = generateSequiturTrace(preset.sequence);
    setStepIndex(Math.max(0, nextTrace.length - 1));
  };

  const handleCustomInputChange = (raw: string) => {
    const cleaned = raw.replace(/\s+/g, '').slice(0, 36);
    setIsPlaying(false);
    setSelectedPresetId('custom');
    setInputSequence(cleaned);
    const nextTrace = generateSequiturTrace(cleaned);
    setStepIndex(Math.max(0, nextTrace.length - 1));
  };

  const handleAppendSymbol = (ch: string) => {
    if (inputSequence.length >= 36) return;
    const nextSeq = inputSequence + ch;
    setIsPlaying(false);
    setSelectedPresetId('custom');
    setInputSequence(nextSeq);
    const nextTrace = generateSequiturTrace(nextSeq);
    // Jump to the APPEND_SYMBOL step of the newly appended character so the user can watch or play its cascade
    const appendIdx = nextTrace.findIndex(
      (s) => s.charIndex === nextSeq.length - 1 && s.kind === 'APPEND_SYMBOL'
    );
    if (appendIdx !== -1 && appendIdx < nextTrace.length - 1) {
      setStepIndex(appendIdx);
      setIsPlaying(true);
    } else {
      setStepIndex(Math.max(0, nextTrace.length - 1));
    }
  };

  const handleStepNextCharacter = () => {
    setIsPlaying(false);
    const currentChar = currentSnapshot.charIndex;
    const nextCharStep = trace.findIndex(
      (s, idx) => idx > safeStepIndex && s.charIndex > currentChar
    );
    if (nextCharStep !== -1) {
      setStepIndex(nextCharStep);
    } else {
      setStepIndex(trace.length - 1);
    }
  };

  const handleTogglePlay = () => {
    if (safeStepIndex >= trace.length - 1) {
      setStepIndex(0);
      setIsPlaying(true);
    } else {
      setIsPlaying((prev) => !prev);
    }
  };

  // Derive quick-append alphabet buttons from current sequence + standard symbols
  const quickAlphabet = useMemo(() => {
    const unique = Array.from(new Set(inputSequence.split('')));
    if (unique.length >= 2) return unique.slice(0, 6);
    return ['a', 'b', 'c', 'd'];
  }, [inputSequence]);

  const getStepGlyph = (kind: MicroStepKind) => {
    switch (kind) {
      case 'DIGRAM_COLLISION':
        return { symbol: '▲', color: 'bg-[#DC2626] text-white' };
      case 'CREATE_NEW_RULE':
      case 'REUSE_EXISTING_RULE':
        return { symbol: '◆', color: 'bg-[#0284C7] text-white' };
      case 'DISSOLVE_UNDERUSED_RULE':
        return { symbol: '◇', color: 'bg-[#D97706] text-white' };
      case 'APPEND_SYMBOL':
        return { symbol: '●', color: 'bg-slate-700 text-white' };
      default:
        return { symbol: '·', color: 'bg-slate-300 text-slate-700' };
    }
  };

  return (
    <div className="min-h-screen flex flex-col bg-[#F8FAFC] text-[#0F172A]">
      {/* Strict 3-Zone Top Navigation Bar Contract */}
      <header className="flex items-center justify-between gap-8 px-6 py-3.5 bg-white border-b border-slate-200 sticky top-0 z-30">
        {/* Zone 1: Single text element wordmark */}
        <a
          href="#top"
          className="font-display text-lg font-bold tracking-tight text-slate-900 whitespace-nowrap shrink-0"
        >
          Lamina
        </a>

        {/* Zone 2: 4 concise single-line navigation links */}
        <nav className="hidden md:flex items-center gap-6 text-sm font-medium text-slate-600">
          <a
            href="#telescoping-tape"
            className="hover:text-slate-900 hover:underline underline-offset-4 transition-colors whitespace-nowrap shrink-0"
          >
            Telescoping Tape
          </a>
          <a
            href="#laminar-strata"
            className="hover:text-slate-900 hover:underline underline-offset-4 transition-colors whitespace-nowrap shrink-0"
          >
            Laminar Strata
          </a>
          <a
            href="#invariant-rules"
            className="hover:text-slate-900 hover:underline underline-offset-4 transition-colors whitespace-nowrap shrink-0"
          >
            Invariant Rules
          </a>
          <a
            href="#digram-radar"
            className="hover:text-slate-900 hover:underline underline-offset-4 transition-colors whitespace-nowrap shrink-0"
          >
            Digram Index
          </a>
        </nav>

        {/* Zone 3: 1 primary action */}
        <div className="flex items-center shrink-0">
          <button
            type="button"
            onClick={handleTogglePlay}
            className="px-4 py-2 text-xs font-semibold text-white bg-[#0284C7] rounded-lg hover:bg-[#0369A1] transition-colors whitespace-nowrap shrink-0 cursor-pointer"
          >
            {isPlaying
              ? 'Pause Evolution'
              : safeStepIndex >= trace.length - 1
              ? 'Replay Evolution'
              : 'Resume Evolution'}
          </button>
        </div>
      </header>

      {/* Main 1440px Container */}
      <main id="top" className="flex-1 max-w-[1440px] w-full mx-auto px-4 sm:px-6 py-6 space-y-6">
        {/* Top Control & Sequence Selection Workbench */}
        <section className="bg-white border border-slate-200 rounded-xl p-5 shadow-2xs space-y-4">
          <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4 pb-4 border-b border-slate-100">
            <div className="max-w-2xl">
              <div className="flex items-center gap-2 text-xs text-slate-500 font-medium">
                <span>Online Grammar Inference (Nevill-Manning &amp; Witten, 1997)</span>
                <span aria-hidden="true">·</span>
                <span>Non-Tree Linear &amp; Laminar Representation</span>
              </div>
              <h1
                className="font-display text-2xl sm:text-3xl font-bold text-slate-900 tracking-tight mt-1"
                style={{ textWrap: 'balance' }}
              >
                Real-Time Sequitur Grammar Evolution
              </h1>
            </div>

            {/* Curated Preset Sequence Selector Tabs */}
            <div className="flex flex-wrap items-center gap-1.5 bg-slate-100 p-1.5 rounded-lg">
              {PRESET_SEQUENCES.map((preset) => (
                <button
                  key={preset.id}
                  type="button"
                  onClick={() => handleSelectPreset(preset)}
                  className={`px-3 py-1.5 text-xs font-medium rounded-md transition-colors whitespace-nowrap shrink-0 cursor-pointer ${
                    selectedPresetId === preset.id
                      ? 'bg-white text-slate-900 shadow-2xs font-semibold'
                      : 'text-slate-600 hover:text-slate-900'
                  }`}
                >
                  {preset.name}
                </button>
              ))}
            </div>
          </div>

          {/* Live Stream Input + Quick Append Keys + Playback Transport */}
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-4 items-center">
            {/* Stream Input Box & Live Append Keys (5 cols) */}
            <div className="lg:col-span-5 flex flex-col gap-2">
              <div className="flex items-center justify-between text-xs">
                <label
                  htmlFor="sequence-input"
                  className="font-semibold text-slate-700"
                >
                  Input Symbol Stream (Live Editable, max 36 chars):
                </label>
                <span className="font-mono text-slate-500 tabular-nums">
                  {inputSequence.length}/36 chars
                </span>
              </div>

              <div className="flex items-center gap-2">
                <input
                  id="sequence-input"
                  type="text"
                  value={inputSequence}
                  onChange={(e) => handleCustomInputChange(e.target.value)}
                  placeholder="Type symbols (e.g. abcdbcabcd)..."
                  className="flex-1 px-3 py-2 text-sm font-mono bg-slate-50 border border-slate-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-[#0284C7] focus:bg-white"
                />

                {/* One-click Live Append Buttons to watch online reactions */}
                <div className="flex items-center gap-1 shrink-0">
                  {quickAlphabet.map((sym) => (
                    <button
                      key={sym}
                      type="button"
                      onClick={() => handleAppendSymbol(sym)}
                      disabled={inputSequence.length >= 36}
                      title={`Append '${sym}' to end of stream and animate Sequitur invariant repair`}
                      className="inline-flex items-center gap-0.5 px-2.5 py-2 text-xs font-mono font-semibold bg-slate-100 text-slate-800 border border-slate-200 rounded-lg hover:bg-[#E0F2FE] hover:border-[#0284C7] hover:text-[#0369A1] disabled:opacity-40 transition-colors whitespace-nowrap shrink-0 cursor-pointer"
                    >
                      <Plus className="w-3 h-3" />
                      <span>{sym}</span>
                    </button>
                  ))}
                </div>
              </div>
            </div>

            {/* Playback Controls & Speed Slider (7 cols) */}
            <div className="lg:col-span-7 flex flex-col gap-2 bg-slate-50 border border-slate-200/80 rounded-lg p-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                {/* Transport Buttons */}
                <div className="flex items-center gap-1.5">
                  <button
                    type="button"
                    onClick={() => {
                      setIsPlaying(false);
                      setStepIndex(0);
                    }}
                    title="Reset to step 0"
                    className="p-2 text-slate-700 bg-white border border-slate-200 rounded-lg hover:bg-slate-100 transition-colors cursor-pointer"
                  >
                    <RotateCcw className="w-3.5 h-3.5" />
                  </button>

                  <button
                    type="button"
                    onClick={() => {
                      setIsPlaying(false);
                      setStepIndex((prev) => Math.max(0, prev - 1));
                    }}
                    disabled={safeStepIndex <= 0}
                    className="px-3 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-lg hover:bg-slate-100 disabled:opacity-40 transition-colors inline-flex items-center gap-1 whitespace-nowrap shrink-0 cursor-pointer"
                  >
                    <SkipBack className="w-3.5 h-3.5" />
                    <span>Prev Step</span>
                  </button>

                  <button
                    type="button"
                    onClick={handleTogglePlay}
                    className="px-4 py-1.5 text-xs font-semibold text-white bg-[#0284C7] rounded-lg hover:bg-[#0369A1] transition-colors inline-flex items-center gap-1.5 whitespace-nowrap shrink-0 cursor-pointer"
                  >
                    {isPlaying ? (
                      <>
                        <Pause className="w-3.5 h-3.5" />
                        <span>Pause</span>
                      </>
                    ) : (
                      <>
                        <Play className="w-3.5 h-3.5" />
                        <span>
                          {safeStepIndex >= trace.length - 1 ? 'Replay' : 'Play'}
                        </span>
                      </>
                    )}
                  </button>

                  <button
                    type="button"
                    onClick={() => {
                      setIsPlaying(false);
                      setStepIndex((prev) => Math.min(trace.length - 1, prev + 1));
                    }}
                    disabled={safeStepIndex >= trace.length - 1}
                    className="px-3 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-lg hover:bg-slate-100 disabled:opacity-40 transition-colors inline-flex items-center gap-1 whitespace-nowrap shrink-0 cursor-pointer"
                  >
                    <span>Next Step</span>
                    <SkipForward className="w-3.5 h-3.5" />
                  </button>

                  <button
                    type="button"
                    onClick={handleStepNextCharacter}
                    disabled={safeStepIndex >= trace.length - 1}
                    title="Jump to next input character"
                    className="px-2.5 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-lg hover:bg-slate-100 disabled:opacity-40 transition-colors inline-flex items-center gap-1 whitespace-nowrap shrink-0 cursor-pointer"
                  >
                    <span>Next Char</span>
                    <FastForward className="w-3.5 h-3.5" />
                  </button>
                </div>

                {/* Speed Slider with explicit label and numeric value */}
                <div className="flex items-center gap-2">
                  <label
                    htmlFor="speed-slider"
                    className="text-xs font-medium text-slate-600 whitespace-nowrap shrink-0"
                  >
                    Speed:{' '}
                    <span className="font-mono font-semibold text-slate-900 tabular-nums">
                      {playbackSpeed.toFixed(2)}×
                    </span>
                  </label>
                  <input
                    id="speed-slider"
                    type="range"
                    min={0.5}
                    max={3.0}
                    step={0.25}
                    value={playbackSpeed}
                    onChange={(e) => setPlaybackSpeed(Number(e.target.value))}
                    className="w-20 accent-[#0284C7] cursor-pointer"
                  />
                </div>
              </div>

              {/* Interactive Event-Annotated Micro-Step Timeline Scrubber */}
              <div className="pt-1">
                <div className="flex items-center justify-between text-[11px] text-slate-500 font-mono tabular-nums mb-1">
                  <span>
                    Micro-Step {safeStepIndex + 1} of {trace.length} ·{' '}
                    <strong className="text-slate-800">{currentSnapshot.title}</strong>
                  </span>
                  <span>
                    ● Append · ▲ Collision · ◆ Rule Mint/Reuse · ◇ Rule Dissolve
                  </span>
                </div>

                <div className="flex items-center gap-0.5 overflow-x-auto py-1">
                  {trace.map((snap, idx) => {
                    const glyph = getStepGlyph(snap.kind);
                    const isCurrent = idx === safeStepIndex;
                    return (
                      <button
                        key={snap.stepIndex}
                        type="button"
                        onClick={() => {
                          setIsPlaying(false);
                          setStepIndex(idx);
                        }}
                        title={`Step ${idx + 1}: ${snap.title}`}
                        className={`h-5 min-w-[18px] flex-1 rounded-xs font-mono text-[10px] flex items-center justify-center transition-transform cursor-pointer ${
                          isCurrent
                            ? `${glyph.color} ring-2 ring-slate-900 font-bold scale-y-110`
                            : idx < safeStepIndex
                            ? 'bg-slate-200 text-slate-700 hover:bg-slate-300'
                            : 'bg-slate-100 text-slate-400 hover:bg-slate-200'
                        }`}
                      >
                        {glyph.symbol}
                      </button>
                    );
                  })}
                </div>
              </div>
            </div>
          </div>

          {/* Contextual Preset Insight Note */}
          {activePreset && (
            <div className="pt-2 border-t border-slate-100 flex flex-wrap items-center justify-between gap-2 text-xs text-slate-600">
              <div>
                <span className="font-semibold text-slate-900">
                  {activePreset.category} — {activePreset.name}:
                </span>{' '}
                <span>{activePreset.insight}</span>
              </div>
              <div className="font-mono text-slate-500 tabular-nums shrink-0">
                Raw: {currentSnapshot.metrics.rawLength} syms → Grammar:{' '}
                {currentSnapshot.metrics.grammarSymbols} syms
              </div>
            </div>
          )}
        </section>

        {/* Two-Zone Sandbox Layout: Left Interactive Stage (65%) + Right Control & Rulebook Deck (35%) */}
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
          {/* Left Interactive Stage (8 of 12 columns = 66.7%) */}
          <div className="lg:col-span-8 space-y-6 min-w-0">
            <TelescopingTape
              snapshot={currentSnapshot}
              hoveredRuleId={hoveredRuleId}
              onHoverRule={setHoveredRuleId}
              hoveredCharIndex={hoveredCharIndex}
              onHoverChar={setHoveredCharIndex}
              hoveredDigramKey={hoveredDigramKey}
              onHoverDigram={setHoveredDigramKey}
            />

            <LaminarStrata
              snapshot={currentSnapshot}
              hoveredRuleId={hoveredRuleId}
              onHoverRule={setHoveredRuleId}
              hoveredCharIndex={hoveredCharIndex}
              onHoverChar={setHoveredCharIndex}
            />
          </div>

          {/* Right Control & Concept Deck (4 of 12 columns = 33.3%) */}
          <div className="lg:col-span-4 min-w-0">
            <RulebookDeck
              snapshot={currentSnapshot}
              hoveredRuleId={hoveredRuleId}
              onHoverRule={setHoveredRuleId}
              hoveredDigramKey={hoveredDigramKey}
              onHoverDigram={setHoveredDigramKey}
            />
          </div>
        </div>
      </main>
    </div>
  );
}
