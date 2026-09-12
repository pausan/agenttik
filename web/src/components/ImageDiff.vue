<script setup>
/* An image's diff: the two pictures, side by side.

   There is no unified shape of a picture and nothing to type into one, so
   this carries neither the toggle the text diff has nor an editor behind it.
   The left is the baseline — HEAD for the working tree, the commit's parent
   for a file opened from a commit — and the right is what it is now. Either
   side can be absent, which is what an added or a deleted image looks like.

   Both sides are one request each, straight from the raw endpoint into an
   <img>: nothing about a picture is worth routing through JSON. */
import { computed } from "vue";

import { rawURL } from "../store";
import ImageFrame from "./ImageFrame.vue";

const props = defineProps({ tab: { type: Object, required: true } });
const emit = defineEmits(["dimensions"]);

const before = computed(() => (props.tab.commit ? props.tab.commit + "^" : "HEAD"));
const after = computed(() => props.tab.commit || "");

/* Which sides exist is already in the diff git printed: it heads a new file
   with "new file mode" and a removed one with "deleted file mode". Reading it
   saves asking for bytes that can only come back as a not found. */
const added = computed(() => /^new file mode /m.test(props.tab.diff || ""));
const deleted = computed(() => /^deleted file mode /m.test(props.tab.diff || ""));
</script>

<template>
  <div class="flex h-full min-h-0 gap-4 px-5 py-4">
    <div class="flex min-h-0 min-w-0 flex-1 border-r border-default pr-4">
      <p v-if="added" class="flex-1 self-center text-center text-dimmed">Added — nothing before it.</p>
      <ImageFrame
        v-else
        :src="rawURL(tab, before)"
        :label="tab.commit ? 'before' : 'HEAD'"
        missing="No image on this side."
        @dimensions="deleted && emit('dimensions', $event)"
      />
    </div>
    <div class="flex min-h-0 min-w-0 flex-1">
      <p v-if="deleted" class="flex-1 self-center text-center text-dimmed">Deleted.</p>
      <ImageFrame
        v-else
        :src="rawURL(tab, after)"
        :label="tab.commit ? 'after' : 'working tree'"
        missing="No image on this side."
        @dimensions="emit('dimensions', $event)"
      />
    </div>
  </div>
</template>
