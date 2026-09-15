// Each lane tracks an ancestor still to be visited in topological order.
// Filtered-out commits are traversed so edges still reach visible ancestors.
export function commitGraph(commits, visible = new Set(commits.map((c) => c.hash))) {
  const byHash = new Map(commits.map((c) => [c.hash, c]));
  let lanes = [];
  let nextColor = 0;
  let width = 1;
  const rows = new Map();
  for (const commit of commits) {
    if (!visible.has(commit.hash)) continue;
    const parents = [];
    const pending = [...(commit.parents || [])];
    const seen = new Set();
    for (let at = 0; at < pending.length; at++) {
      const hash = pending[at];
      if (seen.has(hash)) continue;
      seen.add(hash);
      if (visible.has(hash) || !byHash.has(hash)) parents.push(hash);
      else pending.push(...(byHash.get(hash).parents || []));
    }
    let column = lanes.findIndex((lane) => lane.hash === commit.hash);
    const incoming = [...lanes];
    if (column < 0) {
      column = lanes.length;
      lanes.push({ hash: commit.hash, color: nextColor++ });
    }
    const node = lanes[column];
    const after = lanes.filter((lane) => lane.hash !== commit.hash);
    let insert = column;
    for (const [i, hash] of parents.entries()) {
      if (!after.some((lane) => lane.hash === hash)) {
        after.splice(insert++, 0, { hash, color: i === 0 ? node.color : nextColor++ });
      }
    }
    const edges = incoming.map((lane, from) => ({
      from, to: lane.hash === commit.hash ? column : after.findIndex((l) => l.hash === lane.hash),
      color: lane.color, kind: lane.hash === commit.hash ? "in" : "through",
    }));
    for (const hash of parents) {
      const to = after.findIndex((lane) => lane.hash === hash);
      edges.push({ from: column, to, color: after[to].color, kind: "out" });
    }
    width = Math.max(width, lanes.length, after.length);
    rows.set(commit.hash, { column, color: node.color, edges });
    lanes = after;
  }
  return { rows, width: width * 12 + 8 };
}

export const graphColor = (color) => ["var(--ui-primary)", "var(--ui-secondary)", "#ec4899", "#f59e0b", "#14b8a6"][color % 5];
export const graphX = (column) => 10 + column * 12;
export function graphPath(edge) {
  const x = graphX(edge.from), to = graphX(edge.to);
  if (edge.kind === "in") return `M ${x} 0 L ${to} 16`;
  const start = edge.kind === "out" ? 16 : 0;
  return `M ${x} ${start} C ${x} 24 ${to} 24 ${to} 32`;
}
