const fields = ["turns", "input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "cost_usd", "cost_turns", "inferred_turns"];

export function analyticsTotals(rows) {
  const total = Object.fromEntries(fields.map((field) => [field, 0]));
  for (const row of rows) for (const field of fields) total[field] += row[field] || 0;
  return total;
}

export function analyticsGroups(rows, grouping = "provider", metric = "cost_usd") {
  const groups = new Map();
  for (const row of rows) {
    const key = JSON.stringify(grouping === "model" ? [row.provider, row.model || ""] : grouping === "model_effort" ? [row.provider, row.model || "", row.effort || ""] : grouping === "subscription" ? [row.provider, row.account_id] : [row.provider]);
    if (!groups.has(key)) groups.set(key, { key, model: row.model || "", effort: row.effort || "", provider: row.provider, account_id: grouping === "subscription" ? row.account_id : null, rows: [], projects: new Map() });
    const group = groups.get(key);
    group.rows.push(row);
    if (!group.projects.has(row.project_id)) group.projects.set(row.project_id, { id: row.project_id, name: row.project_name, rows: [] });
    group.projects.get(row.project_id).rows.push(row);
  }
  const compare = (a, b) => b[metric] - a[metric] || b.input_tokens - a.input_tokens || String(a.key ?? a.id ?? a.session_id).localeCompare(String(b.key ?? b.id ?? b.session_id));
  return [...groups.values()].map((group) => ({
    ...group,
    ...analyticsTotals(group.rows),
    projects: [...group.projects.values()].map((project) => ({
      ...project,
      ...analyticsTotals(project.rows),
      task_count: new Set(project.rows.map((row) => row.session_id)).size,
      rows: [...project.rows.reduce((tasks, row) => {
        const key = JSON.stringify([row.session_id, row.provider, row.account_id]);
        const previous = tasks.get(key);
        tasks.set(key, { ...row, ...analyticsTotals(previous ? [previous, row] : [row]) });
        return tasks;
      }, new Map()).values()].sort(compare),
    })).sort(compare),
  })).sort(compare);
}

export function analyticsShare(value, total) {
  return total > 0 ? `${(100 * value / total).toFixed(1)}%` : "—";
}
