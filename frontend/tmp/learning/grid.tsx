import React, { useEffect, useRef, useState } from 'react';
import * as d3 from 'd3';
import { ImpulseNode } from '../types';
import { cn } from '../lib/utils';
import { Play, Pause, RefreshCw, Grid2X2, Waves } from 'lucide-react';

interface ImpulseMapVizProps {
  data: ImpulseNode[];
}

export const ImpulseMapViz: React.FC<ImpulseMapVizProps> = ({ data }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
  const [isPlaying, setIsPlaying] = useState(true);
  const [layoutMode, setLayoutMode] = useState<'grid' | 'regions'>('grid');
  
  // Refs for D3 simulation to persist across renders without triggering React state updates
  const simulationRef = useRef<d3.Simulation<ImpulseNode, undefined> | null>(null);
  const nodesRef = useRef<ImpulseNode[]>(JSON.parse(JSON.stringify(data)));
  const tapeIntervalRef = useRef<number | null>(null);

  // Resize Observer
  useEffect(() => {
    const observeTarget = containerRef.current;
    if (!observeTarget) return;
    const resizeObserver = new ResizeObserver((entries) => {
      if (entries[0]) {
        const { width, height } = entries[0].contentRect;
        setDimensions({ width, height });
      }
    });
    resizeObserver.observe(observeTarget);
    return () => resizeObserver.unobserve(observeTarget);
  }, []);

  // Initialize D3 Visualization
  useEffect(() => {
    if (!svgRef.current || dimensions.width === 0) return;
    
    const svg = d3.select(svgRef.current);
    svg.selectAll("*").remove(); // Clear previous
    
    const width = dimensions.width;
    const height = dimensions.height;
    
    // Zoom behavior
    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.5, 4])
      .on("zoom", (e) => {
        g.attr("transform", e.transform);
      });
    svg.call(zoom);
    
    const g = svg.append("g");
    
    // We create layer groups to ensure correct z-index
    const contourLayer = g.append("g").attr("class", "contours");
    const linkLayer = g.append("g").attr("class", "links");
    const nodeLayer = g.append("g").attr("class", "nodes");
    
    // Compute Grid Positions
    const nodes = nodesRef.current;
    const cols = Math.ceil(Math.sqrt(nodes.length * (width / height)));
    const rows = Math.ceil(nodes.length / cols);
    const stepX = (width * 0.8) / Math.max(1, cols - 1);
    const stepY = (height * 0.8) / Math.max(1, rows - 1);
    const startX = width * 0.1;
    const startY = height * 0.1;
    
    nodes.forEach((n, i) => {
      n.gridX = startX + (i % cols) * stepX;
      n.gridY = startY + Math.floor(i / cols) * stepY;
      
      // Seed initial layout precisely on grid to prevent fly-in on first load
      if (n.x === undefined || isNaN(n.x)) {
        n.x = n.gridX;
        n.y = n.gridY;
      }
    });

    // Define cluster focal points (where sympathy draws them)
    const foci = [
      { x: width * 0.3, y: height * 0.3 }, // Cluster 0
      { x: width * 0.7, y: height * 0.3 }, // Cluster 1
      { x: width * 0.3, y: height * 0.7 }, // Cluster 2
      { x: width * 0.7, y: height * 0.7 }, // Cluster 3
    ];

    // Color Scales
    // Activation heat map: from dark panel -> dim blue -> green -> gold
    const colorScale = d3.scaleSequential(
      (t) => d3.interpolateRgbBasis(['#111113', '#0ea5e9', '#22c55e', '#fbbf24'])(t)
    ).domain([0, 1]);
    
    // Setup Initial Simulation
    const simulation = d3.forceSimulation(nodes)
      .force("x", d3.forceX<ImpulseNode>(d => d.gridX!).strength(0.3))
      .force("y", d3.forceY<ImpulseNode>(d => d.gridY!).strength(0.3))
      .force("collide", d3.forceCollide<ImpulseNode>().radius(d => d.snr * 1.5 + 2).iterations(1).strength(0.1))
      .force("charge", d3.forceManyBody().strength(-2))
      .force("center", d3.forceCenter(width / 2, height / 2).strength(0.01))
      .alphaDecay(0.02);
      
    simulationRef.current = simulation;

    // Create node elements
    const nodeElements = nodeLayer.selectAll("circle")
      .data(nodes)
      .enter()
      .append("circle")
      .attr("r", d => d.snr * 1.5 + 2)
      .attr("fill", d => colorScale(d.activation))
      .attr("stroke", "#050505")
      .attr("stroke-width", 1.5)
      // Tooltip interactions
      .on("mouseover", function(e, d) {
        d3.select(this).attr("stroke", "#fbbf24").attr("stroke-width", 2);
      })
      .on("mouseout", function(e, d) {
        d3.select(this).attr("stroke", "#050505").attr("stroke-width", 1.5);
      });

    // Contour Generator for Regions / Water Table
    const computeDensity = d3.contourDensity<ImpulseNode>()
      .x(d => d.x || 0)
      .y(d => d.y || 0)
      .weight(d => d.activation * d.snr) // Stronger activation & SNR creates higher peaks
      .size([width, height])
      .bandwidth(30)
      .thresholds(15);

    // Render loop
    simulation.on("tick", () => {
      // 1. Decay activations naturally over time
      if (isPlaying) {
        nodes.forEach(n => {
          if (n.activation > 0) {
            n.activation = Math.max(0, n.activation - 0.005);
          }
        });
      }

      // 2. Update Node Positions & Colors
      nodeElements
        .attr("cx", d => d.x || 0)
        .attr("cy", d => d.y || 0)
        .attr("fill", d => colorScale(d.activation));

      // 3. Compute and Draw Topographic Regions (Contours)
      const contourData = computeDensity(nodes);
      
      const paths = contourLayer.selectAll("path")
        .data(contourData);
        
      paths.enter().append("path")
        .merge(paths as any)
        .attr("d", d3.geoPath())
        .attr("fill", (d, i) => {
           // Base fill opacity on elevation depth
           return d3.color('#fbbf24')?.copy({ opacity: i * 0.015 }).toString() || 'none';
        })
        .attr("stroke", (d, i) => {
          // "Water table split" - The Otsu's threshold equivalent
          // We highlight a specific mid-to-high band as the "Agent Reaction Boundary"
          if (i === 6) return 'rgba(34, 197, 94, 0.4)'; // Green boundary
          if (i === 10) return 'rgba(251, 191, 36, 0.8)'; // Gold hot boundary
          return 'none';
        })
        .attr("stroke-width", (d, i) => i === 10 ? 1.5 : (i === 6 ? 1 : 0));
        
      paths.exit().remove();
      
      // 4. Draw Sympathy Links (Second Priority)
      // Connect highly active nodes in the same cluster that are physically close
      const activeLinks: {source: ImpulseNode, target: ImpulseNode}[] = [];
      const threshold = 0.6; // Only link very hot nodes
      
      for (let i = 0; i < nodes.length; i++) {
        if (nodes[i].activation < threshold) continue;
        for (let j = i + 1; j < nodes.length; j++) {
          if (nodes[j].activation < threshold) continue;
          if (nodes[i].cluster !== nodes[j].cluster) continue; // Must be sympathetic
          
          const dx = (nodes[i].x || 0) - (nodes[j].x || 0);
          const dy = (nodes[i].y || 0) - (nodes[j].y || 0);
          const dist = Math.sqrt(dx*dx + dy*dy);
          
          if (dist < 80) {
             activeLinks.push({source: nodes[i], target: nodes[j]});
          }
        }
      }
      
      const links = linkLayer.selectAll("line").data(activeLinks);
      
      links.enter().append("line")
        .merge(links as any)
        .attr("x1", d => d.source.x || 0)
        .attr("y1", d => d.source.y || 0)
        .attr("x2", d => d.target.x || 0)
        .attr("y2", d => d.target.y || 0)
        .attr("stroke", "#fbbf24")
        .attr("stroke-opacity", d => Math.min((d.source.activation + d.target.activation) / 2, 0.8))
        .attr("stroke-width", 1.5);
        
      links.exit().remove();
    });

    return () => {
      simulation.stop();
    };
  }, [dimensions]); // only recreate SVG nodes/paths when container actually resizes

  // Handle Mode Transitions (Grid vs Regions)
  useEffect(() => {
    const simulation = simulationRef.current;
    if (!simulation || dimensions.width === 0) return;
    
    const width = dimensions.width;
    const height = dimensions.height;
    
    const foci = [
      { x: width * 0.3, y: height * 0.3 },
      { x: width * 0.7, y: height * 0.3 },
      { x: width * 0.3, y: height * 0.7 },
      { x: width * 0.7, y: height * 0.7 },
    ];

    if (layoutMode === 'grid') {
      simulation
        .force("x", d3.forceX<ImpulseNode>(d => d.gridX!).strength(0.15))
        .force("y", d3.forceY<ImpulseNode>(d => d.gridY!).strength(0.15))
        .force("collide", null)
        .force("charge", null)
        .alphaDecay(0.015);
        
      // Toggle contours off gracefully
      d3.select(svgRef.current).select(".contours").transition().duration(1000).style("opacity", 0);
    } else {
      simulation
        .force("x", d3.forceX<ImpulseNode>(d => foci[d.cluster].x).strength(0.08))
        .force("y", d3.forceY<ImpulseNode>(d => foci[d.cluster].y).strength(0.08))
        .force("collide", d3.forceCollide<ImpulseNode>().radius(d => d.snr * 1.5 + 2).iterations(2))
        .force("charge", d3.forceManyBody().strength(-15))
        .alphaDecay(0.015);
        
      // Toggle contours on
      d3.select(svgRef.current).select(".contours").transition().duration(1000).style("opacity", 1);
    }
    
    // Give the simulation enough heat to travel across the entire screen smoothly
    simulation.alpha(1).restart();

  }, [layoutMode, dimensions]);

  // Market Tape Feed Simulation (Injecting impulses)
  useEffect(() => {
    if (!isPlaying) {
      if (tapeIntervalRef.current) clearInterval(tapeIntervalRef.current);
      return;
    }
    
    // Simulate tape feeding data and activating clusters
    tapeIntervalRef.current = window.setInterval(() => {
      const nodes = nodesRef.current;
      const simulation = simulationRef.current;
      if (!simulation || nodes.length === 0) return;
      
      // Randomly pick a cluster to receive an "impulse"
      const targetCluster = Math.floor(Math.random() * 4);
      const impulseStrength = Math.random() * 0.8 + 0.2; // 0.2 to 1.0
      
      let activatedCount = 0;
      
      nodes.forEach(n => {
        // If node is in the targeted cluster, it receives the impulse
        if (n.cluster === targetCluster) {
           // Second priority logic: some noise, some correlation
           if (Math.random() < 0.6) {
             n.activation = Math.min(1, n.activation + impulseStrength * (Math.random() * 0.5 + 0.5));
             activatedCount++;
           }
        }
      });
      
      // Briefly increase simulation alpha to allow reorganization based on new weights
      if (activatedCount > 0) {
        simulation.alpha(0.1).restart();
      }
      
    }, 1200); // New tape event every 1.2s

    return () => {
      if (tapeIntervalRef.current) clearInterval(tapeIntervalRef.current);
    };
  }, [isPlaying]);


  return (
    <div className="flex flex-col w-full h-full">
      <div className="h-8 border-b border-[#27272a] flex items-center justify-between px-4 text-xs bg-[#09090b]">
        <div className="flex gap-4">
          <span className="text-[#fbbf24] border-b border-[#fbbf24] py-1.5">Map</span>
          <span className="text-[#52525b] py-1.5">Topography</span>
          <span className="text-[#52525b] py-1.5">Sympathy Grid</span>
        </div>
        <div className="flex items-center gap-3">
          {/* Phase Toggle Controls */}
          <div className="flex items-center gap-1 bg-[#111113] border border-[#27272a] p-1 rounded mr-2">
            <button 
              onClick={() => setLayoutMode('grid')}
              className={`px-3 py-1 rounded transition-colors flex items-center gap-1.5 ${layoutMode === 'grid' ? 'bg-[#27272a] text-[#fbbf24]' : 'text-[#71717a] hover:text-[#a1a1aa]'}`}
            >
              <Grid2X2 className="w-3.5 h-3.5" />
              Initial Grid
            </button>
            <button 
              onClick={() => setLayoutMode('regions')}
              className={`px-3 py-1 rounded transition-colors flex items-center gap-1.5 ${layoutMode === 'regions' ? 'bg-[#27272a] text-[#fbbf24]' : 'text-[#71717a] hover:text-[#a1a1aa]'}`}
            >
              <Waves className="w-3.5 h-3.5" />
              Sympathy Clustering
            </button>
          </div>
          
          <div className="flex items-center gap-1 text-[#52525b]">
            <div className="w-2 h-2 rounded bg-[#fbbf24] opacity-80"></div>
            <span>Agent Action Threshold (Otsu Split)</span>
          </div>
          <div className="h-3 w-px bg-[#27272a]"></div>
          <button 
            onClick={() => {
              nodesRef.current.forEach(n => n.activation = 0);
              simulationRef.current?.alpha(0.3).restart();
            }}
            className="text-[#71717a] hover:text-white transition-colors"
            title="Reset Map"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
          <button 
            onClick={() => setIsPlaying(!isPlaying)}
            className="text-[#fbbf24] hover:text-white transition-colors ml-2"
          >
            {isPlaying ? <Pause className="w-3.5 h-3.5" /> : <Play className="w-3.5 h-3.5" />}
          </button>
        </div>
      </div>
      
      <div ref={containerRef} className="flex-1 relative bg-[#050505] overflow-hidden">
        {/* Background Grid */}
        <div 
          className="absolute inset-0 pointer-events-none opacity-[0.15]"
          style={{
            backgroundImage: 'linear-gradient(#27272a 1px, transparent 1px), linear-gradient(90deg, #27272a 1px, transparent 1px)',
            backgroundSize: '40px 40px',
          }}
        />
        
        <svg ref={svgRef} className="w-full h-full absolute inset-0" />
        
        {/* Market Tape Overlay */}
        <div className="absolute top-4 left-4 w-64 bg-[#09090b]/80 backdrop-blur border border-[#27272a] rounded p-3 pointer-events-none shadow-xl">
          <div className="text-[10px] uppercase tracking-widest text-[#52525b] mb-2 flex items-center justify-between">
            <span>Market Tape Feed</span>
            {isPlaying && <span className="w-1.5 h-1.5 rounded-full bg-[#22c55e] animate-pulse"></span>}
          </div>
          <div className="space-y-1.5 font-mono text-xs">
            <div className="flex justify-between items-center text-[#a1a1aa]">
              <span className="truncate">ETH/USD Vol Break</span>
              <span className="text-[#0ea5e9]">38ms</span>
            </div>
            <div className="flex justify-between items-center text-[#d4d4d4]">
              <span className="truncate">Orderbook Imbalance</span>
              <span className="text-[#fbbf24]">12ms</span>
            </div>
            <div className="flex justify-between items-center text-[#52525b]">
              <span className="truncate">BTC Momentum</span>
              <span>105ms</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
