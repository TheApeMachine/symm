import React, { useEffect, useRef, useState } from 'react';
import * as d3 from 'd3';
import { motion, AnimatePresence } from 'motion/react';
import { Play, Pause, ChevronRight } from 'lucide-react';

type EpisodeType = 'UPWARD EXCURSION' | 'DOWNWARD EXCURSION' | 'STAGNATION';

interface TrainingEpisode {
  id: number;
  type: EpisodeType;
  length: number;
  points: { x: number; y: number }[];
  marks: { A: number; B: number; C: number };
  agent: {
    entryIdx: number | null;
    exitIdx: number | null;
    reward: number;
  };
  magnitude: number;
}

interface TriePath {
  hash: string;
  depth: number;
  visits: number;
  meanEdge: number;
  confidence: number;
  policy: 'ENTER' | 'WAIT' | 'EXIT';
}

interface ActivityEntry {
  id: number;
  time: string;
  message: string;
  pnl: number;
}

const generateEpisode = (id: number): TrainingEpisode => {
  const length = 250;
  const baseA = 30;
  const A = baseA + Math.floor(Math.random() * 20);
  const B = A + 30 + Math.floor(Math.random() * 20);
  const C = B + 60 + Math.floor(Math.random() * 40);

  const r = Math.random();
  let type: EpisodeType = 'STAGNATION';
  let direction = 0;
  if (r > 0.6) { type = 'UPWARD EXCURSION'; direction = 1; }
  else if (r > 0.3) { type = 'DOWNWARD EXCURSION'; direction = -1; }

  const points = [];
  let y = 50;
  let magnitude = 0;
  
  for (let i = 0; i < length; i++) {
    y += (Math.random() - 0.5) * 1.5;
    if (i > A && i < C && type !== 'STAGNATION') {
      const strength = i < B ? 0.4 : 0.2;
      y -= direction * (Math.random() * strength * 2);
      magnitude += direction * strength;
    }
    y += (50 - y) * 0.01;
    y = Math.max(10, Math.min(90, y));
    points.push({ x: i, y });
  }

  let entryIdx: number | null = null;
  let exitIdx: number | null = null;
  let reward = 0;

  if (type !== 'STAGNATION') {
    entryIdx = B + Math.floor((Math.random() - 0.3) * 15);
    exitIdx = C + Math.floor((Math.random() - 0.5) * 20);
    const entryError = Math.abs(entryIdx - B);
    const exitError = Math.abs(exitIdx - C);
    reward = 8 - (entryError * 0.15) - (exitError * 0.1);
    if (Math.random() > 0.9) {
      entryIdx = null;
      exitIdx = null;
      reward = -5;
    }
  } else {
    if (Math.random() > 0.8) {
      entryIdx = A + Math.floor(Math.random() * 40);
      exitIdx = entryIdx + 40;
      reward = -3.5;
    } else {
      reward = 2.5;
    }
  }

  return {
    id,
    type,
    length,
    points,
    marks: { A, B, C },
    agent: { entryIdx, exitIdx, reward },
    magnitude: Math.abs(magnitude / 100)
  };
};

export const ForwardLearningViz: React.FC = () => {
  const tapeRef = useRef<HTMLDivElement>(null);
  const distRef = useRef<HTMLDivElement>(null);
  const [tapeDim, setTapeDim] = useState({ width: 800, height: 300 });
  const [distDim, setDistDim] = useState({ width: 300, height: 150 });
  const [isPlaying, setIsPlaying] = useState(true);
  
  // State
  const [history, setHistory] = useState<TrainingEpisode[]>([]);
  const [currentEpisode, setCurrentEpisode] = useState<TrainingEpisode>(() => generateEpisode(1));
  const [tick, setTick] = useState(0);
  const [phase, setPhase] = useState<'PLAYING' | 'EVALUATING'>('PLAYING');
  
  const [triePaths, setTriePaths] = useState<TriePath[]>(() => 
    Array.from({ length: 8 }).map(() => ({
      hash: `0x${Math.floor(Math.random()*16777215).toString(16).padStart(6, '0')}`,
      depth: Math.floor(Math.random() * 5) + 3,
      visits: Math.floor(Math.random() * 5000) + 100,
      meanEdge: (Math.random() * 2) - 0.5,
      confidence: Math.random() * 100,
      policy: Math.random() > 0.5 ? 'ENTER' : 'WAIT'
    }))
  );
  
  const [globalPnl, setGlobalPnl] = useState(-3.174603);

  const [logs, setLogs] = useState<ActivityEntry[]>([]);
  const logCounter = useRef(0);

  // Resize observers
  useEffect(() => {
    if (tapeRef.current) {
      const ro = new ResizeObserver(entries => {
        if (entries[0]) setTapeDim({ width: entries[0].contentRect.width, height: entries[0].contentRect.height });
      });
      ro.observe(tapeRef.current);
      return () => ro.disconnect();
    }
  }, []);

  useEffect(() => {
    if (distRef.current) {
      const ro = new ResizeObserver(entries => {
        if (entries[0]) setDistDim({ width: entries[0].contentRect.width, height: entries[0].contentRect.height });
      });
      ro.observe(distRef.current);
      return () => ro.disconnect();
    }
  }, []);

  // Main Loop
  useEffect(() => {
    if (!isPlaying) return;
    
    let timer: number;
    if (phase === 'PLAYING') {
      timer = window.setInterval(() => {
        setTick(t => {
          if (t >= currentEpisode.length - 1) {
            setPhase('EVALUATING');
            return t;
          }
          return t + 2;
        });
      }, 16);
    } else if (phase === 'EVALUATING') {
      timer = window.setTimeout(() => {
        const reward = currentEpisode.agent.reward;
        
        setHistory(prev => {
          const next = [...prev, currentEpisode];
          return next.slice(-100); // Keep last 100 for dist
        });
        
        // Update Radix Trie Memory & P&L
        setGlobalPnl(prev => prev + (reward * 0.05));
        setTriePaths(prev => {
           const updated = [...prev];
           updated[0].visits += 1;
           updated[0].meanEdge = (updated[0].meanEdge * 0.9) + (reward * 0.1);
           updated[0].confidence = Math.min(99.9, Math.max(0, updated[0].confidence + (reward > 0 ? 0.5 : -0.5)));
           return updated;
        });

        // Add Log
        const now = new Date();
        const timeStr = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}:${now.getSeconds().toString().padStart(2, '0')}`;
        setLogs(prev => {
          const actionStr = currentEpisode.agent.entryIdx ? 'valued enter' : 'valued wait';
          const newLog = {
            id: logCounter.current++,
            time: timeStr,
            message: `policy ${Math.floor(Math.random()*4)+1} · ${actionStr}`,
            pnl: reward
          };
          return [newLog, ...prev].slice(0, 10);
        });

        setCurrentEpisode(generateEpisode(currentEpisode.id + 1));
        setTick(0);
        setPhase('PLAYING');
      }, 2500);
    }
    
    return () => {
      clearInterval(timer);
      clearTimeout(timer);
    };
  }, [isPlaying, phase, currentEpisode]);

  // Tape Generators
  const xScale = d3.scaleLinear().domain([0, 250]).range([0, tapeDim.width]);
  const yScale = d3.scaleLinear().domain([0, 100]).range([tapeDim.height - 40, 40]);
  
  const currentPoints = currentEpisode.points.slice(0, tick);
  const lineGenerator = d3.line<{x: number, y: number}>()
    .x(d => xScale(d.x))
    .y(d => yScale(d.y))
    .curve(d3.curveMonotoneX);

  // Stats
  const wins = history.filter(h => h.agent.reward > 0).length;
  const losses = history.filter(h => h.agent.reward < 0).length;
  const total = wins + losses || 1; // avoid /0
  const meanEdge = history.length > 0 ? history.reduce((acc, ep) => acc + ep.agent.reward, 0) / history.length : 0.8;
  const variance = history.length > 1 ? history.reduce((acc, ep) => acc + Math.pow(ep.agent.reward - meanEdge, 2), 0) / (history.length - 1) : 12;
  const sd = Math.max(0.1, Math.sqrt(variance));

  // Dist Bell Curve
  const distPdf = (x: number, m: number, s: number) => (1 / (s * Math.sqrt(2 * Math.PI))) * Math.exp(-0.5 * Math.pow((x - m) / s, 2));
  const curveData: {x: number, y: number}[] = [];
  const minX = -15;
  const maxX = 15;
  for(let x = minX; x <= maxX; x += 0.5) {
     curveData.push({ x, y: distPdf(x, meanEdge, sd) });
  }
  
  const distMaxY = Math.max(...curveData.map(d => d.y), 0.1);
  const distXScale = d3.scaleLinear().domain([minX, maxX]).range([20, distDim.width - 20]);
  const distYScale = d3.scaleLinear().domain([0, distMaxY * 1.2]).range([distDim.height - 25, 20]);
  
  const distAreaGen = d3.area<{x: number, y: number}>()
    .x(d => distXScale(d.x))
    .y0(distDim.height - 25)
    .y1(d => distYScale(d.y))
    .curve(d3.curveBasis);

  const distLineGen = d3.line<{x: number, y: number}>()
    .x(d => distXScale(d.x))
    .y(d => distYScale(d.y))
    .curve(d3.curveBasis);

  return (
    <div className="flex flex-col w-full h-full gap-2 font-mono text-[11px] bg-[#050505] p-2 overflow-hidden text-[#a1a1aa]">
      
      {/* TOP ROW: Tape + Right Sidebar */}
      <div className="flex h-3/5 gap-2 min-h-0">
        
        {/* Main Episodic Tape */}
        <div className="flex-1 bg-[#09090b] border border-[#27272a] rounded flex flex-col relative min-w-0">
          <div className="h-8 border-b border-[#27272a] bg-[#050505] flex items-center px-4 justify-between text-[#71717a] shrink-0">
            <div className="flex items-center gap-2">
               <span className="text-[#fbbf24] font-bold">RUN</span>
               <span className="bg-[#111113] border border-[#27272a] px-1.5 py-0.5 rounded text-[10px]">EP-{currentEpisode.id.toString().padStart(4, '0')}</span>
               <ChevronRight className="w-3 h-3 text-[#52525b]" />
               <span className="text-white">LEARNING PHASE</span>
            </div>
            <div className="flex items-center gap-3">
               <span className="text-[10px] uppercase tracking-widest text-[#52525b]">Coordinate <span className="border border-[#52525b] text-[#a1a1aa] px-1 rounded ml-1">midpoint</span></span>
               <button onClick={() => setIsPlaying(!isPlaying)} className="text-[#fbbf24] hover:text-white transition-colors">
                 {isPlaying ? <Pause className="w-3.5 h-3.5" /> : <Play className="w-3.5 h-3.5" />}
               </button>
            </div>
          </div>

          <AnimatePresence>
            {phase === 'EVALUATING' && currentEpisode.type !== 'STAGNATION' && (
              <motion.div 
                initial={{ height: 0, opacity: 0 }} animate={{ height: 20, opacity: 1 }} exit={{ height: 0, opacity: 0 }}
                className={`border-b ${currentEpisode.type === 'UPWARD EXCURSION' ? 'bg-[#22c55e]/10 border-[#22c55e]/20 text-[#22c55e]' : 'bg-[#ef4444]/10 border-[#ef4444]/20 text-[#ef4444]'} flex items-center px-4 text-[10px] font-bold z-10 shrink-0`}
              >
                <span>{currentEpisode.type} <span className="text-[#71717a] ml-1 font-normal">confirmed</span></span>
                <span className="ml-auto">{currentEpisode.type === 'UPWARD EXCURSION' ? '+' : '-'}{(currentEpisode.magnitude).toFixed(2)}%</span>
              </motion.div>
            )}
          </AnimatePresence>

          <div ref={tapeRef} className="flex-1 relative overflow-hidden">
             {tapeDim.width > 0 && (
               <svg width={tapeDim.width} height={tapeDim.height} className="absolute inset-0">
                 <g className="text-[#27272a] stroke-current" strokeWidth="1" strokeDasharray="2 4">
                   <line x1="0" y1={tapeDim.height/2} x2={tapeDim.width} y2={tapeDim.height/2} />
                 </g>

                 {/* Tape Line */}
                 {currentPoints.length > 0 && (
                   <path d={lineGenerator(currentPoints) || undefined} fill="none" stroke="#fbbf24" strokeWidth="1.5" />
                 )}
                 {currentPoints.length > 0 && (
                   <circle cx={xScale(currentPoints[currentPoints.length - 1].x)} cy={yScale(currentPoints[currentPoints.length - 1].y)} r={2} fill="#fbbf24" />
                 )}

                 {/* Hindsight Markers */}
                 <AnimatePresence>
                   {phase === 'EVALUATING' && (
                     <motion.g initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
                       {['A', 'B', 'C'].map((m, i) => {
                         const idx = currentEpisode.marks[m as keyof typeof currentEpisode.marks];
                         return (
                           <g key={m} transform={`translate(${xScale(idx)}, 0)`}>
                             <line x1={0} y1={15} x2={0} y2={tapeDim.height} stroke="#0ea5e9" strokeWidth="1" strokeDasharray="2 4" opacity="0.3" />
                             <rect x={-7} y={8} width={14} height={14} fill="#050505" stroke="#0ea5e9" strokeWidth="1" />
                             <text x={0} y={18} fill="#0ea5e9" fontSize="9px" textAnchor="middle">{m}</text>
                           </g>
                         );
                       })}
                     </motion.g>
                   )}
                 </AnimatePresence>

                 {/* Agent Entries */}
                 {currentEpisode.agent.entryIdx && tick >= currentEpisode.agent.entryIdx && (
                   <g transform={`translate(${xScale(currentEpisode.agent.entryIdx)}, ${yScale(currentEpisode.points[currentEpisode.agent.entryIdx].y)})`}>
                     <circle r={4} fill="#22c55e" />
                     <text x={6} y={-6} fill="#22c55e" fontSize="9px">ENTER</text>
                     <line y2={tapeDim.height} stroke="#22c55e" opacity="0.3" />
                   </g>
                 )}
                 {currentEpisode.agent.exitIdx && tick >= currentEpisode.agent.exitIdx && (
                   <g transform={`translate(${xScale(currentEpisode.agent.exitIdx)}, ${yScale(currentEpisode.points[currentEpisode.agent.exitIdx].y)})`}>
                     <circle r={4} fill="#ef4444" />
                     <text x={6} y={-6} fill="#ef4444" fontSize="9px">EXIT</text>
                     <line y2={tapeDim.height} stroke="#ef4444" opacity="0.3" />
                   </g>
                 )}
               </svg>
             )}
          </div>
        </div>

        {/* Right Sidebar: Agent Skill & Activity */}
        <div className="w-[300px] bg-[#09090b] border border-[#27272a] rounded flex flex-col shrink-0 min-h-0">
          <div className="h-8 border-b border-[#27272a] bg-[#050505] flex items-center px-3 text-[#71717a] shrink-0 justify-between">
            <span className="tracking-widest uppercase">Agent Skill</span>
            <span>{history.length} evaluated</span>
          </div>
          
          <div className="p-4 flex-1 overflow-y-auto custom-scrollbar flex flex-col gap-5">
            <div>
              <div className="uppercase tracking-widest text-[#52525b] text-[10px] mb-2">Mean Completed Decision Benefit</div>
              <div className="text-white text-base">{meanEdge.toFixed(2)} bp</div>
              <div className="text-[#52525b] text-[10px] mt-1 leading-tight">
                Completed tape evaluations as a fraction of starting capital. Negative outcomes remain negative.
              </div>
            </div>

            <div>
              <div className="uppercase tracking-widest text-[#52525b] text-[10px] mb-2">Outcome Signs</div>
              <div className="flex items-baseline gap-2 mb-2">
                <span className="text-[#22c55e]">{wins} positive</span>
                <span className="text-[#52525b]">·</span>
                <span className="text-[#ef4444]">{losses} negative</span>
              </div>
              <div className="h-1.5 w-full bg-[#111113] flex rounded-full overflow-hidden">
                <div className="bg-[#22c55e]" style={{ width: `${(wins / total) * 100}%` }} />
                <div className="bg-[#ef4444]" style={{ width: `${(losses / total) * 100}%` }} />
              </div>
            </div>

            <div>
              <div className="uppercase tracking-widest text-[#52525b] text-[10px] mb-2">System P&L</div>
              <div className="text-white text-base font-mono">{globalPnl.toFixed(8)}</div>
              <div className="text-[#52525b] text-[10px] mt-1 leading-tight">
                Net change in the singular radix trie model's theoretical wallet, including fees.
              </div>
            </div>
            
            <div className="mt-2 pt-4 border-t border-[#27272a]">
              <div className="uppercase tracking-widest text-[#52525b] text-[10px] mb-3">Recent Learning Activity</div>
              <div className="flex flex-col gap-3">
                <AnimatePresence initial={false}>
                  {logs.map(log => (
                    <motion.div 
                      key={log.id} 
                      initial={{ opacity: 0, height: 0 }} 
                      animate={{ opacity: 1, height: 'auto' }}
                      className="text-[10px]"
                    >
                      <div className="text-[#a1a1aa] mb-0.5">{log.time} · {log.message}</div>
                      <div className="text-[#71717a]">Wallet P&L <span className={log.pnl > 0 ? 'text-[#22c55e]' : 'text-[#ef4444]'}>{log.pnl > 0 ? '+' : ''}{log.pnl.toFixed(4)}</span></div>
                    </motion.div>
                  ))}
                </AnimatePresence>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* BOTTOM ROW: Traders Table + Distribution */}
      <div className="flex h-2/5 gap-2 min-h-0">
        
        {/* Radix Trie Memory Table */}
        <div className="flex-1 bg-[#09090b] border border-[#27272a] rounded flex flex-col min-w-0">
           <div className="h-8 border-b border-[#27272a] bg-[#050505] flex items-center px-4 justify-between shrink-0">
             <span className="text-[#71717a] uppercase tracking-widest text-[10px]">Radix Trie Memory (Active Branches)</span>
             <span className="text-[#52525b] text-[10px]">{triePaths.reduce((acc, p) => acc + p.visits, 0).toLocaleString()} observations routed</span>
           </div>
           <div className="flex-1 overflow-auto custom-scrollbar p-2">
             <table className="w-full text-left border-collapse">
               <thead>
                 <tr className="text-[#52525b] border-b border-[#27272a]">
                   <th className="font-normal pb-2 px-2">Path Signature</th>
                   <th className="font-normal pb-2 px-2 text-right">Depth</th>
                   <th className="font-normal pb-2 px-2 text-right">Visits</th>
                   <th className="font-normal pb-2 px-2 text-right">Mean Edge</th>
                   <th className="font-normal pb-2 px-2">Confidence</th>
                   <th className="font-normal pb-2 px-2 text-right">Learned Policy</th>
                 </tr>
               </thead>
               <tbody>
                 {triePaths.map((p, i) => (
                   <tr key={p.hash} className="border-b border-[#27272a]/50 last:border-0 hover:bg-[#111113]">
                     <td className={`py-1.5 px-2 font-mono ${i === 0 ? 'text-[#fbbf24]' : 'text-[#a1a1aa]'}`}>{p.hash}</td>
                     <td className="py-1.5 px-2 text-right">{p.depth}</td>
                     <td className="py-1.5 px-2 text-right">{p.visits.toLocaleString()}</td>
                     <td className={`py-1.5 px-2 text-right ${p.meanEdge > 0 ? 'text-[#22c55e]' : 'text-[#ef4444]'}`}>{p.meanEdge > 0 ? '+' : ''}{p.meanEdge.toFixed(2)} bp</td>
                     <td className="py-1.5 px-2">
                       <div className="flex items-center gap-2">
                         <div className="w-12 h-1 bg-[#111113] rounded-full overflow-hidden">
                           <div className="h-full bg-[#0ea5e9]" style={{ width: `${p.confidence}%` }} />
                         </div>
                         <span>{p.confidence.toFixed(1)}%</span>
                       </div>
                     </td>
                     <td className="py-1.5 px-2 text-right">
                        <span className={`px-1.5 py-0.5 rounded text-[9px] border ${p.policy === 'ENTER' ? 'bg-[#22c55e]/10 border-[#22c55e]/30 text-[#22c55e]' : 'bg-[#71717a]/10 border-[#71717a]/30 text-[#71717a]'}`}>
                          {p.policy}
                        </span>
                     </td>
                   </tr>
                 ))}
               </tbody>
             </table>
           </div>
        </div>

        {/* Edge Distribution */}
        <div className="w-[400px] bg-[#09090b] border border-[#27272a] rounded flex flex-col shrink-0 min-h-0">
          <div className="h-8 border-b border-[#27272a] bg-[#050505] flex items-center px-4 shrink-0 justify-between">
             <div className="flex gap-4 uppercase tracking-widest text-[10px]">
               <span className="text-[#52525b]">Learn</span>
               <span className="text-[#fbbf24]">Edge Distribution</span>
             </div>
          </div>
          
          <div ref={distRef} className="flex-1 relative">
            {distDim.width > 0 && (
              <svg width={distDim.width} height={distDim.height} className="absolute inset-0">
                <defs>
                  <linearGradient id="distFill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#0ea5e9" stopOpacity="0.3" />
                    <stop offset="100%" stopColor="#0ea5e9" stopOpacity="0.0" />
                  </linearGradient>
                </defs>

                {/* X Axis Base */}
                <line x1={20} y1={distDim.height - 25} x2={distDim.width - 20} y2={distDim.height - 25} stroke="#27272a" strokeWidth="1" />
                
                {/* Breakeven Line (0.0) */}
                <line x1={distXScale(0)} y1={20} x2={distXScale(0)} y2={distDim.height - 25} stroke="#52525b" strokeWidth="1" strokeDasharray="2 2" />
                <text x={distXScale(0)} y={15} fill="#52525b" fontSize="9px" textAnchor="middle">0.0 bp</text>

                {/* Mean Line */}
                <line x1={distXScale(meanEdge)} y1={20} x2={distXScale(meanEdge)} y2={distDim.height - 25} stroke="#22c55e" strokeWidth="1" />
                <text x={distXScale(meanEdge)} y={15} fill="#22c55e" fontSize="9px" textAnchor="middle">μ {meanEdge.toFixed(1)}</text>

                {/* Curve Area */}
                <path d={distAreaGen(curveData) || undefined} fill="url(#distFill)" />
                <path d={distLineGen(curveData) || undefined} fill="none" stroke="#0ea5e9" strokeWidth="1.5" />

                {/* Labels */}
                <text x={20} y={distDim.height - 10} fill="#52525b" fontSize="9px">-15 bp</text>
                <text x={distDim.width - 20} y={distDim.height - 10} fill="#52525b" fontSize="9px" textAnchor="end">+15 bp</text>
              </svg>
            )}
          </div>
          
          <div className="p-3 border-t border-[#27272a] text-[10px] text-[#71717a]">
            Normal fit to the authority-weighted mean and variance of completed outcomes.
          </div>
        </div>
        
      </div>
    </div>
  );
};
