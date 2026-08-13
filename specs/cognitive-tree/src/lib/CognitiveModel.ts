export interface TrieNode {
  id: string;
  token: string;
  children: Record<string, TrieNode>;
  counts: Record<string, number>;
  totalVisits: number;
  depth: number;
  lastUpdateStep: number;
}

export interface Episode {
  id: string;
  tokens: string[];
  label: string;
  timestamp: number;
}

export interface BeamPath {
  tokens: string[];
  logProb: number;
}

export class CognitiveModel {
  nextNodeId = 1;
  conceptCounter = 1;
  trie: TrieNode = { id: '0', token: '', children: {}, counts: {}, totalVisits: 0, depth: 0, lastUpdateStep: 0 };
  classes: Set<string> = new Set();
  classTotals: Record<string, number> = {};
  nodeCount = 1;
  decayFactor = 0.995;
  currentStep = 0;
  vocabulary: Set<string> = new Set();
  coOccurrence: Record<string, Record<string, number>> = {};
  extractedSymbols: { symbol: string, label: string, score: number }[] = [];
  
  // Episodic Memory
  episodicBuffer: Episode[] = [];
  maxEpisodicSize = 20;

  tokenize(text: string): string[] {
    return text.split(/[_ ]+/).filter(t => t.length > 0);
  }

  getEffectiveCount(node: TrieNode, label?: string): number {
    const steps = this.currentStep - node.lastUpdateStep;
    const decay = Math.pow(this.decayFactor, steps);
    if (label !== undefined) {
      return (node.counts[label] || 0) * decay;
    }
    return node.totalVisits * decay;
  }

  updateNode(node: TrieNode, label: string, learningRate: number = 1.0) {
    const steps = this.currentStep - node.lastUpdateStep;
    if (steps > 0) {
      const decay = Math.pow(this.decayFactor, steps);
      for (const l in node.counts) {
        node.counts[l] *= decay;
      }
      node.totalVisits *= decay;
      node.lastUpdateStep = this.currentStep;
    }
    node.counts[label] = (node.counts[label] || 0) + learningRate;
    node.totalVisits += learningRate;
  }

  experience(sequence: string, providedLabel?: string): { label: string, surprisal: number, learningRate: number, isNewConcept: boolean } {
    const tokens = this.tokenize(sequence);
    if (tokens.length === 0) return { label: 'None', surprisal: 0, learningRate: 0, isNewConcept: false };

    const surprisals = this.getSurprisal(sequence);
    const avgSurprisal = surprisals.length > 0 ? surprisals.reduce((sum, s) => sum + s.surprisal, 0) / surprisals.length : 0;

    let label = providedLabel;
    let isNewConcept = false;

    if (!label) {
      if (this.classes.size === 0) {
        label = `Concept_${this.conceptCounter++}`;
        isNewConcept = true;
      } else {
        const { scores } = this.classify(sequence);
        let maxScore = -1;
        let bestClass = '';
        for (const [cls, score] of Object.entries(scores)) {
          if (score > maxScore) { maxScore = score; bestClass = cls; }
        }
        
        if (maxScore < 50) { // Confidence threshold for spawning new concepts
          label = `Concept_${this.conceptCounter++}`;
          isNewConcept = true;
        } else {
          label = bestClass;
        }
      }
    }

    // Surprise-Modulated Plasticity
    // If surprisal is 0, learning rate is 0.1 (baseline maintenance).
    // If surprisal is > 2 bits, learning rate approaches 1.0.
    const learningRate = Math.min(1.0, 0.1 + (avgSurprisal / 2.0));

    this.train(sequence, label as string, learningRate);

    return { label: label as string, surprisal: avgSurprisal, learningRate, isNewConcept };
  }

  train(sequence: string, label: string, learningRate: number = 1.0) {
    this.classes.add(label);
    this.currentStep++;
    
    for (const l of this.classes) {
      this.classTotals[l] = (this.classTotals[l] || 0) * this.decayFactor;
    }
    this.classTotals[label] = (this.classTotals[label] || 0) + learningRate;

    const tokens = [...this.tokenize(sequence), "$"];
    const words = tokens.filter(t => t !== "$");
    
    // Add to Episodic Buffer
    this.episodicBuffer.unshift({
      id: (this.nextNodeId++).toString(),
      tokens: words,
      label,
      timestamp: this.currentStep
    });
    if (this.episodicBuffer.length > this.maxEpisodicSize) {
      this.episodicBuffer.pop();
    }
    
    for (let i = 0; i < words.length; i++) {
      const w1 = words[i];
      this.vocabulary.add(w1);
      if (!this.coOccurrence[w1]) this.coOccurrence[w1] = {};
      for (let j = Math.max(0, i - 2); j <= Math.min(words.length - 1, i + 2); j++) {
        if (i !== j) {
          const w2 = words[j];
          this.coOccurrence[w1][w2] = (this.coOccurrence[w1][w2] || 0) + 1;
        }
      }
    }

    // Train suffix paths up to length 5
    for (let i = 0; i < tokens.length; i++) {
      let node = this.trie;
      this.updateNode(node, label, learningRate);
      
      for (let j = i; j < Math.min(tokens.length, i + 5); j++) {
        const token = tokens[j];
        if (!node.children[token]) {
          node.children[token] = {
            id: (this.nextNodeId++).toString(),
            token,
            children: {},
            counts: {},
            totalVisits: 0,
            depth: node.depth + 1,
            lastUpdateStep: this.currentStep
          };
          this.nodeCount++;
        }
        node = node.children[token];
        this.updateNode(node, label, learningRate);
      }
    }
    
    this.prune();
    this.extractSymbols();
  }

  countNodes(node: TrieNode): number {
    let count = 1;
    for (const key in node.children) {
      count += this.countNodes(node.children[key]);
    }
    return count;
  }

  prune() {
    if (this.currentStep % 10 !== 0) return;
    const pruneNode = (node: TrieNode) => {
      for (const key in node.children) {
        const child = node.children[key];
        if (this.getEffectiveCount(child) < 0.05) {
          this.nodeCount -= this.countNodes(child);
          delete node.children[key];
        } else {
          pruneNode(child);
        }
      }
    };
    pruneNode(this.trie);
  }

  getCharNgrams(word: string, n: number = 2): Record<string, number> {
    const ngrams: Record<string, number> = {};
    const padded = "^" + word + "$";
    for (let i = 0; i <= padded.length - n; i++) {
      const gram = padded.substring(i, i + n);
      ngrams[gram] = (ngrams[gram] || 0) + 1;
    }
    return ngrams;
  }

  getNgramSimilarity(w1: string, w2: string): number {
    const ng1 = this.getCharNgrams(w1);
    const ng2 = this.getCharNgrams(w2);
    let dot = 0, m1 = 0, m2 = 0;
    for (const k in ng1) {
      dot += ng1[k] * (ng2[k] || 0);
      m1 += ng1[k] * ng1[k];
    }
    for (const k in ng2) {
      m2 += ng2[k] * ng2[k];
    }
    return m1 && m2 ? dot / (Math.sqrt(m1) * Math.sqrt(m2)) : 0;
  }

  levenshtein(a: string, b: string): number {
    const matrix = Array.from({ length: a.length + 1 }, () => new Array(b.length + 1).fill(0));
    for (let i = 0; i <= a.length; i++) matrix[i][0] = i;
    for (let j = 0; j <= b.length; j++) matrix[0][j] = j;
    for (let i = 1; i <= a.length; i++) {
      for (let j = 1; j <= b.length; j++) {
        const cost = a[i - 1] === b[j - 1] ? 0 : 1;
        matrix[i][j] = Math.min(
          matrix[i - 1][j] + 1,
          matrix[i][j - 1] + 1,
          matrix[i - 1][j - 1] + cost
        );
      }
    }
    return matrix[a.length][b.length];
  }

  getSemanticEquivalent(word: string): { original: string, mapped: string, similarity: number } {
    if (this.vocabulary.has(word)) return { original: word, mapped: word, similarity: 1 };
    
    let bestWord = word;
    let bestSim = -1;
    
    for (const knownWord of this.vocabulary) {
      if (Math.abs(knownWord.length - word.length) <= 2) {
         const ed = this.levenshtein(word, knownWord);
         if (ed <= 1) {
             return { original: word, mapped: knownWord, similarity: 0.95 };
         }
      }
      
      const sim = this.getNgramSimilarity(word, knownWord);
      if (sim > bestSim) {
          bestSim = sim;
          bestWord = knownWord;
      }
    }
    
    return { original: word, mapped: bestWord, similarity: bestSim > 0 ? bestSim : 1 };
  }

  getAttentionContext(context: string) {
    const words = this.tokenize(context);
    return words.map(w => this.getSemanticEquivalent(w));
  }

  getEpisodicProbs(contextTokens: string[]): Record<string, number> {
    const probs: Record<string, number> = {};
    let totalWeight = 0;

    if (contextTokens.length === 0) return probs;

    for (let i = 0; i < this.episodicBuffer.length; i++) {
      const ep = this.episodicBuffer[i];
      const recencyWeight = Math.pow(0.9, i); // Newer memories are stronger

      let matchFound = false;
      for (let ctxLen = contextTokens.length; ctxLen > 0 && !matchFound; ctxLen--) {
        const searchCtx = contextTokens.slice(-ctxLen);
        
        for (let j = 0; j <= ep.tokens.length - searchCtx.length; j++) {
          let match = true;
          for (let k = 0; k < searchCtx.length; k++) {
            if (ep.tokens[j + k] !== searchCtx[k]) {
              match = false;
              break;
            }
          }
          if (match && j + searchCtx.length < ep.tokens.length) {
            const nextToken = ep.tokens[j + searchCtx.length];
            const weight = recencyWeight * (ctxLen / contextTokens.length); // Stronger if longer context matched
            probs[nextToken] = (probs[nextToken] || 0) + weight;
            totalWeight += weight;
            matchFound = true;
            break; // Found the most recent occurrence in this episode
          }
        }
      }
    }

    if (totalWeight > 0) {
      for (const k in probs) probs[k] /= totalWeight;
    }
    return probs;
  }

  getInterpolatedProbs(contextTokens: string[], label?: string): Record<string, number> {
    const semanticProbs: Record<string, number> = {};
    const maxSuffix = Math.min(contextTokens.length, 4);
    let semanticTotalWeight = 0;
    
    for (let k = 0; k <= maxSuffix; k++) {
      const suffix = contextTokens.slice(contextTokens.length - k);
      let node = this.trie;
      let match = true;
      for (const token of suffix) {
        if (node.children[token]) {
          node = node.children[token];
        } else {
          let bestFuzzyMatch = null;
          let bestFuzzyCount = -1;
          for (const childToken in node.children) {
             if (this.levenshtein(token, childToken) <= 1) {
                 const count = this.getEffectiveCount(node.children[childToken], label);
                 if (count > bestFuzzyCount) {
                     bestFuzzyCount = count;
                     bestFuzzyMatch = node.children[childToken];
                 }
             }
          }
          if (bestFuzzyMatch) {
              node = bestFuzzyMatch;
          } else {
              match = false; break;
          }
        }
      }
      
      if (match) {
        const weight = k + 1; // Linear schedule instead of exponential
        semanticTotalWeight += weight;
        
        let nodeTotal = 0;
        for (const childToken in node.children) {
          nodeTotal += this.getEffectiveCount(node.children[childToken], label);
        }
        
        const V = Object.keys(node.children).length || 1;
        for (const childToken in node.children) {
          const count = this.getEffectiveCount(node.children[childToken], label);
          const p = (count + 0.1) / (nodeTotal + 0.1 * V);
          semanticProbs[childToken] = (semanticProbs[childToken] || 0) + p * weight;
        }
      }
    }
    
    if (semanticTotalWeight > 0) {
      for (const token in semanticProbs) {
        semanticProbs[token] /= semanticTotalWeight;
      }
    }

    // Episodic Memory (RAG Buffer)
    const episodicProbs = this.getEpisodicProbs(contextTokens);
    const hasEpisodic = Object.keys(episodicProbs).length > 0;
    
    // Blend Semantic and Episodic
    // If we have a strong episodic match, we give it a 30% boost (one-shot learning effect)
    const episodicWeight = hasEpisodic ? 0.3 : 0;
    const semanticWeight = 1.0 - episodicWeight;

    const probs: Record<string, number> = {};
    let totalWeight = 0;
    const allTokens = new Set([...Object.keys(semanticProbs), ...Object.keys(episodicProbs)]);
    for (const t of allTokens) {
      const sProb = semanticProbs[t] || 0;
      const eProb = episodicProbs[t] || 0;
      probs[t] = (sProb * semanticWeight) + (eProb * episodicWeight);
      totalWeight += probs[t];
    }

    if (totalWeight > 0) {
      for (const k in probs) probs[k] /= totalWeight;
    }

    return probs;
  }

  classify(context: string): { scores: Record<string, number>, contributions: Record<string, { token: string, logProb: number }[]> } {
    const tokens = this.tokenize(context);
    const logProbs: Record<string, number> = {};
    const contributions: Record<string, { token: string, logProb: number }[]> = {};
    
    for (const label of this.classes) {
      logProbs[label] = 0;
      contributions[label] = [];
    }

    for (const label of this.classes) {
      let logProb = Math.log((this.classTotals[label] || 0.1) / (this.currentStep || 1));
      contributions[label].push({ token: 'PRIOR', logProb });
      
      for (let i = 0; i <= tokens.length; i++) {
        const ctx = tokens.slice(Math.max(0, i - 3), i);
        const probs = this.getInterpolatedProbs(ctx, label);
        if (i < tokens.length) {
            const p = probs[tokens[i]] || 0.001;
            const lp = Math.log(p);
            logProb += lp;
            contributions[label].push({ token: tokens[i], logProb: lp });
        }
      }
      logProbs[label] = logProb;
    }

    const maxLog = Math.max(...Object.values(logProbs), -Infinity);
    let sumExp = 0;
    const expScores: Record<string, number> = {};
    for (const label in logProbs) {
      const exp = Math.exp(logProbs[label] - maxLog);
      expScores[label] = exp;
      sumExp += exp;
    }
    
    for (const label in expScores) {
      expScores[label] = sumExp > 0 ? (expScores[label] / sumExp) * 100 : 0;
    }

    return { scores: expScores, contributions };
  }

  getSurprisal(context: string) {
    const tokens = this.tokenize(context);
    const surprisals: { token: string, surprisal: number }[] = [];
    
    for (let i = 0; i < tokens.length; i++) {
      const ctx = tokens.slice(Math.max(0, i - 3), i);
      const probs = this.getInterpolatedProbs(ctx);
      const p = probs[tokens[i]] || 0.001;
      surprisals.push({ token: tokens[i], surprisal: -Math.log2(p) });
    }
    return surprisals;
  }

  getNextProbabilities(context: string, label: string, temperature: number) {
    const tokens = this.tokenize(context);
    const probs = this.getInterpolatedProbs(tokens, label);
    
    let options = Object.entries(probs).map(([token, prob]) => ({ token, prob }));
    if (options.length === 0) return [];

    if (temperature === 0) {
      const maxP = Math.max(...options.map(o => o.prob));
      const best = options.filter(o => o.prob === maxP);
      return best.map(o => ({ token: o.token, prob: 1 / best.length })).sort((a, b) => b.prob - a.prob);
    }

    options = options.map(o => ({ token: o.token, prob: Math.pow(o.prob, 1 / temperature) }));
    const sum = options.reduce((acc, o) => acc + o.prob, 0);
    return options.map(o => ({ token: o.token, prob: o.prob / sum })).sort((a, b) => b.prob - a.prob);
  }

  generateDream(context: string, label: string, temperature: number, maxLength: number = 20): string {
    let tokens = this.tokenize(context);
    let result = [...tokens];
    const recentTokens: string[] = [];

    for (let i = 0; i < maxLength; i++) {
      let probs = this.getNextProbabilities(result.join(" "), label, temperature);
      if (probs.length === 0) break;
      
      probs = probs.map(p => {
         const penalty = recentTokens.includes(p.token) ? 0.5 : 1;
         return { ...p, prob: p.prob * penalty };
      });
      const sum = probs.reduce((acc, p) => acc + p.prob, 0);
      probs.forEach(p => p.prob /= sum);

      let nextToken = probs[probs.length - 1].token;
      if (temperature === 0) {
          const maxP = Math.max(...probs.map(p => p.prob));
          const best = probs.filter(p => p.prob === maxP);
          nextToken = best[Math.floor(Math.random() * best.length)].token;
      } else {
          const r = Math.random();
          let running = 0;
          for (const opt of probs) {
            running += opt.prob;
            if (r <= running) {
                nextToken = opt.token;
                break;
            }
          }
      }

      if (nextToken === '$') break;
      result.push(nextToken);
      
      recentTokens.push(nextToken);
      if (recentTokens.length > 3) recentTokens.shift();
    }
    
    return result.slice(tokens.length).join("_");
  }

  beamSearch(context: string, beamWidth: number = 3, maxHops: number = 4, label?: string): { sequence: string, score: number }[] {
    const initialTokens = this.tokenize(context);
    if (initialTokens.length === 0) return [];

    let beams = [{ tokens: initialTokens, score: 0 }];
    
    for (let i = 0; i < maxHops; i++) {
      let newBeams: { tokens: string[], score: number }[] = [];
      
      for (const beam of beams) {
        if (beam.tokens[beam.tokens.length - 1] === '$') {
          newBeams.push(beam);
          continue;
        }
        
        const probs = this.getNextProbabilities(beam.tokens.join(" "), label, 1.0);
        if (probs.length === 0) {
          newBeams.push(beam);
          continue;
        }
        
        for (const p of probs.slice(0, beamWidth)) {
          newBeams.push({
            tokens: [...beam.tokens, p.token],
            score: beam.score + Math.log(p.prob)
          });
        }
      }
      
      newBeams.sort((a, b) => b.score - a.score);
      
      // Deduplicate
      const uniqueBeams: { tokens: string[], score: number }[] = [];
      const seen = new Set<string>();
      for (const b of newBeams) {
        const key = b.tokens.join('_');
        if (!seen.has(key)) {
          seen.add(key);
          uniqueBeams.push(b);
          if (uniqueBeams.length >= beamWidth) break;
        }
      }
      beams = uniqueBeams;
      
      if (beams.every(b => b.tokens[b.tokens.length - 1] === '$')) break;
    }
    
    return beams.map(b => ({
      sequence: b.tokens.slice(initialTokens.length).filter(t => t !== '$').join("_"),
      score: b.score
    }));
  }

  extractSymbols() {
    const candidates: Record<string, Record<string, number>> = {};
    
    const traverse = (node: TrieNode, path: string[]) => {
      if (path.length > 0) {
        const symbol = path.join("_");
        if (!candidates[symbol]) candidates[symbol] = {};
        for (const label of this.classes) {
          candidates[symbol][label] = (candidates[symbol][label] || 0) + this.getEffectiveCount(node, label);
        }
      }
      for (const key in node.children) {
        traverse(node.children[key], [...path, key]);
      }
    };
    
    for (const key in this.trie.children) {
        traverse(this.trie.children[key], [key]);
    }

    const scored: { symbol: string, label: string, score: number }[] = [];
    
    for (const [symbol, counts] of Object.entries(candidates)) {
      const total = Object.values(counts).reduce((a, b) => a + b, 0);
      if (total < 2) continue;

      for (const label of this.classes) {
        const count = counts[label] || 0;
        if (count === 0) continue;
        
        const pClassGivenSymbol = count / total;
        const distinctiveness = pClassGivenSymbol;
        const score = distinctiveness * Math.log(1 + count) * Math.sqrt(symbol.split("_").length);
        
        if (score > 1.5) {
          scored.push({ symbol, label, score });
        }
      }
    }

    this.extractedSymbols = scored.sort((a, b) => b.score - a.score).slice(0, 50);
    return this.extractedSymbols;
  }

  getPosteriorsOverTime(context: string) {
    const tokens = this.tokenize(context);
    const posteriors: Record<string, number>[] = [];
    
    let currentContext = "";
    posteriors.push(this.classify(currentContext).scores);
    for (let i = 0; i < tokens.length; i++) {
      currentContext += (i > 0 ? " " : "") + tokens[i];
      posteriors.push(this.classify(currentContext).scores);
    }
    
    return posteriors;
  }

  remSleepTick(temperature: number): { dream: string, label: string, distinctiveness: number } | null {
    const classArray = Array.from(this.classes);
    if (classArray.length === 0) return null;
    const randomClass = classArray[Math.floor(Math.random() * classArray.length)];

    const dream = this.generateDream("", randomClass, temperature, 10);
    if (dream.length < 2) return null;

    const { scores } = this.classify(dream);
    const targetScore = scores[randomClass] || 0;
    
    if (targetScore > 85) {
      const tokens = this.tokenize(dream);
      let node = this.trie;
      let isNovel = false;
      for (const t of tokens) {
          if (node.children[t]) node = node.children[t];
          else { isNovel = true; break; }
      }
      
      if (isNovel) {
          this.train(dream, randomClass);
          return { dream, label: randomClass, distinctiveness: targetScore };
      }
    }
    return null;
  }
}
