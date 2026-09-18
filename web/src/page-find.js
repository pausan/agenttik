// Literal, case-insensitive matches, preserving UTF-16 offsets for DOM ranges.
export function findMatches(text, query) {
  if (!query) return [];
  const escaped = query.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return Array.from(text.matchAll(new RegExp(escaped, "giu")), (m) => ({ start: m.index, end: m.index + m[0].length }));
}

export function pageRanges(root, query) {
  if (!root || !query) return [];
  const doc = root.ownerDocument;
  const walker = doc.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const groups = [];
  let group;
  for (let node; (node = walker.nextNode());) {
    const el = node.parentElement;
    if (el.closest('textarea, input, script, style, [data-find-ignore], [hidden]')) continue;
    if (!el.getClientRects().length || getComputedStyle(el).visibility === "hidden") continue;
    const block = el.closest('p, pre, li, h1, h2, h3, h4, h5, h6, td, th, button, label, div');
    if (!group || group.block !== block) {
      group = { block, text: "", nodes: [] };
      groups.push(group);
    }
    group.nodes.push({ node, start: group.text.length });
    group.text += node.data;
  }
  return groups.flatMap(({ text, nodes }) => findMatches(text, query).map(({ start, end }) => {
    const first = nodes.find(({ node, start: at }) => at + node.length > start);
    const last = nodes.find(({ node, start: at }) => at + node.length >= end);
    const range = doc.createRange();
    range.setStart(first.node, start - first.start);
    range.setEnd(last.node, end - last.start);
    return range;
  }));
}
