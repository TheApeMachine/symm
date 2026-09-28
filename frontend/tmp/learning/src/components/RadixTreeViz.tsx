import React, { useEffect, useRef, useState, useMemo } from 'react';
import * as d3 from 'd3';
import { motion, AnimatePresence } from 'motion/react';
import { TrieNodeData } from '../types';
import { cn } from '../lib/utils';

interface RadixTreeVizProps {
  data: TrieNodeData;
  minProbability: number;
  colorMode?: 'threshold' | 'gradient';
  projection?: 'horizontal' | 'vertical' | 'radial';
}

export const RadixTreeViz: React.FC<RadixTreeVizProps> = ({ data, minProbability, colorMode = 'threshold', projection = 'horizontal' }) => {
  const svgRef = useRef<SVGSVGElement>(null);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const zoomBehavior = useRef<d3.ZoomBehavior<SVGSVGElement, unknown> | null>(null);
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
  const [transform, setTransform] = useState<d3.ZoomTransform>(d3.zoomIdentity);
  
  const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
  const [tooltip, setTooltip] = useState<{data: TrieNodeData, x: number, y: number} | null>(null);
  
  // Create a deep copy of data for mutability by D3 (for expanding/collapsing)
  const [treeData, setTreeData] = useState<TrieNodeData>(JSON.parse(JSON.stringify(data)));

  useEffect(() => {
    const observeTarget = wrapperRef.current;
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

  // Update tree data if base data changes (e.g. new root)
  useEffect(() => {
    setTreeData(JSON.parse(JSON.stringify(data)));
  }, [data]);

  // Recursively filter tree based on probability
  const filterTree = (node: TrieNodeData, minProb: number): TrieNodeData | null => {
    if (node.probability < minProb) return null;
    const filteredNode = { ...node };
    
    if (filteredNode.children) {
      filteredNode.children = filteredNode.children
        .map(child => filterTree(child, minProb))
        .filter((child): child is TrieNodeData => child !== null);
    }
    
    if (filteredNode._children) {
      filteredNode._children = filteredNode._children
        .map(child => filterTree(child, minProb))
        .filter((child): child is TrieNodeData => child !== null);
    }
    
    return filteredNode;
  };

  const filteredTreeData = useMemo(() => filterTree(treeData, minProbability), [treeData, minProbability]);

  // D3 Zoom setup
  useEffect(() => {
    if (!svgRef.current) return;
    
    const svg = d3.select(svgRef.current);
    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.1, 4])
      .on('zoom', (e) => {
        setTransform(e.transform);
      });
      
    zoomBehavior.current = zoom;
    svg.call(zoom);
    
    // Initial center transform
    const initialTransform = d3.zoomIdentity.translate(50, dimensions.height / 2).scale(1);
    svg.call(zoom.transform, initialTransform);
    
  }, [dimensions.width, dimensions.height]);

  // Toggle children on click
  const handleNodeClick = (nodeData: TrieNodeData) => {
    const toggleNode = (n: TrieNodeData) => {
      if (n.id === nodeData.id) {
        if (n.children) {
          n._children = n.children;
          n.children = undefined;
        } else if (n._children) {
          n.children = n._children;
          n._children = undefined;
        }
        return true;
      }
      
      let found = false;
      if (n.children) {
        for (let i = 0; i < n.children.length; i++) {
          if (toggleNode(n.children[i])) found = true;
        }
      }
      if (n._children && !found) {
         for (let i = 0; i < n._children.length; i++) {
          if (toggleNode(n._children[i])) found = true;
        }
      }
      return found;
    };
    
    const newData = JSON.parse(JSON.stringify(treeData));
    toggleNode(newData);
    setTreeData(newData);
  };

  const focusBestPath = () => {
    const newData = JSON.parse(JSON.stringify(data));
    
    const expandGreedyPath = (node: TrieNodeData) => {
      const allChildren = [...(node.children || []), ...(node._children || [])];
      if (allChildren.length === 0) {
         node.children = undefined;
         node._children = undefined;
         return;
      }
      
      const bestChild = allChildren.reduce((max, child) => child.probability > max.probability ? child : max, allChildren[0]);
      
      node.children = allChildren;
      node._children = undefined;
      
      node.children.forEach(c => {
         if (c.id === bestChild.id) {
           expandGreedyPath(c);
         } else {
           const cAll = [...(c.children || []), ...(c._children || [])];
           if (cAll.length > 0) {
              c._children = cAll;
              c.children = undefined;
           }
         }
      });
    };
    
    expandGreedyPath(newData);
    setTreeData(newData);
  };

  // Compute Layout
  const { nodes, links } = useMemo(() => {
    if (!filteredTreeData) return { nodes: [], links: [] };
    
    const root = d3.hierarchy<TrieNodeData>(filteredTreeData);
    
    // Configure tree layout
    const treeLayout = d3.tree<TrieNodeData>();
    
    if (projection === 'radial') {
      let maxDepth = 0;
      root.each(d => { if (d.depth > maxDepth) maxDepth = d.depth; });
      const radius = Math.max(300, maxDepth * 180);
      treeLayout.size([2 * Math.PI, radius]);
    } else if (projection === 'vertical') {
      const nodeWidth = 120;
      const nodeHeight = 80;
      treeLayout.nodeSize([nodeWidth * 1.2, nodeHeight * 2]);
    } else {
      // horizontal
      const nodeWidth = 140;
      const nodeHeight = 60;
      treeLayout.nodeSize([nodeHeight * 1.5, nodeWidth * 2.2]);
    }
      
    treeLayout(root);
    
    return {
      nodes: root.descendants(),
      links: root.links()
    };
  }, [filteredTreeData, projection]);
  
  const activePathIds = useMemo(() => {
    if (!hoveredNodeId) return null;
    const ids = new Set<string>();
    let current = nodes.find(n => n.data.id === hoveredNodeId);
    while (current) {
      ids.add(current.data.id);
      current = current.parent;
    }
    return ids;
  }, [hoveredNodeId, nodes]);

  const getStateColor = (state?: string) => {
    if (state === 'POLICY CHOICE') return '#22c55e'; // Green
    if (state === 'EVALUATED') return '#fbbf24'; // Gold
    if (state === 'ESTIMATED') return '#0ea5e9'; // Blue
    return '#52525b';
  };

  const getEdgeColor = (prob: number) => {
    if (colorMode === 'gradient') {
      return d3.interpolateRgb('#3f3f46', '#fbbf24')(prob);
    }
    if (prob > 0.3) return '#fbbf24'; // Gold
    if (prob > 0.1) return '#d97706'; // Dim Gold
    return '#3f3f46'; // Gray
  };

  return (
    <div ref={wrapperRef} className="w-full h-full bg-[#050505] rounded-md border border-[#27272a] relative overflow-hidden flex-1">
      
      {/* Grid Background Pattern */}
      <div 
        className="absolute inset-0 pointer-events-none opacity-20"
        style={{
          backgroundImage: 'linear-gradient(#27272a 1px, transparent 1px), linear-gradient(90deg, #27272a 1px, transparent 1px)',
          backgroundSize: '20px 20px',
          transform: `translate(${transform.x % 20}px, ${transform.y % 20}px) scale(${transform.k})`,
          transformOrigin: '0 0'
        }}
      />
      
      <svg ref={svgRef} className="w-full h-full absolute inset-0 cursor-grab active:cursor-grabbing">
        <g transform={transform.toString()}>
          {/* Links */}
          <g className="links">
            <AnimatePresence>
              {links.map((link, i) => {
                const getPath = (source: d3.HierarchyPointNode<TrieNodeData>, target: d3.HierarchyPointNode<TrieNodeData>) => {
                  if (projection === 'radial') {
                    const angle = (d: any) => d.x;
                    const radius = (d: any) => d.y;
                    return d3.linkRadial<any, any>().angle(angle).radius(radius)({ source, target }) || undefined;
                  }
                  if (projection === 'vertical') {
                    return d3.linkVertical<any, any>().x(d => d.x).y(d => d.y)({ source, target }) || undefined;
                  }
                  return d3.linkHorizontal<any, any>().x(d => d.y).y(d => d.x)({ source, target }) || undefined;
                };

                const getNodePos = (node: d3.HierarchyPointNode<TrieNodeData>) => {
                  if (projection === 'radial') {
                    const angle = node.x - Math.PI / 2;
                    return { x: node.y * Math.cos(angle), y: node.y * Math.sin(angle) };
                  }
                  if (projection === 'vertical') {
                    return { x: node.x, y: node.y };
                  }
                  return { x: node.y, y: node.x };
                };
                  
                const d = getPath(link.source, link.target);
                const initialD = getPath(link.source, link.source);
                
                const sPos = getNodePos(link.source);
                const tPos = getNodePos(link.target);
                const midX = (sPos.x + tPos.x) / 2;
                const midY = (sPos.y + tPos.y) / 2;
                  
                const isLinkActive = activePathIds ? activePathIds.has(link.target.data.id) : true;

                return (
                  <motion.g 
                    key={`link-${link.source.data.id}-${link.target.data.id}`}
                    initial={{ opacity: 0 }}
                    animate={{ opacity: isLinkActive ? 1 : 0.15 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.4 }}
                  >
                    <motion.path
                      d={d}
                      initial={{ d: initialD }}
                      animate={{ d: d }}
                      exit={{ d: initialD }}
                      transition={{ duration: 0.4, ease: "easeInOut" }}
                      fill="none"
                      stroke={getEdgeColor(link.target.data.probability)}
                      strokeWidth={Math.max(2, link.target.data.probability * 8)}
                      strokeOpacity={0.6}
                    />
                    {link.target.data.tokens && (
                      <motion.text
                        initial={{ opacity: 0 }}
                        animate={{ opacity: 1 }}
                        exit={{ opacity: 0 }}
                        transition={{ duration: 0.4, delay: 0.1 }}
                        x={midX}
                        y={projection === 'vertical' ? midY - 4 : midY - 8}
                        fill="#71717a"
                        fontSize="10px"
                        textAnchor="middle"
                        className="pointer-events-none select-none font-mono tracking-widest"
                      >
                        [{link.target.data.tokens.join(', ')}]
                      </motion.text>
                    )}
                  </motion.g>
                );
              })}
            </AnimatePresence>
          </g>
          
          {/* Nodes */}
          <g className="nodes">
            <AnimatePresence>
              {nodes.map((node) => {
                const data = node.data;
                const hasChildren = !!(data.children || data._children);
                const isCollapsed = !!data._children;
                const probColor = getEdgeColor(data.probability);
                const isHighProb = data.probability > 0.2;
                
                const parent = node.parent;
                const getNodePos = (n: typeof node) => {
                  if (projection === 'radial') {
                    const angle = n.x - Math.PI / 2;
                    return { x: n.y * Math.cos(angle), y: n.y * Math.sin(angle) };
                  }
                  if (projection === 'vertical') {
                    return { x: n.x, y: n.y };
                  }
                  return { x: n.y, y: n.x };
                };
                
                const pos = getNodePos(node);
                const initialPos = parent ? getNodePos(parent) : pos;
                
                const isNodeActive = activePathIds ? activePathIds.has(data.id) : true;

                // Adjust ports based on projection
                const parentPort = projection === 'vertical' ? { cx: 40, cy: -15 } : { cx: -10, cy: 0 };
                const childPort = projection === 'vertical' ? { cx: 40, cy: 15 } : { cx: 90, cy: 0 };

                // Rotation for radial text readability (optional, for now we keep boxes un-rotated for clarity, or apply light rotation)
                // We'll keep radial nodes axis-aligned for readability, which works well if radius is large enough.

                return (
                  <motion.g 
                    key={`node-${data.id}`}
                    initial={{ opacity: 0, x: initialPos.x, y: initialPos.y }}
                    animate={{ opacity: isNodeActive ? 1 : 0.15, x: pos.x, y: pos.y }}
                    exit={{ opacity: 0, x: initialPos.x, y: initialPos.y, transition: { duration: 0.3 } }}
                    transition={{ duration: 0.4, ease: "easeInOut" }}
                    onClick={(e: React.MouseEvent) => {
                      e.stopPropagation();
                      if (hasChildren) handleNodeClick(data);
                    }}
                    onMouseEnter={(e: React.MouseEvent) => {
                      setHoveredNodeId(data.id);
                      setTooltip({ data, x: e.clientX, y: e.clientY });
                    }}
                    onMouseMove={(e: React.MouseEvent) => {
                      setTooltip({ data, x: e.clientX, y: e.clientY });
                    }}
                    onMouseLeave={() => {
                      setHoveredNodeId(null);
                      setTooltip(null);
                    }}
                    className={cn(
                      hasChildren ? "cursor-pointer" : "cursor-default"
                    )}
                  >
                    <motion.g
                      animate={projection === 'radial' ? { rotate: (node.x * 180 / Math.PI - 90) } : { rotate: 0 }}
                      className="origin-center"
                    >
                      <rect
                        x={-10}
                        y={-15}
                        width={100}
                        height={30}
                        rx={2}
                        fill="#111113"
                        stroke={colorMode === 'gradient' ? probColor : (isHighProb ? '#fbbf24' : '#3f3f46')}
                        strokeWidth={isHighProb || colorMode === 'gradient' ? 1.5 : 1}
                        className={cn(
                          "transition-colors",
                          hasChildren && "hover:stroke-symm-gold"
                        )}
                      />
                      
                      {/* Left Port */}
                      {parent && (
                        <circle cx={parentPort.cx} cy={parentPort.cy} r={3} fill="#111113" stroke="#52525b" strokeWidth={1.5} />
                      )}
                      
                      {/* Right Port */}
                      {(hasChildren || isCollapsed) && (
                        <circle cx={childPort.cx} cy={childPort.cy} r={isCollapsed ? 4 : 3} fill={isCollapsed ? "#fbbf24" : "#111113"} stroke={isCollapsed ? "#fbbf24" : "#52525b"} strokeWidth={1.5} />
                      )}
                      
                      <text
                        x={0}
                        y={0}
                        dy="0.32em"
                        fill={colorMode === 'gradient' ? d3.interpolateRgb('#a1a1aa', '#fbbf24')(data.probability) : (isHighProb ? '#fbbf24' : '#a1a1aa')}
                        fontSize="12px"
                        fontWeight="500"
                        className="select-none pointer-events-none"
                      >
                        {data.prefix}
                      </text>
                      
                      <text
                        x={80}
                        y={-20}
                        fill="#71717a"
                        fontSize="10px"
                        textAnchor="end"
                        className="select-none pointer-events-none font-mono"
                      >
                        {(data.probability * 100).toFixed(1)}%
                      </text>
                      
                      {/* State Badge on Node */}
                      {data.state && (
                        <text
                          x={0}
                          y={-22}
                          fill={getStateColor(data.state)}
                          fontSize="8px"
                          className="font-mono tracking-widest pointer-events-none select-none uppercase"
                        >
                          {data.state}
                        </text>
                      )}
                    </motion.g>
                    
                  </motion.g>
                );
              })}
            </AnimatePresence>
          </g>
        </g>
      </svg>
      
      {/* Tooltip Overlay */}
      {tooltip && (
        <div 
          className="fixed z-50 bg-[#09090b] border border-[#27272a] rounded shadow-xl shadow-black/50 p-3 text-xs pointer-events-none flex flex-col gap-2 font-mono"
          style={{ left: tooltip.x + 15, top: tooltip.y + 15, minWidth: 220 }}
        >
          <div className="flex items-center gap-2 border-b border-[#27272a] pb-2">
            <span className="text-[#fbbf24] font-bold">{tooltip.data.prefix}</span>
            {tooltip.data.state && (
               <span className="ml-auto text-[9px] border border-current px-1 rounded uppercase tracking-wider" style={{ color: getStateColor(tooltip.data.state)}}>
                  {tooltip.data.state}
               </span>
            )}
          </div>
          
          <div className="flex justify-between">
            <span className="text-[#71717a]">Sequence Prob:</span>
            <span className="text-[#a1a1aa]">{(tooltip.data.probability * 100).toFixed(2)}%</span>
          </div>
          {tooltip.data.stepProbability && (
            <div className="flex justify-between">
               <span className="text-[#71717a]">Step Prob:</span>
               <span className="text-[#a1a1aa]">{(tooltip.data.stepProbability * 100).toFixed(2)}%</span>
            </div>
          )}
          {tooltip.data.tokens && (
            <div className="flex justify-between mt-1 pt-1 border-t border-[#27272a]/50 items-center">
               <span className="text-[#71717a]">Tokens:</span>
               <span className="text-[#fbbf24] bg-[#27272a]/30 px-1 rounded font-bold">[{tooltip.data.tokens.join(', ')}]</span>
            </div>
          )}
        </div>
      )}

      {/* Controls Overlay */}
      <div className="absolute bottom-4 right-4 flex gap-2">
        <button 
          onClick={focusBestPath}
          title="Focus Best Beam"
          className="bg-[#111113] border border-[#27272a] text-[#fbbf24] hover:bg-[#27272a] px-3 py-1.5 rounded transition-colors text-xs font-bold tracking-widest flex items-center gap-1.5 shadow-lg"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/></svg>
          BEST BEAM
        </button>
        <button 
          onClick={() => {
            if (svgRef.current && zoomBehavior.current) {
               d3.select(svgRef.current).transition().duration(500).call(
                 zoomBehavior.current.scaleBy, 1.2
               );
            }
          }}
          className="bg-symm-panel border border-symm-border text-symm-text hover:text-symm-gold p-1.5 rounded transition-colors shadow-lg"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 5v14M5 12h14"/></svg>
        </button>
        <button 
          onClick={() => {
             if (svgRef.current && zoomBehavior.current) {
               d3.select(svgRef.current).transition().duration(500).call(
                 zoomBehavior.current.scaleBy, 0.8
               );
            }
          }}
          className="bg-symm-panel border border-symm-border text-symm-text hover:text-symm-gold p-1.5 rounded transition-colors shadow-lg"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M5 12h14"/></svg>
        </button>
      </div>
    </div>
  );
};
