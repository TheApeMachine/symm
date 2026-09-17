import React, { useState } from 'react';
import { RadixTreeViz } from './components/RadixTreeViz';
import { ImpulseMapViz } from './components/ImpulseMapViz';
import { ForwardLearningViz } from './components/ForwardLearningViz';
import { generateMockTrie } from './data/mockTrie';
import { generateMockImpulseNodes } from './data/mockImpulseMap';
import { Activity, Brain, Compass, LineChart, Network, SlidersHorizontal, Terminal, Zap } from 'lucide-react';

export default function App() {
  const [minProbability, setMinProbability] = useState<number>(0.0);
  const [colorMode, setColorMode] = useState<'threshold' | 'gradient'>('threshold');
  const [projection, setProjection] = useState<'horizontal' | 'vertical' | 'radial'>('horizontal');
  const [activeTab, setActiveTab] = useState<'cognitive' | 'impulse' | 'dashboard' | 'forward' | 'diagnostics' | 'hindsight'>('forward');
  
  const trieData = generateMockTrie();
  const [impulseData] = useState(() => generateMockImpulseNodes(120));

  return (
    <div className="flex h-screen bg-[#050505] text-[#a1a1aa] font-mono text-sm overflow-hidden selection:bg-[#fbbf24] selection:text-black">
      
      {/* Sidebar */}
      <aside className="w-64 border-r border-[#27272a] bg-[#09090b] flex flex-col">
        <div className="h-12 border-b border-[#27272a] flex items-center px-4 gap-3 text-xs tracking-widest text-[#71717a]">
          <Compass className="w-4 h-4 text-[#fbbf24]" />
          <span>SURFACES</span>
        </div>
        
        <nav className="flex-1 overflow-y-auto py-2">
          <ul className="space-y-0.5">
            {[
              { id: 'dashboard', label: 'Dashboard', icon: LineChart },
              { id: 'forward', label: 'Forward learning', icon: Activity },
              { id: 'diagnostics', label: 'System diagnostics', icon: Terminal },
              { id: 'impulse', label: 'Impulse map', icon: Zap },
              { id: 'cognitive', label: 'Cognitive tree', icon: Network },
              { id: 'hindsight', label: 'Hindsight', icon: Brain },
            ].map((item) => (
              <li key={item.id}>
                <button
                  onClick={() => setActiveTab(item.id as any)}
                  className={`w-full flex items-center gap-3 px-4 py-2 text-left transition-colors ${
                    activeTab === item.id 
                      ? 'bg-[#18181b] text-white border-l-2 border-[#fbbf24]' 
                      : 'text-[#a1a1aa] hover:bg-[#111113] hover:text-[#d4d4d4] border-l-2 border-transparent'
                  }`}
                >
                  <item.icon className={`w-4 h-4 ${activeTab === item.id ? 'text-[#fbbf24]' : 'text-[#52525b]'}`} />
                  <span>{item.label}</span>
                </button>
              </li>
            ))}
          </ul>
        </nav>
        
        <div className="p-4 border-t border-[#27272a] text-xs">
          <div className="flex justify-between items-center mb-2">
            <span className="text-[#52525b] uppercase tracking-widest">Engine</span>
          </div>
          <div className="space-y-1">
            <div className="flex justify-between"><span className="text-[#52525b]">seq</span><span className="text-white">7749</span></div>
            <div className="flex justify-between"><span className="text-[#52525b]">meas</span><span className="text-white">10</span></div>
            <div className="flex justify-between"><span className="text-[#52525b]">open</span><span className="text-white">0</span></div>
          </div>
        </div>
      </aside>

      {/* Main Content */}
      <main className="flex-1 flex flex-col min-w-0">
        
        {/* Top Header */}
        <header className="h-12 border-b border-[#27272a] bg-[#09090b] flex items-center px-4 justify-between shrink-0">
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <div className="w-4 h-4 rounded-full border border-[#fbbf24] flex items-center justify-center">
                <div className="w-2 h-2 rounded-full bg-[#fbbf24]"></div>
              </div>
              <span className="font-bold text-white tracking-widest">SYMM</span>
            </div>
            
            <div className="h-4 w-px bg-[#27272a] mx-2"></div>
            
            <div className="flex items-center gap-2 text-xs border border-[#27272a] bg-[#111113] px-2 py-1 rounded">
              <div className="w-1.5 h-1.5 rounded-full bg-[#22c55e]"></div>
              <span>RTC LIVE · CONNECTED</span>
            </div>
            
            <div className="flex items-center gap-2 text-xs border border-[#0ea5e9]/30 text-[#0ea5e9] bg-[#0ea5e9]/10 px-2 py-1 rounded">
              <div className="w-1.5 h-1.5 rounded-full bg-[#0ea5e9]"></div>
              <span>AGENT · LEARNING</span>
            </div>
          </div>
          
          <div className="flex items-center gap-6 text-xs text-[#52525b]">
            <div className="flex items-center gap-2">
              <span>SKILL</span>
              <span className="text-white">18.7%</span>
            </div>
            <div className="flex items-center gap-2">
              <span>EDGE</span>
              <span className="text-white">-0.1 bp</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-white">0</span>
              <span>open positions</span>
            </div>
            
            <div className="h-4 w-px bg-[#27272a] mx-2"></div>
            
            <div className="px-2 py-1 border border-[#fbbf24]/30 text-[#fbbf24] bg-[#fbbf24]/5 rounded font-bold">
              BTC/USD
            </div>
          </div>
        </header>

        {/* Workspace */}
        <div className="flex-1 p-4 flex flex-col gap-4 overflow-hidden bg-[#050505]">
          
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <h1 className="uppercase tracking-widest text-[#d4d4d4] text-xs font-bold">
                {activeTab === 'cognitive' ? 'COGNITIVE TREE / BEAM SEARCH' : (activeTab === 'impulse' ? 'IMPULSE MAP / SYMPATHY CLUSTERING' : activeTab.toUpperCase())}
              </h1>
              <span className="text-[#52525b] text-xs">Horizon 132 ms · 8 epochs observed</span>
            </div>
            
            {/* Toolbar */}
            {activeTab === 'cognitive' && (
            <div className="flex items-center gap-4 text-xs">
              <div className="flex items-center gap-1 bg-[#111113] border border-[#27272a] p-1 rounded">
                <button 
                  onClick={() => setProjection('horizontal')}
                  className={`px-3 py-1 rounded transition-colors ${projection === 'horizontal' ? 'bg-[#27272a] text-[#fbbf24]' : 'text-[#71717a] hover:text-[#a1a1aa]'}`}
                >
                  Horizontal
                </button>
                <button 
                  onClick={() => setProjection('vertical')}
                  className={`px-3 py-1 rounded transition-colors ${projection === 'vertical' ? 'bg-[#27272a] text-[#fbbf24]' : 'text-[#71717a] hover:text-[#a1a1aa]'}`}
                >
                  Vertical
                </button>
                <button 
                  onClick={() => setProjection('radial')}
                  className={`px-3 py-1 rounded transition-colors ${projection === 'radial' ? 'bg-[#27272a] text-[#fbbf24]' : 'text-[#71717a] hover:text-[#a1a1aa]'}`}
                >
                  Radial
                </button>
              </div>
              <div className="flex items-center gap-1 bg-[#111113] border border-[#27272a] p-1 rounded">
                <button 
                  onClick={() => setColorMode('threshold')}
                  className={`px-3 py-1 rounded transition-colors ${colorMode === 'threshold' ? 'bg-[#27272a] text-[#fbbf24]' : 'text-[#71717a] hover:text-[#a1a1aa]'}`}
                >
                  Threshold
                </button>
                <button 
                  onClick={() => setColorMode('gradient')}
                  className={`px-3 py-1 rounded transition-colors ${colorMode === 'gradient' ? 'bg-[#27272a] text-[#fbbf24]' : 'text-[#71717a] hover:text-[#a1a1aa]'}`}
                >
                  Gradient
                </button>
              </div>
              <div className="flex items-center gap-2 bg-[#111113] border border-[#27272a] px-3 py-1.5 rounded">
                <SlidersHorizontal className="w-3.5 h-3.5 text-[#a1a1aa]" />
                <span className="text-[#71717a]">Prob Threshold:</span>
                <span className="text-[#fbbf24] w-12 text-right">{(minProbability * 100).toFixed(0)}%</span>
                <input 
                  type="range" 
                  min="0" 
                  max="0.5" 
                  step="0.01" 
                  value={minProbability} 
                  onChange={(e) => setMinProbability(parseFloat(e.target.value))}
                  className="w-24 accent-[#fbbf24] bg-[#27272a] h-1 rounded-full appearance-none outline-none"
                />
              </div>
            </div>
            )}
          </div>
          
          {/* Main Visualizer Panel */}
          {activeTab === 'forward' ? (
            <div className="flex-1 min-h-0 flex flex-col">
              <ForwardLearningViz />
            </div>
          ) : (
          <div className="flex-1 flex flex-col bg-[#111113] border border-[#27272a] rounded-sm overflow-hidden">
            {activeTab === 'cognitive' ? (
              <>
                <div className="h-8 border-b border-[#27272a] flex items-center justify-between px-4 text-xs bg-[#09090b]">
                  <div className="flex gap-4">
                    <span className="text-[#fbbf24] border-b border-[#fbbf24] py-1.5">Radix Trie</span>
                    <span className="text-[#52525b] hover:text-[#a1a1aa] py-1.5 cursor-pointer transition-colors">Action spectrum</span>
                    <span className="text-[#52525b] hover:text-[#a1a1aa] py-1.5 cursor-pointer transition-colors">Trajectory</span>
                  </div>
                  <div className="text-[#52525b]">
                    interactive topology · probability pruning · scroll to zoom
                  </div>
                </div>
                
                <div className="flex-1 relative">
                  <RadixTreeViz data={trieData} minProbability={minProbability} colorMode={colorMode} projection={projection} />
                </div>
              </>
            ) : (activeTab === 'impulse') ? (
              <ImpulseMapViz data={impulseData} />
            ) : (
              <div className="flex-1 flex items-center justify-center text-[#52525b] font-mono text-sm tracking-widest">
                MODULE NOT INITIALIZED
              </div>
            )}
          </div>
          )}
          
          {/* Bottom Data Panel */}
          {activeTab === 'cognitive' && (
          <div className="h-48 shrink-0 bg-[#111113] border border-[#27272a] rounded-sm overflow-hidden flex flex-col">
             <div className="h-8 border-b border-[#27272a] flex items-center px-4 text-xs bg-[#09090b] text-[#52525b] uppercase tracking-widest">
               Feasible actions at this impulse
             </div>
             <div className="flex-1 overflow-auto">
               <table className="w-full text-left text-xs">
                 <thead className="text-[#52525b] border-b border-[#27272a]">
                   <tr>
                     <th className="font-normal px-4 py-2 w-12">Rank</th>
                     <th className="font-normal px-4 py-2 w-32">Action</th>
                     <th className="font-normal px-4 py-2">Prefix Sequence</th>
                     <th className="font-normal px-4 py-2 text-right">Probability</th>
                     <th className="font-normal px-4 py-2 text-right">State</th>
                   </tr>
                 </thead>
                 <tbody className="divide-y divide-[#27272a]">
                   <tr className="hover:bg-[#18181b] transition-colors group cursor-default">
                     <td className="px-4 py-2 text-[#fbbf24]">1</td>
                     <td className="px-4 py-2 text-[#fbbf24]">wait</td>
                     <td className="px-4 py-2 text-[#a1a1aa]">ROOT / wait / 100ms</td>
                     <td className="px-4 py-2 text-right text-white">30.0%</td>
                     <td className="px-4 py-2 text-right"><span className="border border-[#27272a] px-1.5 py-0.5 rounded text-[#52525b] group-hover:border-[#fbbf24]/50 group-hover:text-[#fbbf24]">EVALUATED</span></td>
                   </tr>
                   <tr className="hover:bg-[#18181b] transition-colors group cursor-default">
                     <td className="px-4 py-2 text-[#fbbf24]">2</td>
                     <td className="px-4 py-2 text-[#fbbf24]">enter · 1/8</td>
                     <td className="px-4 py-2 text-[#a1a1aa]">ROOT / enter / 1/8 / @ask</td>
                     <td className="px-4 py-2 text-right text-white">12.0%</td>
                     <td className="px-4 py-2 text-right"><span className="border border-[#22c55e]/30 px-1.5 py-0.5 rounded text-[#22c55e]">POLICY CHOICE</span></td>
                   </tr>
                   <tr className="hover:bg-[#18181b] transition-colors group cursor-default">
                     <td className="px-4 py-2 text-[#d4d4d4]">3</td>
                     <td className="px-4 py-2 text-[#d4d4d4]">enter · 1/8</td>
                     <td className="px-4 py-2 text-[#a1a1aa]">ROOT / enter / 1/8 / @bid</td>
                     <td className="px-4 py-2 text-right text-white">8.0%</td>
                     <td className="px-4 py-2 text-right"><span className="border border-[#27272a] px-1.5 py-0.5 rounded text-[#52525b]">EVALUATED</span></td>
                   </tr>
                 </tbody>
               </table>
             </div>
          </div>
          )}

        </div>
      </main>

    </div>
  );
}
