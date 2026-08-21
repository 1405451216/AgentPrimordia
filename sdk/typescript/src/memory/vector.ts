import type { VectorSearchResult } from '../types.js';

// HNSW (Hierarchical Navigable Small World) index for approximate nearest neighbor search.
// Complexity: O(log n) per query vs O(n) brute-force.
// Based on the algorithm described in "Efficient and robust approximate nearest neighbor search using Hierarchical Navigable Small World graphs" (Malkov & Yashunin, 2016).

interface HNSWNode {
  id: string;
  vector: number[];
  metadata?: Record<string, string>;
  // Connections at each layer. Layer 0 is the base layer (all nodes).
  // Higher layers have progressively fewer nodes.
  connections: number[][]; // connections[layer] = array of node indices
  maxLayer: number;
}

const DEFAULT_M = 16; // max connections per node per layer (except layer 0 which is 2*M)
const DEFAULT_EF_CONSTRUCTION = 200; // search width during construction
const DEFAULT_EF_SEARCH = 50; // search width during query
const _ML = 1 / Math.log(2); // level generation factor
const BRUTE_FORCE_THRESHOLD = 100; // below this size, brute-force is faster

export class VectorStore {
  private dim: number;
  private nodes: HNSWNode[] = [];
  private idToIndex: Map<string, number> = new Map();
  private entryPoint: number = -1; // index of the entry node
  private maxLayer: number = -1;
  private readonly M: number;
  private readonly efConstruction: number;
  private readonly efSearch: number;
  // 层级生成随机源；可注入固定种子 PRNG 获得确定性图构建（v5.1 召回门依赖可复现）
  private readonly random: () => number;

  constructor(
    dimensions: number = 16,
    opts?: { M?: number; efConstruction?: number; efSearch?: number; random?: () => number },
  ) {
    this.dim = dimensions;
    this.M = opts?.M ?? DEFAULT_M;
    this.efConstruction = opts?.efConstruction ?? DEFAULT_EF_CONSTRUCTION;
    this.efSearch = opts?.efSearch ?? DEFAULT_EF_SEARCH;
    this.random = opts?.random ?? Math.random;
  }

  add(id: string, vector: number[], metadata?: Record<string, string>): void {
    if (vector.length !== this.dim) {
      throw new Error(`vector dimension mismatch: expected ${this.dim}, got ${vector.length}`);
    }
    if (!vector.every(Number.isFinite)) {
      throw new Error('vector contains non-finite values (NaN or Infinity)');
    }

    // If already exists, remove and re-add
    if (this.idToIndex.has(id)) {
      this.delete(id);
    }

    const index = this.nodes.length;
    const level = this.randomLevel();

    const node: HNSWNode = {
      id,
      vector,
      metadata,
      connections: [],
      maxLayer: level,
    };

    // Initialize empty connection arrays for each layer
    for (let i = 0; i <= level; i++) {
      node.connections.push([]);
    }

    this.nodes.push(node);
    this.idToIndex.set(id, index);

    // First node becomes the entry point
    if (this.entryPoint === -1) {
      this.entryPoint = index;
      this.maxLayer = level;
      return;
    }

    // Insert into the graph
    this.insertNode(index, level);
  }

  search(query: number[], topK: number = 10): VectorSearchResult[] {
    if (query.length !== this.dim) {
      throw new Error(`query dimension mismatch: expected ${this.dim}, got ${query.length}`);
    }
    if (!query.every(Number.isFinite)) {
      throw new Error('query contains non-finite values (NaN or Infinity)');
    }

    if (this.nodes.length === 0) return [];

    // For small datasets, brute-force is faster (no graph traversal overhead)
    if (this.nodes.length <= BRUTE_FORCE_THRESHOLD) {
      return this.bruteForceSearch(query, topK);
    }

    // HNSW search: start from top layer, greedy descent, then ef-search at layer 0
    const ef = Math.max(this.efSearch, topK);
    let currentEntryPoint = this.entryPoint;

    // Greedy descent from top layer to layer 1
    for (let layer = this.maxLayer; layer > 0; layer--) {
      currentEntryPoint = this.greedySearchLayer(query, currentEntryPoint, layer, 1);
    }

    // ef-search at layer 0
    const candidates = this.searchLayer(query, currentEntryPoint, 0, ef);

    // Sort by similarity and return topK
    const results: VectorSearchResult[] = [];
    for (const idx of candidates) {
      const node = this.nodes[idx];
      results.push({
        id: node.id,
        score: cosineSimilarity(query, node.vector),
        metadata: node.metadata,
      });
    }
    results.sort((a, b) => b.score - a.score);
    return results.slice(0, topK);
  }

  delete(id: string): boolean {
    const index = this.idToIndex.get(id);
    if (index === undefined) return false;

    const node = this.nodes[index];

    // Remove connections from other nodes that point to this node
    for (let layer = 0; layer <= node.maxLayer; layer++) {
      for (const neighborIdx of node.connections[layer]) {
        const neighbor = this.nodes[neighborIdx];
        if (neighbor && layer < neighbor.connections.length) {
          neighbor.connections[layer] = neighbor.connections[layer].filter(
            (idx) => idx !== index,
          );
        }
      }
    }

    this.idToIndex.delete(id);

    // If this was the entry point, find a new one
    if (this.entryPoint === index) {
      if (this.nodes.length > 1) {
        // Find the node with the highest layer
        let maxLayerNode = -1;
        let maxLayer = -1;
        for (let i = 0; i < this.nodes.length; i++) {
          if (i !== index && this.nodes[i].maxLayer > maxLayer) {
            maxLayer = this.nodes[i].maxLayer;
            maxLayerNode = i;
          }
        }
        this.entryPoint = maxLayerNode;
        this.maxLayer = maxLayer;
      } else {
        this.entryPoint = -1;
        this.maxLayer = -1;
      }
    }

    // Mark node as deleted (keep array slot to preserve indices)
    this.nodes[index] = {
      id: '__deleted__',
      vector: [],
      metadata: undefined,
      connections: [],
      maxLayer: -1,
    };

    return true;
  }

  get(id: string): { vector: number[]; metadata?: Record<string, string> } | undefined {
    const index = this.idToIndex.get(id);
    if (index === undefined) return undefined;
    const node = this.nodes[index];
    return { vector: node.vector, metadata: node.metadata };
  }

  count(): number {
    return this.idToIndex.size;
  }

  dimensions(): number {
    return this.dim;
  }

  // ===== HNSW Internal Methods =====

  private randomLevel(): number {
    let level = 0;
    while (this.random() < 1 / Math.exp(1) && level < 16) {
      level++;
    }
    return level;
  }

  private insertNode(nodeIndex: number, nodeLevel: number): void {
    const node = this.nodes[nodeIndex];
    const entryPoint = this.entryPoint;

    // Greedy descent from top layer to nodeLevel + 1
    let currentEntryPoint = entryPoint;
    for (let layer = this.maxLayer; layer > nodeLevel; layer--) {
      currentEntryPoint = this.greedySearchLayer(node.vector, currentEntryPoint, layer, 1);
    }

    // From min(nodeLevel, maxLayer) down to 0, find neighbors and connect
    for (let layer = Math.min(nodeLevel, this.maxLayer); layer >= 0; layer--) {
      const ef = this.efConstruction;
      const candidates = this.searchLayer(node.vector, currentEntryPoint, layer, ef);

      // Select M nearest neighbors
      const M = layer === 0 ? this.M * 2 : this.M;
      const neighbors = this.selectNeighbors(node.vector, candidates, M);

      // Connect node to neighbors
      for (const neighborIdx of neighbors) {
        node.connections[layer].push(neighborIdx);
        const neighbor = this.nodes[neighborIdx];
        if (layer < neighbor.connections.length) {
          neighbor.connections[layer].push(nodeIndex);

          // Prune neighbor's connections if exceeding M
          const maxConn = layer === 0 ? this.M * 2 : this.M;
          if (neighbor.connections[layer].length > maxConn) {
            const pruned = this.selectNeighbors(neighbor.vector, neighbor.connections[layer], maxConn);
            neighbor.connections[layer] = pruned;
          }
        }
      }

      currentEntryPoint = neighbors.length > 0 ? neighbors[0] : currentEntryPoint;
    }

    // Update entry point if new node has higher level
    if (nodeLevel > this.maxLayer) {
      this.maxLayer = nodeLevel;
      this.entryPoint = nodeIndex;
    }
  }

  private greedySearchLayer(query: number[], entryPoint: number, layer: number, _ef: number): number {
    let current = entryPoint;
    let currentDist = cosineSimilarity(query, this.nodes[current].vector);
    let improved = true;

    while (improved) {
      improved = false;
      const node = this.nodes[current];
      if (layer < node.connections.length) {
        for (const neighborIdx of node.connections[layer]) {
          const neighbor = this.nodes[neighborIdx];
          if (neighbor.id === '__deleted__') continue;
          const dist = cosineSimilarity(query, neighbor.vector);
          if (dist > currentDist) {
            current = neighborIdx;
            currentDist = dist;
            improved = true;
          }
        }
      }
    }

    return current;
  }

  private searchLayer(query: number[], entryPoint: number, layer: number, ef: number): number[] {
    const visited = new Set<number>([entryPoint]);
    const candidates: Array<{ index: number; dist: number }> = [
      { index: entryPoint, dist: cosineSimilarity(query, this.nodes[entryPoint].vector) },
    ];
    const results: Array<{ index: number; dist: number }> = [...candidates];

    while (candidates.length > 0) {
      // Get closest candidate
      candidates.sort((a, b) => b.dist - a.dist);
      const closest = candidates.shift()!;

      // Get worst result
      results.sort((a, b) => b.dist - a.dist);
      const worst = results[results.length - 1];

      if (closest.dist < worst.dist && results.length >= ef) {
        break;
      }

      // Explore neighbors
      const node = this.nodes[closest.index];
      if (layer < node.connections.length) {
        for (const neighborIdx of node.connections[layer]) {
          if (visited.has(neighborIdx)) continue;
          visited.add(neighborIdx);

          const neighbor = this.nodes[neighborIdx];
          if (neighbor.id === '__deleted__') continue;

          const dist = cosineSimilarity(query, neighbor.vector);

          results.sort((a, b) => b.dist - a.dist);
          const worstResult = results[results.length - 1];

          if (dist > worstResult.dist || results.length < ef) {
            candidates.push({ index: neighborIdx, dist });
            results.push({ index: neighborIdx, dist });

            // Trim results
            if (results.length > ef) {
              results.sort((a, b) => b.dist - a.dist);
              results.length = ef;
            }
          }
        }
      }
    }

    return results.map((r) => r.index);
  }

  /**
   * selectNeighbors — 论文 Algorithm 4（Malkov & Yashunin 2016）多样性启发式
   * （extendCandidates=false，keepPrunedConnections=true）。
   * v5.1 检索质量革命：替代简单 top-M 截断——截断会系统性驱逐簇间桥接边，
   * 聚类数据上图碎片化导致召回崩塌。本实现用 cosine similarity（越大越近），
   * "被覆盖"判定：候选到某已选点的相似度高于其到 query 的相似度。
   */
  private selectNeighbors(queryVector: number[], candidates: number[], M: number): number[] {
    const scored = candidates.map((idx) => ({
      index: idx,
      dist: cosineSimilarity(queryVector, this.nodes[idx].vector),
    }));
    scored.sort((a, b) => b.dist - a.dist);

    const selected: Array<{ index: number; vec: number[] }> = [];
    const pruned: number[] = [];
    for (const c of scored) {
      if (selected.length >= M) break;
      const cv = this.nodes[c.index].vector;
      let dominated = false;
      for (const s of selected) {
        if (cosineSimilarity(cv, s.vec) > c.dist) {
          dominated = true;
          break;
        }
      }
      if (!dominated) selected.push({ index: c.index, vec: cv });
      else pruned.push(c.index);
    }
    // keepPrunedConnections：不足 M 时用被裁剪的最近点补齐
    for (const idx of pruned) {
      if (selected.length >= M) break;
      selected.push({ index: idx, vec: this.nodes[idx].vector });
    }
    return selected.map((s) => s.index);
  }

  private bruteForceSearch(query: number[], topK: number): VectorSearchResult[] {
    const results: VectorSearchResult[] = [];
    for (const [id, index] of this.idToIndex) {
      const node = this.nodes[index];
      const score = cosineSimilarity(query, node.vector);
      results.push({ id, score, metadata: node.metadata });
    }
    results.sort((a, b) => b.score - a.score);
    return results.slice(0, topK);
  }
}

function cosineSimilarity(a: number[], b: number[]): number {
  if (a.length !== b.length || a.length === 0) return 0;
  let dot = 0, normA = 0, normB = 0;
  for (let i = 0; i < a.length; i++) {
    dot += a[i] * b[i];
    normA += a[i] * a[i];
    normB += b[i] * b[i];
  }
  if (normA === 0 || normB === 0) return 0;
  return dot / (Math.sqrt(normA) * Math.sqrt(normB));
}
