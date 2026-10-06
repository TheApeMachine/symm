export interface TrieNodeData {
  id: string;
  prefix: string;
  probability: number;
  stepProbability?: number;
  tokens?: string[];
  state?: 'EVALUATED' | 'POLICY CHOICE' | 'ESTIMATED';
  isEnd?: boolean;
  children?: TrieNodeData[];
  _children?: TrieNodeData[]; // Used to store collapsed children
}

export interface TreeLink {
  source: d3.HierarchyPointNode<TrieNodeData>;
  target: d3.HierarchyPointNode<TrieNodeData>;
}

export interface ImpulseNode {
  id: string;
  label: string;
  cluster: number;      // Determines attraction group (sympathy)
  snr: number;          // Signal-to-Noise ratio (determines mass/gravity)
  activation: number;   // Current heat/lighting up from market tape (0 to 1)
  x?: number;
  y?: number;
  vx?: number;
  vy?: number;
  gridX?: number;       // Initial layout target X
  gridY?: number;       // Initial layout target Y
}

export interface TrainingTick {
  id: number;
  timestamp: number;
  marketImpulse: number; // The underlying market wave/price
  agentAction: 'ENTER_ASK' | 'ENTER_BID' | 'WAIT' | 'RETREAT' | null;
  evaluationState: 'PENDING' | 'EVALUATED';
  edge: number; // Reward / Advantage (positive or negative)
}
