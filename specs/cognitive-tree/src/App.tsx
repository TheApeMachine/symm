import React, { useState, useEffect, useMemo, useRef } from 'react';
import { CognitiveModel, BeamPath } from './lib/CognitiveModel';
import { motion, AnimatePresence } from 'motion/react';
import { Brain, Sparkles, Network, Activity, Database, Plus, Settings2, Terminal, Flame, Moon, GitBranch } from 'lucide-react';
import { cn } from './lib/utils';

const initialData = {
  "Truck": ["blue_cab_big_wheel", "blue_cab_flat_bed", "white_cab_big_wheel", "red_cab_flat_bed", "heavy_duty_truck", "diesel_engine_roar"],
  "Car": ["blue_hood_small_tire", "red_hood_small_tire", "blue_hood_spoiler", "white_hood_small_tire", "fast_sports_car", "electric_sedan"],
  "Bike": ["red_tank_two_wheel", "black_tank_two_wheel", "blue_tank_two_wheel", "mountain_bike_tires", "carbon_fiber_frame"]
};

const colorPalette = [
  { bg: 'bg-orange-500', text: 'text-orange-400', border: 'border-orange-500/50' },
  { bg: 'bg-cyan-500', text: 'text-cyan-400', border: 'border-cyan-500/50' },
  { bg: 'bg-purple-500', text: 'text-purple-400', border: 'border-purple-500/50' },
  { bg: 'bg-emerald-500', text: 'text-emerald-400', border: 'border-emerald-500/50' },
  { bg: 'bg-rose-500', text: 'text-rose-400', border: 'border-rose-500/50' },
  { bg: 'bg-amber-500', text: 'text-amber-400', border: 'border-amber-500/50' },
];

export default function App() {
  const [model] = useState(() => {
    const m = new CognitiveModel();
    for (const [label, sequences] of Object.entries(initialData)) {
      sequences.forEach(seq => m.train(seq, label));
    }
    return m;
  });

  const [input, setInput] = useState('blue_');
  const [temperature, setTemperature] = useState(0.5);
  const [teachInput, setTeachInput] = useState('');
  const [teachClass, setTeachClass] = useState('Truck');
  const [newClassName, setNewClassName] = useState('');
  const [trainingCount, setTrainingCount] = useState(0); // Used to trigger symbol re-extraction
  const [isSleeping, setIsSleeping] = useState(false);
  const [consolidatedDreams, setConsolidatedDreams] = useState<{dream: string, label: string}[]>([]);
  const [beams, setBeams] = useState<{sequence: string, score: number}[]>([]);

  useEffect(() => {
    if (!isSleeping) return;
    
    const interval = setInterval(() => {
      const result = model.remSleepTick(Math.max(0.7, temperature));
      if (result) {
        setConsolidatedDreams(prev => [result, ...prev].slice(0, 5));
        setTrainingCount(c => c + 1);
      }
    }, 1500);
    
    return () => clearInterval(interval);
  }, [isSleeping, model, temperature]);

  // Derived state
  const { activeContext, scores, contributions, winner, runnerUp, dreams, nextProbs, surprisals, classList, posteriors, attendedWords } = useMemo(() => {
    const activeContext = model.tokenize(input).join("_");
    const { scores, contributions } = model.classify(input);
    
    let currentWinner = '';
    let maxScore = -1;
    let currentRunnerUp = '';
    let secondMax = -1;
    
    for (const [label, score] of Object.entries(scores)) {
      if (score > maxScore) {
        secondMax = maxScore;
        currentRunnerUp = currentWinner;
        maxScore = score;
        currentWinner = label;
      } else if (score > secondMax) {
        secondMax = score;
        currentRunnerUp = label;
      }
    }

    const dreams = [];
    let nextProbs: {token: string, prob: number}[] = [];
    if (currentWinner) {
      for (let i = 0; i < 3; i++) {
        dreams.push(model.generateDream(input, currentWinner, temperature, 15));
      }
      nextProbs = model.getNextProbabilities(input, currentWinner, temperature).slice(0, 5);
    }

    const surprisals = model.getSurprisal(input);
    const classList = Array.from(model.classes);
    const posteriors = model.getPosteriorsOverTime(input);
    const attendedWords = model.getAttentionContext(input);
    
    return { activeContext, scores, contributions, winner: currentWinner, runnerUp: currentRunnerUp, dreams, nextProbs, surprisals, classList, posteriors, attendedWords };
  }, [input, temperature, model, trainingCount]);

  // Cached symbols
  const symbols = useMemo(() => {
    return model.extractSymbols();
  }, [model, trainingCount]);

  // Dynamic class colors
  const classColors = useMemo(() => {
    const colors: Record<string, typeof colorPalette[0]> = {};
    classList.forEach((c, i) => {
      colors[c] = colorPalette[i % colorPalette.length];
    });
    return colors;
  }, [classList]);

  // Smooth scores for animation
  const [smoothScores, setSmoothScores] = useState<Record<string, number>>({});

  useEffect(() => {
    let animationFrame: number;
    const lerp = (start: number, end: number, factor: number) => start + (end - start) * factor;

    const animate = () => {
      setSmoothScores(prev => {
        const next = { ...prev };
        let changed = false;
        for (const label of classList) {
          const target = scores[label] || 0;
          const current = prev[label] || 0;
          if (Math.abs(target - current) > 0.01) {
            next[label] = lerp(current, target, 0.1);
            changed = true;
          } else {
            next[label] = target;
          }
        }
        if (changed) animationFrame = requestAnimationFrame(animate);
        return next;
      });
    };
    animationFrame = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(animationFrame);
  }, [scores, classList]);

  const handleTeach = (e: React.FormEvent) => {
    e.preventDefault();
    const targetClass = teachClass === 'NEW_CLASS' ? newClassName : teachClass;
    if (teachInput.length > 2 && targetClass.trim().length > 0) {
      model.train(teachInput, targetClass.trim());
      setTeachInput('');
      if (teachClass === 'NEW_CLASS') {
        setTeachClass(targetClass.trim());
        setNewClassName('');
      }
      setTrainingCount(c => c + 1);
    }
  };

  const handleExperience = () => {
    if (!input.trim()) return;
    const result = model.experience(input);
    setTrainingCount(c => c + 1);
    
    // Briefly flash the input to show it was processed
    const originalInput = input;
    setInput(`[Learned: ${result.label} | Rate: ${result.learningRate.toFixed(2)}]`);
    setTimeout(() => setInput(originalInput), 1000);
  };

  const getSurprisalColor = (val: number) => {
    // val is typically between 0 and 10
    const intensity = Math.min(val / 8, 1);
    return `rgba(239, 68, 68, ${intensity * 0.8})`; // Red heatmap
  };

  return (
    <div className="min-h-screen bg-slate-950 text-slate-300 font-mono selection:bg-cyan-900 selection:text-cyan-100 flex flex-col">
      {/* Header */}
      <header className="border-b border-slate-800 bg-slate-950/50 backdrop-blur-md sticky top-0 z-10">
        <div className="max-w-7xl mx-auto px-4 h-16 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-cyan-500/20 flex items-center justify-center border border-cyan-500/50">
              <Brain className="w-5 h-5 text-cyan-400" />
            </div>
            <h1 className="text-xl font-bold bg-gradient-to-r from-cyan-400 to-purple-400 bg-clip-text text-transparent">
              Cognitive Engine v6.0
            </h1>
          </div>
          <div className="flex items-center gap-6 text-sm">
            <div className="flex items-center gap-2">
              <Network className="w-4 h-4 text-slate-500" />
              <span className="text-slate-400">Nodes:</span>
              <span className="text-cyan-400 font-semibold">{model.nodeCount}</span>
            </div>
            <div className="flex items-center gap-2">
              <Database className="w-4 h-4 text-slate-500" />
              <span className="text-slate-400">Vocabulary:</span>
              <span className="text-purple-400 font-semibold">{symbols.length}</span>
            </div>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="flex-1 max-w-7xl mx-auto w-full p-4 grid grid-cols-1 lg:grid-cols-12 gap-6">
        
        {/* Left Column: Controls */}
        <div className="lg:col-span-3 space-y-6">
          <div className="bg-slate-900/50 border border-slate-800 rounded-xl p-5 space-y-4">
            <div className="flex items-center gap-2 text-slate-200 font-semibold mb-2">
              <Terminal className="w-4 h-4 text-emerald-400" />
              Sensory Input
            </div>
            <input
              type="text"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              className="w-full bg-slate-950 border border-slate-700 rounded-lg px-4 py-2.5 text-emerald-400 focus:outline-none focus:border-emerald-500 focus:ring-1 focus:ring-emerald-500/50 transition-all font-mono"
              placeholder="Type sequence..."
            />
            
            <div className="pt-4 border-t border-slate-800">
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-2 text-slate-200 font-semibold">
                  <Settings2 className="w-4 h-4 text-orange-400" />
                  Thermodynamic Temp
                </div>
                <span className="text-orange-400 text-sm">{temperature.toFixed(2)}</span>
              </div>
              <input
                type="range"
                min="0"
                max="2"
                step="0.05"
                value={temperature}
                onChange={(e) => setTemperature(parseFloat(e.target.value))}
                className="w-full accent-orange-500"
              />
              <div className="flex justify-between text-xs text-slate-500 mt-1">
                <span>Strict</span>
                <span>Creative</span>
              </div>
            </div>
          </div>

          <div className="bg-slate-900/50 border border-slate-800 rounded-xl p-5 space-y-4">
            <div className="flex items-center gap-2 text-slate-200 font-semibold mb-2">
              <Plus className="w-4 h-4 text-purple-400" />
              Neuroplasticity (Teach)
            </div>
            <form onSubmit={handleTeach} className="space-y-3">
              <input
                type="text"
                value={teachInput}
                onChange={(e) => setTeachInput(e.target.value)}
                className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-slate-300 focus:outline-none focus:border-purple-500 transition-all text-sm"
                placeholder="New sequence..."
              />
              <div className="flex gap-2">
                <select
                  value={teachClass}
                  onChange={(e) => setTeachClass(e.target.value)}
                  className="bg-slate-950 border border-slate-700 rounded-lg px-2 py-2 text-slate-300 focus:outline-none focus:border-purple-500 text-sm flex-1"
                >
                  {classList.map(c => (
                    <option key={c} value={c}>{c}</option>
                  ))}
                  <option value="NEW_CLASS">+ New Class</option>
                </select>
                <button
                  type="submit"
                  disabled={teachInput.length <= 2}
                  className="bg-purple-500/20 text-purple-400 border border-purple-500/50 rounded-lg px-4 py-2 text-sm font-semibold hover:bg-purple-500/30 disabled:opacity-50 disabled:cursor-not-allowed transition-all"
                >
                  Learn
                </button>
              </div>
              {teachClass === 'NEW_CLASS' && (
                <input
                  type="text"
                  placeholder="Class name..."
                  value={newClassName}
                  onChange={(e) => setNewClassName(e.target.value)}
                  className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-slate-300 focus:outline-none focus:border-purple-500 transition-all text-sm mt-2"
                  autoFocus
                />
              )}
            </form>
            <div className="pt-4 border-t border-slate-800">
              <button
                onClick={handleExperience}
                disabled={input.length <= 2}
                className="w-full bg-emerald-500/20 text-emerald-400 border border-emerald-500/50 rounded-lg px-4 py-2 text-sm font-semibold hover:bg-emerald-500/30 disabled:opacity-50 disabled:cursor-not-allowed transition-all flex items-center justify-center gap-2"
              >
                <Sparkles className="w-4 h-4" />
                Unsupervised Learn (from Input)
              </button>
            </div>
          </div>
          <div className="bg-slate-900/50 border border-slate-800 rounded-xl p-5 space-y-4">
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2 text-slate-200 font-semibold">
                <Moon className="w-4 h-4 text-indigo-400" />
                REM Sleep (Consolidation)
              </div>
              <button
                onClick={() => setIsSleeping(!isSleeping)}
                className={cn(
                  "px-3 py-1 rounded-md text-xs font-bold transition-colors",
                  isSleeping 
                    ? "bg-indigo-500 text-white shadow-[0_0_10px_rgba(99,102,241,0.5)]" 
                    : "bg-slate-800 text-slate-400 hover:bg-slate-700"
                )}
              >
                {isSleeping ? "WAKE" : "SLEEP"}
              </button>
            </div>
            
            <AnimatePresence>
              {consolidatedDreams.map((d, i) => (
                <motion.div
                  key={i + d.dream}
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: 'auto' }}
                  exit={{ opacity: 0, height: 0 }}
                  className="text-xs font-mono text-indigo-300/80 border-l-2 border-indigo-500/30 pl-2 py-1"
                >
                  Learned: [{d.label}] {d.dream}
                </motion.div>
              ))}
            </AnimatePresence>
            {consolidatedDreams.length === 0 && isSleeping && (
              <div className="text-indigo-400/50 text-xs italic animate-pulse">Dreaming...</div>
            )}
          </div>

          {/* Episodic Memory Buffer */}
          <div className="bg-slate-900/50 border border-slate-800 rounded-xl p-5 space-y-4">
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2 text-slate-200 font-semibold">
                <Database className="w-4 h-4 text-emerald-400" />
                Episodic Buffer (RAG)
              </div>
              <span className="text-xs text-slate-500">{model.episodicBuffer.length} / {model.maxEpisodicSize}</span>
            </div>
            
            <div className="space-y-2 max-h-[200px] overflow-y-auto pr-2 custom-scrollbar">
              <AnimatePresence>
                {model.episodicBuffer.map((ep) => (
                  <motion.div
                    key={ep.id}
                    initial={{ opacity: 0, x: -10 }}
                    animate={{ opacity: 1, x: 0 }}
                    className="text-xs font-mono bg-slate-950/50 border border-slate-800 rounded p-2 flex flex-col gap-1"
                  >
                    <div className="flex justify-between text-slate-500">
                      <span>[{ep.label}]</span>
                      <span>t={ep.timestamp}</span>
                    </div>
                    <div className="text-emerald-400/80 break-all">
                      {ep.tokens.join('_')}
                    </div>
                  </motion.div>
                ))}
              </AnimatePresence>
              {model.episodicBuffer.length === 0 && (
                <div className="text-slate-500 text-xs italic text-center py-4">Buffer empty. Experience something!</div>
              )}
            </div>
          </div>
        </div>

        {/* Middle Column: Active State & Probabilities */}
        <div className="lg:col-span-5 space-y-6">
          <div className="bg-slate-900/50 border border-slate-800 rounded-xl p-5 min-h-[200px]">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2 text-slate-200 font-semibold">
                <Activity className="w-4 h-4 text-cyan-400" />
                Active Context & Surprisal
              </div>
              <div className="flex items-center gap-1 text-xs text-slate-500">
                <Flame className="w-3 h-3 text-red-500/70" />
                Heat = Surprise
              </div>
            </div>
            
            <div className="flex flex-wrap gap-1 mb-6">
              {surprisals.map((item, i) => {
                const isActive = true;
                const surprisal = item.surprisal || 0;
                const token = item.token;
                
                return (
                  <motion.div
                    key={i}
                    initial={{ opacity: 0, scale: 0.8 }}
                    animate={{ opacity: 1, scale: 1 }}
                    className={cn(
                      "relative px-2 h-10 flex items-center justify-center rounded border text-lg font-bold transition-colors overflow-hidden whitespace-nowrap",
                      isActive 
                        ? "border-cyan-500/50 text-cyan-100 shadow-[0_0_10px_rgba(6,182,212,0.2)]" 
                        : "border-slate-700/50 text-slate-500"
                    )}
                  >
                    {/* Surprisal Heatmap Background */}
                    <div 
                      className="absolute inset-0 z-0" 
                      style={{ backgroundColor: getSurprisalColor(surprisal) }}
                    />
                    <span className="relative z-10">{token === ' ' ? '␣' : token}</span>
                    
                    {/* Tooltip for surprisal */}
                    <div className="absolute -top-8 left-1/2 -translate-x-1/2 bg-slate-800 text-xs px-1.5 py-0.5 rounded opacity-0 hover:opacity-100 transition-opacity pointer-events-none z-20 whitespace-nowrap">
                      {surprisal.toFixed(1)} bits
                    </div>
                  </motion.div>
                );
              })}
              {input.length === 0 && (
                <div className="text-slate-500 italic text-sm py-2">Awaiting sensory input...</div>
              )}
            </div>

            <div className="mb-6 pt-4 border-t border-slate-800">
              <div className="text-sm font-semibold text-slate-400 mb-2">Lexical Remapping (Edit & Co-occurrence)</div>
              <div className="flex flex-wrap gap-2">
                {attendedWords.map((w, i) => (
                  <div key={i} className="flex items-center gap-1 text-xs font-mono bg-slate-800/50 px-2 py-1 rounded border border-slate-700/50">
                    <span className="text-slate-400">{w.original}</span>
                    {w.original !== w.mapped && (
                      <>
                        <span className="text-slate-600">→</span>
                        <span className="text-emerald-400">{w.mapped}</span>
                        <span className="text-emerald-500/50">({(w.similarity * 100).toFixed(0)}%)</span>
                      </>
                    )}
                  </div>
                ))}
                {attendedWords.length === 0 && (
                  <span className="text-slate-500 text-xs italic">No semantic tokens attended.</span>
                )}
              </div>
            </div>

            {winner && runnerUp && (
              <div className="mb-6 pt-4 border-t border-slate-800">
                <div className="text-sm font-semibold text-slate-400 mb-2">Contrastive Explanations ({winner} vs {runnerUp})</div>
                <div className="space-y-2">
                  {contributions[winner]?.map((c, i) => {
                    const diff = c.logProb - (contributions[runnerUp]?.[i]?.logProb || 0);
                    if (Math.abs(diff) < 0.1) return null;
                    return (
                      <div key={i} className="flex items-center justify-between text-xs font-mono bg-slate-800/30 p-1.5 rounded border border-slate-700/50">
                        <span className="text-slate-300">{c.token}</span>
                        <span className={diff > 0 ? "text-emerald-400" : "text-rose-400"}>
                          {diff > 0 ? "+" : ""}{diff.toFixed(2)} bits
                        </span>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}

            <div className="mb-6 pt-4 border-t border-slate-800">
              <div className="flex items-center justify-between mb-2">
                <div className="text-sm font-semibold text-slate-400">Multi-Hop Reasoning (Beam Search)</div>
                <button
                  onClick={() => {
                    const b = model.beamSearch(input, 3, 5);
                    setBeams(b);
                  }}
                  className="px-2 py-1 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded text-xs flex items-center gap-1 transition-colors"
                >
                  <GitBranch className="w-3 h-3" />
                  Reason
                </button>
              </div>
              <div className="space-y-2">
                {beams.map((beam, i) => (
                  <div key={i} className="flex items-center justify-between bg-slate-800/30 p-2 rounded border border-slate-700/50">
                    <span className="text-slate-300 font-mono text-sm break-all">{beam.sequence}</span>
                    <span className="text-slate-500 text-xs font-mono">score: {beam.score.toFixed(2)}</span>
                  </div>
                ))}
                {beams.length === 0 && (
                  <span className="text-slate-500 text-xs italic">Click 'Reason' to run multi-hop inference.</span>
                )}
              </div>
            </div>

            <div className="space-y-3">
              <div className="text-sm font-semibold text-slate-400 mb-2">Next Token Probabilities (Given {winner || '?'})</div>
              <div className="space-y-2">
                <AnimatePresence mode="popLayout">
                  {nextProbs.map((item) => (
                    <motion.div
                      key={item.token}
                      layout
                      initial={{ opacity: 0, x: -20 }}
                      animate={{ opacity: 1, x: 0 }}
                      exit={{ opacity: 0, scale: 0.9 }}
                      className="flex items-center gap-3"
                    >
                      <div className="px-2 h-8 rounded bg-slate-800 flex items-center justify-center font-bold text-slate-300 border border-slate-700 whitespace-nowrap">
                        {item.token === ' ' ? '␣' : item.token}
                      </div>
                      <div className="flex-1 h-2 bg-slate-800 rounded-full overflow-hidden">
                        <motion.div
                          className="h-full bg-emerald-500"
                          initial={{ width: 0 }}
                          animate={{ width: `${item.prob * 100}%` }}
                          transition={{ type: "spring", bounce: 0, duration: 0.5 }}
                        />
                      </div>
                      <div className="w-12 text-right text-xs text-slate-400 font-mono">
                        {(item.prob * 100).toFixed(1)}%
                      </div>
                    </motion.div>
                  ))}
                </AnimatePresence>
                {nextProbs.length === 0 && (
                  <div className="text-slate-500 text-sm italic">No predictions available.</div>
                )}
              </div>
            </div>
          </div>

          <div className="bg-slate-900/50 border border-slate-800 rounded-xl p-5">
            <div className="flex items-center gap-2 text-slate-200 font-semibold mb-4">
              <Sparkles className="w-4 h-4 text-purple-400" />
              Stateful Subconscious Dreams
            </div>
            <div className="space-y-3">
              {dreams.map((dream, i) => (
                <motion.div
                  key={i}
                  initial={{ opacity: 0, y: 10 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: i * 0.1 }}
                  className="p-3 rounded-lg bg-slate-950 border border-slate-800 font-mono text-sm break-all"
                >
                  <span className={cn("font-bold mr-2", classColors[winner]?.text || "text-slate-400")}>[{winner}]</span>
                  <span className="text-slate-300">{dream}</span>
                </motion.div>
              ))}
              {dreams.length === 0 && (
                <div className="text-slate-500 text-sm italic">Need more context to dream...</div>
              )}
            </div>
          </div>
        </div>

        {/* Right Column: Basins & Symbols */}
        <div className="lg:col-span-4 space-y-6">
          <div className="bg-slate-900/50 border border-slate-800 rounded-xl p-5">
            <div className="flex items-center justify-between mb-6">
              <div className="flex items-center gap-2 text-slate-200 font-semibold">
                <Network className="w-4 h-4 text-orange-400" />
                Normalized Attractor Basins
              </div>
            </div>
            
            {posteriors.length > 1 && (
              <div className="mb-6 h-24 w-full border-b border-slate-800/50 relative">
                <svg width="100%" height="100%" viewBox="0 0 100 100" preserveAspectRatio="none" className="overflow-visible">
                  {classList.map(label => {
                    const points = posteriors.map((p, i) => {
                      const x = (i / (posteriors.length - 1)) * 100;
                      const y = 100 - (p[label] || 0);
                      return `${x},${y}`;
                    }).join(' L ');
                    
                    const colors = classColors[label] || colorPalette[0];
                    
                    return (
                      <path
                        key={label}
                        d={`M ${points}`}
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        vectorEffect="non-scaling-stroke"
                        className={cn("transition-all duration-300 opacity-70", colors.text)}
                      />
                    );
                  })}
                </svg>
                <div className="absolute top-0 right-0 text-[10px] text-slate-500 bg-slate-950/80 px-1 rounded">Trajectory</div>
              </div>
            )}

            <div className="space-y-5">
              {classList.map(label => {
                const score = smoothScores[label] || 0;
                const isWinner = label === winner;
                const colors = classColors[label] || colorPalette[0];
                
                return (
                  <div key={label} className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span className={cn("font-semibold", isWinner ? colors.text : "text-slate-400")}>
                        {label}
                      </span>
                      <span className="text-slate-500 font-mono">{score.toFixed(1)}%</span>
                    </div>
                    <div className="h-3 bg-slate-950 rounded-full overflow-hidden border border-slate-800">
                      <motion.div
                        className={cn("h-full", colors.bg)}
                        initial={{ width: 0 }}
                        animate={{ width: `${score}%` }}
                        transition={{ type: "spring", bounce: 0, duration: 0.5 }}
                      />
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          <div className="bg-slate-900/50 border border-slate-800 rounded-xl p-5">
            <div className="flex items-center gap-2 text-slate-200 font-semibold mb-4">
              <Database className="w-4 h-4 text-blue-400" />
              Discriminative Concepts
            </div>
            <div className="flex flex-wrap gap-2 max-h-[300px] overflow-y-auto pr-2 custom-scrollbar">
              <AnimatePresence>
                {symbols.slice(0, 30).map((item, i) => {
                  const isActive = input.includes(item.symbol);
                  return (
                    <motion.div
                      key={item.symbol + i}
                      initial={{ opacity: 0, scale: 0.8 }}
                      animate={{ opacity: 1, scale: 1 }}
                      exit={{ opacity: 0, scale: 0.8 }}
                      className={cn(
                        "px-3 py-1.5 rounded-md text-sm font-mono flex items-center gap-2 transition-colors",
                        isActive 
                          ? "bg-blue-500/20 border border-blue-500/50 text-blue-300 shadow-[0_0_10px_rgba(59,130,246,0.2)]" 
                          : "bg-slate-800/30 border border-slate-700/50 text-slate-500"
                      )}
                    >
                      <span className={isActive ? "text-blue-400" : "text-slate-600"}>📦</span>
                      <span className={cn("font-bold text-xs", classColors[item.label]?.text || "text-slate-400")}>[{item.label}]</span>
                      {item.symbol}
                      <span className="text-slate-500 text-[10px]">({item.score.toFixed(1)})</span>
                    </motion.div>
                  );
                })}
              </AnimatePresence>
              {symbols.length === 0 && (
                <div className="text-slate-500 text-sm italic py-2">No deep concepts recognized yet.</div>
              )}
            </div>
          </div>
        </div>

      </main>
    </div>
  );
}
