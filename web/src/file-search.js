import { fuzzy } from "./fuzzy.js";

// Search every path, but render only the best 100 results.
export function searchFiles(paths, query, limit = 100) {
  const needle = query.trim();
  if (!needle) return paths.slice(0, limit).map((path) => ({ path, hits: null }));
  const matches = [];
  for (const path of paths) {
    const match = fuzzy(path, needle);
    if (match) matches.push({ path, ...match });
  }
  matches.sort((a, b) => a.score - b.score || a.path.length - b.path.length || a.path.localeCompare(b.path));
  return matches.slice(0, limit);
}
