import { useState, useRef, useEffect } from 'react';
import { Settings2, Play, Pause } from 'lucide-react';
import { cn } from '#/lib/utils';

const INITIAL_METRICS = [
  { id: "alpha", name: "Alpha Yield", xFactor: 0, yFactor: -0.75, color: "#10b981", baseWeight: 65 },
  { id: "beta", name: "Beta Retention", xFactor: 0.72, yFactor: -0.25, color: "#06b6d4", baseWeight: 45 },
  { id: "gamma", name: "Gamma Latency", xFactor: 0.45, yFactor: 0.6, color: "#f43f5e", baseWeight: -30 },
  { id: "delta", name: "Delta Inflow", xFactor: -0.45, yFactor: 0.6, color: "#f59e0b", baseWeight: 50 },
  { id: "epsilon", name: "Epsilon Overhead", xFactor: -0.72, yFactor: -0.25, color: "#6366f1", baseWeight: -40 }
];

export default function VectorLab() {
  const [dimensions, setDimensions] = useState({ width: 800, height: 500 });
  const containerRef = useRef<HTMLDivElement>(null);
  
  const [snr, setSnr] = useState(21);
  const [activeMetricId, setActiveMetricId] = useState("alpha");
  const [weights, setWeights] = useState<Record<string, number>>({
    alpha: 65,
    beta: 45,
    gamma: -30,
    delta: 50,
    epsilon: -40
  });
  
  const [phase, setPhase] = useState(0);
  const [isPlaying, setIsPlaying] = useState(true);
  
  const lastTimeRef = useRef(Date.now());
  const animationFrameRef = useRef<number | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;
    const observer = new ResizeObserver((entries) => {
      if (!entries || entries.length === 0) return;
      const { width, height } = entries[0].contentRect;
      // We will fill the parent container completely
      setDimensions({ width, height });
    });
    observer.observe(containerRef.current);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const tick = () => {
      if (!isPlaying) return;
      const now = Date.now();
      const delta = (now - lastTimeRef.current) * 0.002;
      lastTimeRef.current = now;
      setPhase((prev) => (prev + delta) % (Math.PI * 2));
      animationFrameRef.current = requestAnimationFrame(tick);
    };

    if (isPlaying) {
      lastTimeRef.current = Date.now();
      animationFrameRef.current = requestAnimationFrame(tick);
    }
    
    return () => {
      if (animationFrameRef.current) {
        cancelAnimationFrame(animationFrameRef.current);
      }
    };
  }, [isPlaying]);

  const cx = dimensions.width / 2;
  const cy = (dimensions.height * 0.6) / 2; // Shift up slightly to leave room for HUD
  const radius = Math.min(dimensions.width, dimensions.height * 0.6) * 0.38;

  const nodes = INITIAL_METRICS.map((m) => {
    const x = cx + m.xFactor * radius;
    const y = cy + m.yFactor * radius;
    const currentWeight = weights[m.id] ?? m.baseWeight;
    return { ...m, x, y, currentWeight };
  });

  const totalInfluencePower = Object.values(weights).reduce((acc, w) => acc + Math.abs(w), 0);
  const netSystemBalance = Object.values(weights).reduce((acc, w) => acc + w, 0);
  const dynamicEntropy = Math.max(0, (100 - snr) * (1 + Math.abs(netSystemBalance) * 0.05)).toFixed(1);

  // Pre-calculate vector field to render in SVG
  const gridColumns = 24;
  const gridRows = 16;
  const vectorArrows = [];

  for (let c = 1; c < gridColumns; c++) {
    for (let r = 1; r < gridRows; r++) {
      const vx = (dimensions.width / gridColumns) * c;
      const vy = (dimensions.height / gridRows) * r;

      let fx = 0;
      let fy = 0;

      nodes.forEach((node) => {
        const dx = vx - node.x;
        const dy = vy - node.y;
        const distSq = dx * dx + dy * dy + 400; // soft-core to prevent infinite singularity
        const magnitude = (node.currentWeight * 12000) / distSq;
        
        fx += (dx / Math.sqrt(distSq)) * magnitude;
        fy += (dy / Math.sqrt(distSq)) * magnitude;
      });

      // Noise from SNR
      const noiseIntensity = (100 - snr) * 0.25;
      if (noiseIntensity > 0) {
        const noiseX = Math.sin(vx * 0.05 + vy * 0.02 + phase) * noiseIntensity;
        const noiseY = Math.cos(vy * 0.05 + vx * 0.02 + phase) * noiseIntensity;
        fx += noiseX;
        fy += noiseY;
      }

      const totalMag = Math.sqrt(fx * fx + fy * fy) || 0.001;
      const arrowLength = Math.min(18, Math.max(4, totalMag * 0.12));
      
      const dxNormalized = (fx / totalMag) * arrowLength;
      const dyNormalized = (fy / totalMag) * arrowLength;

      vectorArrows.push({ x: vx, y: vy, dx: dxNormalized, dy: dyNormalized, mag: totalMag });
    }
  }

  const contourLevels = [25, 55, 95];
  const activeMetric = nodes.find(n => n.id === activeMetricId) || nodes[0];

  return (
    <div className="w-full h-full bg-slate-950 text-slate-100 flex flex-col overflow-hidden font-sans relative" ref={containerRef}>
      
      {/* Top Bar */}
      <div className="flex items-center justify-between p-4 border-b border-slate-800/50 bg-slate-950/80 backdrop-blur-sm z-10 absolute top-0 left-0 right-0">
        <div className="flex items-center gap-2 px-2">
          <Settings2 className="w-5 h-5 text-slate-400" />
          <span className="text-sm font-medium text-slate-300 tracking-wide uppercase">Field Synthesis Vector Lab</span>
        </div>
        <button
          onClick={() => setIsPlaying(!isPlaying)}
          className="w-10 h-10 rounded-full flex items-center justify-center cursor-pointer transition-colors bg-slate-800 hover:bg-slate-700 text-slate-200"
          aria-label={isPlaying ? "Pause simulation" : "Play simulation"}
        >
          {isPlaying ? <Pause className="w-5 h-5" /> : <Play className="w-5 h-5 ml-0.5" />}
        </button>
      </div>

      {/* Main Visualization Canvas */}
      <div className="w-full h-full relative overflow-hidden">
        
        {/* Node Labels Overlay */}
        {nodes.map((node) => (
          <div
            key={node.id}
            style={{
              left: `${(node.x / dimensions.width) * 100}%`,
              top: `calc(${(node.y / dimensions.height) * 100}% - 36px)`,
              transform: "translateX(-50%)"
            }}
            className="absolute bg-slate-900/80 text-slate-200 rounded-md px-2.5 py-1 text-xs font-semibold pointer-events-none whitespace-nowrap backdrop-blur-md flex items-center gap-2 border border-slate-700/50 shadow-lg"
          >
            <span className="w-2 h-2 rounded-full shadow-[0_0_8px_currentColor]" style={{ backgroundColor: node.color, color: node.color }} />
            {node.name}
            <span className="text-[10px] text-slate-400 tabular-nums">
              ({node.currentWeight > 0 ? "+" : ""}{node.currentWeight})
            </span>
          </div>
        ))}

        {/* SVG Mathematical Construct */}
        <svg
          role="img"
          aria-label="Matrix vector field dashboard"
          viewBox={`0 0 ${dimensions.width} ${dimensions.height}`}
          className="w-full h-full block"
        >
          {/* Background Grid */}
          <g opacity="0.08" stroke="currentColor" strokeWidth="1" className="text-slate-500">
            {Array.from({ length: gridRows }).map((_, i) => (
              <line key={`hg-${i}`} x1="0" y1={(dimensions.height / gridRows) * i} x2={dimensions.width} y2={(dimensions.height / gridRows) * i} />
            ))}
            {Array.from({ length: gridColumns }).map((_, i) => (
              <line key={`vg-${i}`} x1={(dimensions.width / gridColumns) * i} y1="0" x2={(dimensions.width / gridColumns) * i} y2={dimensions.height} />
            ))}
          </g>

          {/* Contour Rings */}
          {nodes.map((node) => (
            <g key={`contour-${node.id}`} opacity={activeMetricId === node.id ? 0.25 : 0.05}>
              {contourLevels.map((level, idx) => {
                const computedRadius = Math.max(10, Math.abs(node.currentWeight) * (idx + 1) * 1.1);
                return (
                  <circle
                    key={`c-ring-${idx}`}
                    cx={node.x}
                    cy={node.y}
                    r={computedRadius}
                    fill="none"
                    stroke={node.color}
                    strokeWidth="1.5"
                    strokeDasharray={node.currentWeight < 0 ? "4 4" : "none"}
                  />
                );
              })}
            </g>
          ))}

          {/* Vector Field Arrows */}
          <g opacity="0.65">
            {vectorArrows.map((arrow, index) => {
              const strokeOp = Math.min(0.8, Math.max(0.1, arrow.mag * 0.04));
              return (
                <g key={`arrow-${index}`} transform={`translate(${arrow.x}, ${arrow.y})`} opacity={strokeOp}>
                  <line
                    x1={-arrow.dx / 2}
                    y1={-arrow.dy / 2}
                    x2={arrow.dx / 2}
                    y2={arrow.dy / 2}
                    stroke="#94a3b8" // slate-400
                    strokeWidth="1.5"
                    strokeLinecap="round"
                  />
                  <circle cx={arrow.dx / 2} cy={arrow.dy / 2} r="1.5" fill="#cbd5e1" />
                </g>
              );
            })}
          </g>

          {/* Connective Chords */}
          <g opacity="0.5">
            {nodes.map((source, sIdx) =>
              nodes.map((target, tIdx) => {
                if (sIdx >= tIdx) return null;
                const baselineStrength = (Math.abs(source.currentWeight) + Math.abs(target.currentWeight)) / 2;
                if (baselineStrength < (100 - snr) * 0.4) return null;

                const midX = (source.x + target.x) / 2;
                const midY = (source.y + target.y) / 2;
                
                // Curve factor
                const weightFactor = 0.18;
                const ctrlX = midX + (cx - midX) * weightFactor;
                const ctrlY = midY + (cy - midY) * weightFactor;

                const isActive = activeMetricId === source.id || activeMetricId === target.id;

                return (
                  <path
                    key={`chord-${source.id}-${target.id}`}
                    d={`M ${source.x} ${source.y} Q ${ctrlX} ${ctrlY} ${target.x} ${target.y}`}
                    fill="none"
                    stroke={isActive ? "#94a3b8" : "#334155"}
                    strokeWidth={isActive ? "2" : "1"}
                    strokeDasharray={snr < 40 ? "5 5" : "none"}
                    className="transition-colors duration-300"
                  />
                );
              })
            )}
          </g>

          {/* Node Hubs */}
          {nodes.map((node) => {
            const isSelected = node.id === activeMetricId;
            return (
              <g
                key={`hub-${node.id}`}
                transform={`translate(${node.x}, ${node.y})`}
                className="cursor-pointer"
                onClick={() => setActiveMetricId(node.id)}
              >
                <circle
                  r={Math.max(16, 16 + Math.abs(node.currentWeight) * 0.25)}
                  fill={node.color}
                  opacity={isSelected ? 0.2 + Math.sin(phase * 2) * 0.1 : 0.1}
                  className="transition-opacity duration-200"
                />
                <circle
                  r={isSelected ? 11 : 8}
                  fill={node.color}
                  stroke="#020617" // slate-950
                  strokeWidth="2"
                  className="transition-all duration-300"
                />
                {isSelected && (
                  <circle
                    r="16"
                    fill="none"
                    stroke={node.color}
                    strokeWidth="1.5"
                    strokeDasharray="4 3"
                  />
                )}
              </g>
            );
          })}
        </svg>
      </div>

      {/* Bottom Control Panel */}
      <div className="absolute bottom-0 left-0 right-0 bg-slate-950/90 backdrop-blur-xl border-t border-slate-800">
        
        {/* Global Metrics Strip */}
        <div className="flex items-center justify-center gap-8 md:gap-16 border-b border-slate-800/50 px-6 py-4">
          <div className="flex flex-col items-center gap-1">
            <span className="text-xs text-slate-500 font-semibold tracking-wider uppercase">Total Influence</span>
            <span className="text-base font-bold text-slate-200 tabular-nums">{totalInfluencePower.toFixed(0)} rad</span>
          </div>
          <div className="flex flex-col items-center gap-1">
            <span className="text-xs text-slate-500 font-semibold tracking-wider uppercase">System Balance</span>
            <span className="text-base font-bold text-slate-200 tabular-nums">{netSystemBalance > 0 ? "+" : ""}{netSystemBalance.toFixed(0)}%</span>
          </div>
          <div className="flex flex-col items-center gap-1">
            <span className="text-xs text-slate-500 font-semibold tracking-wider uppercase">Field Entropy</span>
            <span className="text-base font-bold text-amber-500 tabular-nums">{dynamicEntropy} nats</span>
          </div>
        </div>

        {/* Interactive Controls */}
        <div className="p-6 flex flex-col gap-6 max-w-5xl mx-auto w-full">
          
          {/* Target Selector */}
          <div className="flex flex-col gap-3">
            <span className="text-xs font-semibold text-slate-400 uppercase tracking-wider">Select Target Vector Pivot</span>
            <div className="flex w-full bg-slate-900 rounded-xl p-1.5 border border-slate-800">
              {nodes.map((node) => {
                const isActive = activeMetricId === node.id;
                return (
                  <button
                    key={node.id}
                    onClick={() => setActiveMetricId(node.id)}
                    className={cn(
                      "flex-1 text-center py-2 rounded-lg text-sm font-semibold transition-all duration-200",
                      isActive 
                        ? "bg-slate-700 text-white shadow-md" 
                        : "text-slate-500 hover:text-slate-300 hover:bg-slate-800/50"
                    )}
                  >
                    {node.name.split(" ")[0]}
                  </button>
                );
              })}
            </div>
          </div>

          {/* Sliders Grid */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-x-12 gap-y-6">
            
            {/* Amplitude Slider */}
            <div className="flex flex-col gap-3">
              <div className="flex justify-between items-center">
                <label className="text-xs font-semibold text-slate-400 uppercase tracking-wider">{activeMetric.name} Amplitude</label>
                <span className="text-sm font-bold text-white tabular-nums w-12 text-right">{weights[activeMetricId]}</span>
              </div>
              <input
                type="range"
                min="-100"
                max="100"
                step="1"
                value={weights[activeMetricId]}
                onChange={(e) => setWeights(prev => ({ ...prev, [activeMetricId]: parseInt(e.target.value) }))}
                className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-emerald-500"
                style={{ accentColor: activeMetric.color }}
              />
            </div>

            {/* SNR Slider */}
            <div className="flex flex-col gap-3">
              <div className="flex justify-between items-center">
                <label className="text-xs font-semibold text-slate-400 uppercase tracking-wider">Signal-to-Noise Ratio (SNR)</label>
                <span className="text-sm font-bold text-white tabular-nums w-12 text-right">{snr}%</span>
              </div>
              <input
                type="range"
                min="5"
                max="100"
                step="1"
                value={snr}
                onChange={(e) => setSnr(parseInt(e.target.value))}
                className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-sky-500"
              />
            </div>

          </div>
        </div>
      </div>
    </div>
  );
}
