/* Small, component-local text history for controlled textareas. Vue updates
   the value after every input, so relying on the browser's native undo stack
   loses history when a tab re-renders. Keep both content and selection here. */
import { nextTick, ref, watch } from "vue";

function snapshot(text, el) {
  const at = el?.selectionStart ?? text.length;
  return { text, start: at, end: el?.selectionEnd ?? at };
}

export function useTextHistory(text, write) {
  const current = ref(snapshot(text.value));
  const undo = ref([]);
  const redo = ref([]);

  // A tab switch, a saved draft, or data received from the server starts a
  // new history. Changes made by this helper set current first, so they do
  // not reset the stack on the next reactive update.
  watch(text, (value) => {
    if (value !== current.value.text) {
      current.value = snapshot(value);
      undo.value = [];
      redo.value = [];
    }
  });

  function input(e) {
    const next = snapshot(e.target.value, e.target);
    if (next.text === current.value.text) return;
    undo.value.push(current.value);
    current.value = next;
    redo.value = [];
  }

  async function apply(next, el) {
    current.value = next;
    write(next.text);
    await nextTick();
    el?.setSelectionRange(next.start, next.end);
  }

  function keydown(e) {
    if ((!e.ctrlKey && !e.metaKey) || e.altKey) return false;
    const backwards = e.code === "KeyZ" && !e.shiftKey;
    const forwards = e.code === "KeyY" || (e.code === "KeyZ" && e.shiftKey);
    if (!backwards && !forwards) return false;
    e.preventDefault();
    const from = backwards ? undo.value : redo.value;
    const to = backwards ? redo.value : undo.value;
    const next = from.pop();
    if (!next) return true;
    to.push(current.value);
    apply(next, e.target);
    return true;
  }

  return { input, keydown };
}
