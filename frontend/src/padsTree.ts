export interface PadNode {
  id: string;
  name: string;
  pads?: number[];
  children?: PadNode[];
}

export function collectPads(node: PadNode): number[] {
  const out = [...(node.pads || [])];
  for (const child of node.children || []) {
    out.push(...collectPads(child));
  }
  return [...new Set(out)];
}

export function collectAllPads(nodes: PadNode[]): number[] {
  return [...new Set(nodes.flatMap(collectPads))];
}

export function nodeState(node: PadNode, selected: Set<number>): "all" | "some" | "none" {
  const pads = collectPads(node);
  if (pads.length === 0) return "none";
  let hit = 0;
  for (const id of pads) {
    if (selected.has(id)) hit += 1;
  }
  if (hit === 0) return "none";
  if (hit === pads.length) return "all";
  return "some";
}

export function toggleNode(node: PadNode, selected: number[]): number[] {
  const pads = collectPads(node);
  const current = new Set(selected);
  const allOn = pads.length > 0 && pads.every((id) => current.has(id));
  for (const id of pads) {
    if (allOn) current.delete(id);
    else current.add(id);
  }
  return [...current];
}

export function padLabels(nodes: PadNode[], selected: number[]): string[] {
  const wanted = new Set(selected);
  const labels: string[] = [];
  const walk = (node: PadNode, trail: string[]) => {
    const next = node.name ? [...trail, node.name] : trail;
    const own = (node.pads || []).filter((id) => wanted.has(id));
    if (own.length > 0 && (!node.children || node.children.length === 0)) {
      labels.push(next.join(" / "));
    }
    for (const child of node.children || []) walk(child, next);
  };
  for (const node of nodes) walk(node, []);
  return labels;
}
