<script setup>
/* Every shortcut the app answers to, in one dialog.

   The list is written out rather than derived from the handlers: the chords
   live in App.vue's keydown, in the prompt box and on the dividers, and a list
   that reads well is worth more than one generated from three places. A row's
   keys are one string — "Ctrl+N / Ctrl+T" — which keeps the source legible and
   the render two splits: alternatives on the slash, keys on the plus. */
const open = defineModel("open", { type: Boolean, default: false });

const groups = [
  {
    label: "Conversations",
    rows: [
      { keys: "Ctrl+N / Ctrl+T", what: "New session in the current project" },
      { keys: "Ctrl+W", what: "Close the current conversation" },
      { keys: "Ctrl+Shift+T", what: "Reopen the last closed session" },
    ],
  },
  {
    label: "Tabs",
    rows: [
      { keys: "Ctrl+PageUp", what: "Previous tab" },
      { keys: "Ctrl+PageDown", what: "Next tab" },
      { keys: "Alt+1…9", what: "Go straight to one of the first nine" },
      { keys: "Alt+A…H", what: "Switch to one of the first eight projects" },
    ],
  },
  {
    label: "Panels",
    rows: [
      { keys: "Ctrl+P", what: "Go to anywhere" },
      { keys: "Alt+P", what: "Projects" },
      { keys: "Alt+S", what: "Sessions, cursor in the filter" },
      { keys: "Alt+T", what: "Tree, cursor in the filter" },
      { keys: "← / →", what: "Resize a selected divider" },
    ],
  },
  {
    label: "Files",
    rows: [
      { keys: "Ctrl+S", what: "Save the file in front" },
      { keys: "Tab", what: "Indent, with the caret in a file" },
    ],
  },
  {
    label: "Prompt",
    rows: [
      { keys: "Enter", what: "Send" },
      { keys: "Shift+Enter", what: "New line" },
    ],
  },
];

function chords(keys) {
  return keys.split("/").map((chord) => chord.trim().split("+"));
}
</script>

<template>
  <UModal v-model:open="open" title="Keyboard shortcuts" :ui="{ content: 'max-w-lg' }">
    <template #body>
      <div class="max-h-[60vh] overflow-auto">
        <section
          v-for="g in groups"
          :key="g.label"
          class="not-first:mt-4 not-first:border-t not-first:border-default not-first:pt-3.5"
        >
          <h3 class="mb-1 font-semibold text-highlighted">{{ g.label }}</h3>
          <div v-for="r in g.rows" :key="r.keys" class="flex items-center gap-3 py-1">
            <span class="flex w-44 shrink-0 items-center gap-1">
              <template v-for="(chord, c) in chords(r.keys)" :key="c">
                <span v-if="c" class="text-dimmed">or</span>
                <template v-for="(k, i) in chord" :key="i">
                  <span v-if="i" class="text-dimmed">+</span>
                  <UKbd :value="k" />
                </template>
              </template>
            </span>
            <span class="min-w-0 text-muted">{{ r.what }}</span>
          </div>
        </section>
      </div>
    </template>
    <template #footer>
      <div class="flex w-full justify-end">
        <UButton label="Done" @click="open = false" />
      </div>
    </template>
  </UModal>
</template>
