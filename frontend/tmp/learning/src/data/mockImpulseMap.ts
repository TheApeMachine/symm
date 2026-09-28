import { ImpulseNode } from "../types";

const CLUSTER_NAMES = [
  "Momentum / Trend",
  "Mean Reversion",
  "Orderbook Imbalance",
  "Volatility Breakout",
];

export const generateMockImpulseNodes = (count: number = 120): ImpulseNode[] => {
  const nodes: ImpulseNode[] = [];
  
  for (let i = 0; i < count; i++) {
    // Distribute nodes roughly across 4 clusters
    const cluster = Math.floor(Math.random() * 4);
    
    // SNR: Power law distribution, most are low SNR, few are high SNR (Gravity hubs)
    const snr = Math.max(1, Math.min(10, Math.pow(Math.random(), 3) * 12));
    
    // Initial activation is low
    const activation = Math.random() * 0.2;
    
    nodes.push({
      id: `node_${i}`,
      label: `Sig_${Math.floor(Math.random() * 1000)}`,
      cluster,
      snr,
      activation,
    });
  }
  
  return nodes;
};
