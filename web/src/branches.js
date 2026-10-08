import { fuzzy } from "./fuzzy.js";

// Keep local branches first, even when a remote branch is a closer match.
export function branchOptions(local = [], remote = [], search = "") {
  const groups = new Map();
  for (const item of remote) {
    if (!groups.has(item.remote)) groups.set(item.remote, []);
    groups.get(item.remote).push({ label: `${item.remote}/${item.branch}`, value: `refs/remotes/${item.remote}/${item.branch}`, branch: item.branch, remote: item.remote });
  }
  const query = search.trim();
  return [["Local", local.map(branch => ({ label: branch, value: `refs/heads/${branch}`, branch }))], ...groups].flatMap(([label, items]) => {
    const matches = query ? items.map(item => ({ item, match: fuzzy(item.label, query) }))
      .filter(row => row.match).sort((a, b) => a.match.score - b.match.score).map(row => row.item) : items;
    return matches.length ? [[{ type: "label", label }, ...matches]] : [];
  });
}
