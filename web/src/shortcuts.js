/* Every chord the app answers to, and the two functions that compare one to a
   keyboard event.

   A chord is written the way it reads — "Ctrl+Shift+T" — but matched on the
   physical key: on some layouts Alt and a digit produce a different character,
   so e.code is what the comparison uses and the letter in the chord is turned
   back into a code.

   Actions marked `fixed` are families ("Alt+1…9") or the editing conventions
   the textareas answer to themselves. They are listed so the dialog is the
   whole truth, and they are not rebindable. */

export const ACTIONS = [
  { id: "task.new", group: "Tasks", what: "New task in the current project", keys: ["Ctrl+N", "Ctrl+T"] },
  { id: "task.rename", group: "Tasks", what: "Rename the current task or scheduled job", keys: ["F2"] },
  { id: "tab.close", group: "Conversations", what: "Close the current tab", keys: ["Ctrl+W"] },
  { id: "tab.reopen", group: "Conversations", what: "Restore the last closed tab in this project", keys: ["Ctrl+Shift+T"] },

  { id: "tab.prev", group: "Tabs", what: "Previous tab", keys: ["Ctrl+Shift+Tab"] },
  { id: "tab.next", group: "Tabs", what: "Next tab", keys: ["Ctrl+Tab"] },
  { id: "task.prev", group: "Tasks", what: "Previous project or task in the sidebar", keys: ["Ctrl+PageUp"] },
  { id: "task.next", group: "Tasks", what: "Next project or task in the sidebar", keys: ["Ctrl+PageDown"] },
  { id: "task.at", group: "Tasks", what: "Go straight to one of the first nine tasks", keys: ["Alt+1…9"], fixed: true },
  { id: "project.at", group: "Tabs", what: "Switch to one of the first eight projects, again to fold its tasks", keys: ["Alt+A…H"], fixed: true },

  { id: "goto", group: "Panels", what: "Go to anywhere", keys: ["Ctrl+P"] },
  { id: "panel.projects", group: "Panels", what: "Projects", keys: ["Alt+P"] },
  { id: "panel.tree", group: "Panels", what: "Tree, cursor in the filter", keys: ["Alt+T"] },
  { id: "divider.resize", group: "Panels", what: "Resize a selected divider", keys: ["←", "→"], fixed: true },

  { id: "file.save", group: "Files", what: "Save the file in front", keys: ["Ctrl+S"] },
  { id: "file.indent", group: "Files", what: "Indent, with the caret in a file", keys: ["Tab"], fixed: true },
  { id: "edit.undo", group: "Files", what: "Undo / redo file and draft edits", keys: ["Ctrl+Z", "Ctrl+Y"], fixed: true },

  { id: "prompt.send", group: "Prompt", what: "Send", keys: ["Ctrl+Enter"] },
  { id: "prompt.enqueue", group: "Prompt", what: "Enqueue", keys: ["Enter"] },
  { id: "prompt.newline", group: "Prompt", what: "New line", keys: ["Shift+Enter"], fixed: true },
];

export const GROUPS = [...new Set(ACTIONS.map((a) => a.group))];

export function actionOf(id) {
  return ACTIONS.find((a) => a.id === id);
}

/* Parsing is memoised because the global keydown asks about every binding on
   every keystroke, and the answer for a given chord never changes. */
const parsed = new Map();

function parse(chord) {
  let c = parsed.get(chord);
  if (c) return c;
  const parts = chord.split("+").map((p) => p.trim()).filter(Boolean);
  const key = parts.pop() || "";
  const has = (m) => parts.some((p) => p.toLowerCase() === m);
  c = { ctrl: has("ctrl"), alt: has("alt"), shift: has("shift"), meta: has("meta"), code: codeOf(key) };
  parsed.set(chord, c);
  return c;
}

function codeOf(key) {
  if (/^[a-z]$/i.test(key)) return "Key" + key.toUpperCase();
  if (/^[0-9]$/.test(key)) return "Digit" + key;
  return key;
}

/* matches is exact on the modifiers: Ctrl+T and Ctrl+Shift+T are two chords,
   so the order the handlers test them in does not matter. */
export function matches(e, chord) {
  const c = parse(chord);
  if (e.ctrlKey !== c.ctrl || e.altKey !== c.alt || e.shiftKey !== c.shift || e.metaKey !== c.meta) {
    return false;
  }
  if (e.code === c.code) return true;
  // The numeric keypad reaches the tab digits too.
  return c.code.startsWith("Digit") && e.code === "Numpad" + c.code.slice(5);
}

const MODIFIER_CODES = /^(Control|Alt|Shift|Meta|OS)(Left|Right)?$/;

/* keyOf names the key a code stands for, the way a chord spells it. Empty
   while only modifiers are down, which is what lets the recorder show them
   before the chord is complete. */
function keyOf(code) {
  if (MODIFIER_CODES.test(code)) return "";
  const letter = /^Key([A-Z])$/.exec(code);
  if (letter) return letter[1];
  const digit = /^(?:Digit|Numpad)([0-9])$/.exec(code);
  if (digit) return digit[1];
  return code;
}

export function modifiersOf(e) {
  const parts = [];
  if (e.ctrlKey) parts.push("Ctrl");
  if (e.altKey) parts.push("Alt");
  if (e.shiftKey) parts.push("Shift");
  if (e.metaKey) parts.push("Meta");
  return parts;
}

/* chordOf writes an event back out as a chord, for the recorder. */
export function chordOf(e) {
  const key = keyOf(e.code);
  if (!key) return "";
  return [...modifiersOf(e), key].join("+");
}
