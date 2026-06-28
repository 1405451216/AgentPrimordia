// ===== Workflow Visualization =====

export interface VisualizeConfig {
  direction: 'TD' | 'LR';
  highlightPath: string[];
  failedNodes: string[];
  showLabels: boolean;
}

export function defaultVisualizeConfig(): VisualizeConfig {
  return { direction: 'TD', highlightPath: [], failedNodes: [], showLabels: true };
}

export interface VizNode {
  id: string;
  name: string;
  type: string;
}

export interface VizTransition {
  from: string;
  to: string;
  condition?: string;
}

export interface VizWorkflow {
  nodes: Map<string, VizNode>;
  transitions: VizTransition[];
  startNodeID: string;
}

// ===== Mermaid Generator =====

export class MermaidGenerator {
  generate(workflow: VizWorkflow, config?: VisualizeConfig): string {
    const cfg = config ?? defaultVisualizeConfig();
    const lines: string[] = [];

    lines.push(`graph ${cfg.direction}`);

    // Nodes
    for (const [id, node] of workflow.nodes) {
      const shape = this.nodeShape(node.type);
      const isFailed = cfg.failedNodes.includes(id);
      const isHighlighted = cfg.highlightPath.includes(id);
      let style = '';
      if (isFailed) style = ':::failed';
      else if (isHighlighted) style = ':::highlight';

      lines.push(`  ${id}${shape.start}${node.name}${shape.end}${style}`);
    }

    // Edges
    for (const trans of workflow.transitions) {
      const label = cfg.showLabels && trans.condition ? `|${trans.condition}|` : '';
      lines.push(`  ${trans.from} -->${label} ${trans.to}`);
    }

    // Styles
    lines.push('  classDef failed fill:#f88,stroke:#c00');
    lines.push('  classDef highlight fill:#8f8,stroke:#0a0');

    return lines.join('\n');
  }

  private nodeShape(type: string): { start: string; end: string } {
    switch (type) {
      case 'start': return { start: '([', end: '])' };
      case 'end': return { start: '[[', end: ']]' };
      case 'condition': return { start: '{', end: '}' };
      case 'parallel': return { start: '[/', end: '/]' };
      case 'loop': return { start: '((', end: '))' };
      default: return { start: '[', end: ']' };
    }
  }
}

// ===== DOT Generator =====

export class DOTGenerator {
  generate(workflow: VizWorkflow, config?: VisualizeConfig): string {
    const cfg = config ?? defaultVisualizeConfig();
    const lines: string[] = [];

    lines.push('digraph workflow {');
    lines.push(`  rankdir=${cfg.direction === 'TD' ? 'TB' : 'LR'};`);

    // Nodes
    for (const [id, node] of workflow.nodes) {
      const isFailed = cfg.failedNodes.includes(id);
      const isHighlighted = cfg.highlightPath.includes(id);
      const shape = this.dotNodeShape(node.type);
      let color = 'white';
      if (isFailed) color = 'red';
      else if (isHighlighted) color = 'lightgreen';

      lines.push(`  ${id} [label="${node.name}", shape=${shape}, style=filled, fillcolor=${color}];`);
    }

    // Edges
    for (const trans of workflow.transitions) {
      const label = cfg.showLabels && trans.condition ? ` [label="${trans.condition}"]` : '';
      lines.push(`  ${trans.from} -> ${trans.to}${label};`);
    }

    lines.push('}');
    return lines.join('\n');
  }

  private dotNodeShape(type: string): string {
    switch (type) {
      case 'start':
      case 'end': return 'doublecircle';
      case 'condition': return 'diamond';
      case 'parallel': return 'hexagon';
      case 'loop': return 'ellipse';
      default: return 'box';
    }
  }
}

// ===== Visualizer =====

export class WorkflowVisualizer {
  private mermaid: MermaidGenerator;
  private dot: DOTGenerator;

  constructor() {
    this.mermaid = new MermaidGenerator();
    this.dot = new DOTGenerator();
  }

  toMermaid(workflow: VizWorkflow, config?: VisualizeConfig): string {
    return this.mermaid.generate(workflow, config);
  }

  toDOT(workflow: VizWorkflow, config?: VisualizeConfig): string {
    return this.dot.generate(workflow, config);
  }

  toJSON(workflow: VizWorkflow): string {
    return JSON.stringify({
      nodes: Array.from(workflow.nodes.entries()).map(([id, node]) => ({ id, name: node.name, type: node.type })),
      transitions: workflow.transitions,
      startNodeID: workflow.startNodeID,
    }, null, 2);
  }

  toHTML(workflow: VizWorkflow, config?: VisualizeConfig): string {
    const mermaid = this.toMermaid(workflow, config);
    return `<!DOCTYPE html>
<html>
<head>
  <title>Workflow Visualization</title>
  <script src="https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js"></script>
  <style>
    body { font-family: sans-serif; margin: 20px; }
    .mermaid { text-align: center; }
  </style>
</head>
<body>
  <h1>Workflow Visualization</h1>
  <div class="mermaid">
${mermaid}
  </div>
  <script>mermaid.initialize({ startOnLoad: true });</script>
</body>
</html>`;
  }
}

// ===== Visual Editor =====

export interface EditorAction {
  type: 'add_node' | 'remove_node' | 'add_edge' | 'remove_edge' | 'update_node' | 'move_node';
  data: Record<string, unknown>;
}

export interface EditorState {
  workflow: VizWorkflow;
  history: EditorAction[];
  future: EditorAction[];
}

export class VisualEditor {
  private state: EditorState;

  constructor(workflow: VizWorkflow) {
    this.state = { workflow, history: [], future: [] };
  }

  addNode(node: VizNode): void {
    this.state.workflow.nodes.set(node.id, node);
    this.state.history.push({ type: 'add_node', data: { node } });
    this.state.future = [];
  }

  removeNode(id: string): void {
    const node = this.state.workflow.nodes.get(id);
    if (node) {
      this.state.workflow.nodes.delete(id);
      this.state.workflow.transitions = this.state.workflow.transitions.filter(
        t => t.from !== id && t.to !== id
      );
      this.state.history.push({ type: 'remove_node', data: { id, node } });
      this.state.future = [];
    }
  }

  addEdge(transition: VizTransition): void {
    this.state.workflow.transitions.push(transition);
    this.state.history.push({ type: 'add_edge', data: { transition } });
    this.state.future = [];
  }

  removeEdge(from: string, to: string): void {
    const idx = this.state.workflow.transitions.findIndex(t => t.from === from && t.to === to);
    if (idx >= 0) {
      const [removed] = this.state.workflow.transitions.splice(idx, 1);
      this.state.history.push({ type: 'remove_edge', data: { transition: removed } });
      this.state.future = [];
    }
  }

  updateNode(id: string, updates: Partial<VizNode>): void {
    const node = this.state.workflow.nodes.get(id);
    if (node) {
      const oldNode = { ...node };
      Object.assign(node, updates);
      this.state.history.push({ type: 'update_node', data: { id, oldNode, updates } });
      this.state.future = [];
    }
  }

  undo(): boolean {
    const action = this.state.history.pop();
    if (!action) return false;
    this.state.future.push(action);
    this.applyReverse(action);
    return true;
  }

  redo(): boolean {
    const action = this.state.future.pop();
    if (!action) return false;
    this.state.history.push(action);
    this.applyForward(action);
    return true;
  }

  getWorkflow(): VizWorkflow {
    return this.state.workflow;
  }

  private applyReverse(action: EditorAction): void {
    switch (action.type) {
      case 'add_node':
        this.state.workflow.nodes.delete((action.data.node as VizNode).id);
        break;
      case 'remove_node':
        this.state.workflow.nodes.set(action.data.id as string, action.data.node as VizNode);
        break;
      case 'add_edge':
        this.state.workflow.transitions = this.state.workflow.transitions.filter(
          t => t !== action.data.transition
        );
        break;
      case 'remove_edge':
        this.state.workflow.transitions.push(action.data.transition as VizTransition);
        break;
      case 'update_node':
        Object.assign(
          this.state.workflow.nodes.get(action.data.id as string)!,
          action.data.oldNode
        );
        break;
    }
  }

  private applyForward(action: EditorAction): void {
    switch (action.type) {
      case 'add_node':
        this.state.workflow.nodes.set(
          (action.data.node as VizNode).id,
          action.data.node as VizNode
        );
        break;
      case 'remove_node':
        this.state.workflow.nodes.delete(action.data.id as string);
        break;
      case 'add_edge':
        this.state.workflow.transitions.push(action.data.transition as VizTransition);
        break;
      case 'remove_edge':
        this.state.workflow.transitions = this.state.workflow.transitions.filter(
          t => t !== action.data.transition
        );
        break;
      case 'update_node':
        Object.assign(
          this.state.workflow.nodes.get(action.data.id as string)!,
          action.data.updates
        );
        break;
    }
  }
}
