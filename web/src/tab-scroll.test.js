import assert from "node:assert/strict";
import test from "node:test";
import { nextTick } from "vue";
import { tabScroll, vTabScroll } from "./tab-scroll.js";

test("scroll positions belong to each open tab and file mode, including zero", async () => {
  const first = {}, second = {};
  const el = { scrollTop: 0, scrollLeft: 0 };
  vTabScroll.mounted(el, { value: [first, "edit"] });
  await nextTick();
  el.scrollTop = 320;
  el.scrollLeft = 45;
  const switchTo = async (value) => {
    vTabScroll.beforeUpdate(el, { value });
    vTabScroll.updated(el);
    await nextTick();
  };
  await switchTo([second, "edit"]);
  assert.equal(el.scrollTop, 0);
  el.scrollTop = 150;
  await switchTo([first, "edit"]);
  assert.equal(el.scrollTop, 320);
  assert.equal(el.scrollLeft, 45);
  await switchTo([first, "preview"]);
  assert.equal(el.scrollTop, 0);
  await switchTo([first, "edit"]);
  assert.equal(el.scrollTop, 320);
  el.scrollTop = 0;
  vTabScroll.beforeUnmount(el);
  assert.equal(tabScroll(first, "edit").top, 0);
  assert.equal(tabScroll(second, "edit").top, 150);
  assert.equal(tabScroll({}, "edit"), undefined);
});

test("restoration waits for loaded content and survives unmounting", async () => {
  const tab = {};
  const old = { scrollTop: 0, scrollLeft: 0 };
  vTabScroll.mounted(old, { value: [tab] });
  await nextTick();
  old.scrollTop = 700;
  vTabScroll.beforeUnmount(old);
  let height = 0, top = 0;
  const el = {
    scrollLeft: 0,
    get scrollTop() { return top; },
    set scrollTop(value) { top = Math.min(height, value); },
  };
  vTabScroll.mounted(el, { value: [tab, "", false, false] });
  await nextTick();
  assert.equal(el.scrollTop, 0);
  height = 1000;
  vTabScroll.beforeUpdate(el, { value: [tab, "", false, true] });
  vTabScroll.updated(el);
  await nextTick();
  assert.equal(el.scrollTop, 700);
  vTabScroll.beforeUnmount(el);
});
