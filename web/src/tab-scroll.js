import { nextTick } from "vue";

// Weak keys give scroll memory exactly the lifetime of an open tab, without
// serializing it into the saved workspace. File modes have separate positions.
const positions = new WeakMap();
const panes = new WeakMap();

export function tabScroll(tab, view = "") {
  return tab && positions.get(tab)?.get(view);
}

export function rememberTabScroll(tab, view, position) {
  let views = positions.get(tab);
  if (!views) positions.set(tab, (views = new Map()));
  views.set(view, position);
}

function save(el, state) {
  if (!state.tab || state.pending) return;
  rememberTabScroll(state.tab, state.view, { top: el.scrollTop, left: el.scrollLeft });
}

function select(state, [tab, view = "", bottom = false, ready = true]) {
  state.ready = ready;
  state.tab = tab;
  state.view = view;
  state.pending = tabScroll(tab, view) || { top: bottom ? Infinity : 0, left: 0 };
}

function restore(el, state) {
  const position = state.pending;
  if (!position || !state.ready) return;
  el.scrollTop = position.top === Infinity ? el.scrollHeight : position.top;
  el.scrollLeft = position.left;
  state.pending = null;
}

export const vTabScroll = {
  mounted(el, { value }) {
    const state = {};
    select(state, value);
    panes.set(el, state);
    nextTick(() => restore(el, state));
  },
  beforeUpdate(el, { value }) {
    const state = panes.get(el);
    const [tab, view = "", , ready = true] = value;
    state.ready = ready;
    if (state.tab === tab && state.view === view) return;
    save(el, state);
    select(state, value);
  },
  updated(el) {
    const state = panes.get(el);
    nextTick(() => restore(el, state));
  },
  beforeUnmount(el) {
    save(el, panes.get(el));
    panes.delete(el);
  },
};
