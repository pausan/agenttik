<script setup>
import { computed, reactive, ref, watchEffect } from "vue";

import {
  S,
  accountUsage,
  addAccount,
  configureAccount,
  fail,
  removeAccount,
  setDefaultAccount,
  signInAccount,
  updateAccount,
} from "../../store";
import { fuzzyAny } from "../../fuzzy";
import StatusDot from "../StatusDot.vue";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);

/* A provider the filter names keeps all its subscriptions; otherwise the
   aliases themselves are matched, so "personal" finds the one row. */
const providers = computed(() =>
  S.providers.filter((p) => p.kind !== "api").flatMap((p) => {
    if (fuzzyAny([p.display_name, p.name, "subscriptions", "accounts", "providers"], props.filter) !== null) {
      return [p];
    }
    const accounts = p.accounts.filter(
      (a) => fuzzyAny([a.alias, a.detail || "", a.home || ""], props.filter) !== null,
    );
    return accounts.length ? [{ ...p, accounts }] : [];
  }),
);

watchEffect(() => emit("count", providers.value.length));

/* Adding, editing, removing and signing in are each one row's transient
   state, kept by provider or by account id rather than in the row, so the
   provider list can be re-read without losing what is half-typed. */
const adding = reactive({}); // provider -> { alias, home } | undefined
const editing = reactive({}); // account id -> { alias, home }
const removing = reactive({}); // account id -> { sessions, schedules }
const command = reactive({}); // provider:account -> { command, spawned }
const busy = ref(0);
const connecting = reactive({});
const rowKey = (provider, account) => `${provider}:${account.id}`;
function startConnect(provider, account) {
  connecting[rowKey(provider, account)] = { connection: account.connection || "auto", key: "" };
}
async function saveConnection(provider, account) {
  const id = rowKey(provider, account);
  const form = connecting[id];
  busy.value += 1;
  try {
    await configureAccount(provider, account.id, form.connection, form.key);
    delete connecting[id];
  } catch (e) {
    fail(e);
  } finally {
    busy.value -= 1;
  }
}

function startAdd(provider) {
  adding[provider] = { alias: "", home: "" };
}

async function submitAdd(provider) {
  const form = adding[provider];
  if (!form?.alias.trim()) return;
  busy.value += 1;
  try {
    await addAccount(provider, form.alias.trim(), form.home.trim());
    delete adding[provider];
  } catch (e) {
    fail(e);
  } finally {
    busy.value -= 1;
  }
}

function startEdit(provider, account) {
  editing[`${provider}:${account.id}`] = { alias: account.alias, home: account.home };
}

async function submitEdit(provider, account) {
  const form = editing[`${provider}:${account.id}`];
  if (!form) return;
  busy.value += 1;
  try {
    await updateAccount(provider, account.id, { alias: form.alias.trim(), ...(account.system ? {} : { home: form.home.trim() }) });
    delete editing[`${provider}:${account.id}`];
  } catch (e) {
    fail(e);
  } finally {
    busy.value -= 1;
  }
}

/* Removing asks first, and says what still points at it: a task on a
   subscription that is gone stops rather than moving to another account's
   allowance, so the number is the consequence. */
async function startRemove(provider, account) {
  try {
    removing[account.id] = await accountUsage(provider, account.id);
  } catch (e) {
    fail(e);
  }
}

async function confirmRemove(provider, account) {
  busy.value += 1;
  try {
    await removeAccount(provider, account.id);
    delete removing[account.id];
  } catch (e) {
    fail(e);
  } finally {
    busy.value -= 1;
  }
}

async function choose(provider, id) {
  busy.value += 1;
  try {
    await setDefaultAccount(provider, id);
  } catch (e) {
    fail(e);
  } finally {
    busy.value -= 1;
  }
}

/* Signing in opens the CLI's own login in a terminal on the machine the CLIs
   are installed on. The command is shown either way — there may be no
   terminal to open, or this window may be a browser on another machine. */
async function signIn(provider, account) {
  busy.value += 1;
  try {
    command[rowKey(provider, account)] = await signInAccount(provider, account.id);
  } catch (e) {
    fail(e);
  } finally {
    busy.value -= 1;
  }
}

function copy(text) {
  navigator.clipboard?.writeText(text).catch(() => {});
}
</script>

<template>
  <section v-if="providers.length">
    <div class="mb-0.5 font-semibold text-highlighted">Subscriptions</div>
    <p class="mb-3.5 text-xs text-dimmed">
      One machine can be signed in to more than one account per provider — a company
      subscription and a personal one. Each has its own login folder. The one marked default is what a new
      task starts on, and any task can be swapped to another from its model picker.
    </p>

    <!-- One group per provider, named, so its subscriptions are reachable as
         a set rather than as four rows all called System. -->
    <section
      v-for="p in providers"
      :key="p.name"
      role="group"
      :aria-label="`${p.display_name} subscriptions`"
      class="not-first:mt-4 not-first:border-t not-first:border-default not-first:pt-3.5"
    >
      <div class="mb-1.5 flex items-center gap-2">
        <span class="font-semibold text-highlighted">{{ p.display_name }}</span>
        <UBadge :color="p.available ? 'primary' : 'neutral'" variant="soft" size="sm">
          <StatusDot :status="p.available ? 'ok' : 'idle'" />
          {{ p.available ? "ready" : "unavailable" }}
        </UBadge>
      </div>

      <p v-if="p.cli_requirement" class="mb-2 text-xs text-dimmed">
        {{ p.cli_requirement }}
        <a :href="p.install_url" target="_blank" rel="noopener noreferrer" class="text-primary underline">Installation instructions</a>
        <span> · {{ p.cli_installed ? "Installed" : "Not installed" }}</span>
      </p>

      <div
        v-for="a in p.accounts"
        :key="a.id"
        class="rounded-[var(--ui-radius)] px-2 py-1.5 not-first:mt-1"
        :class="a.is_default ? 'bg-elevated/60' : ''"
      >
        <!-- The row: what it is called, whether it is signed in, where its
             login lives, and whether new tasks start on it. -->
        <div class="flex items-center gap-2.5">
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-1.5">
              <span class="truncate text-[13px] text-highlighted">{{ a.alias }}</span>
              <UBadge v-if="a.system" color="neutral" variant="soft" size="sm">system</UBadge>
              <UBadge v-if="a.is_default" color="primary" variant="soft" size="sm">default</UBadge>
              <UBadge :color="a.signed_in ? 'success' : 'warning'" variant="soft" size="sm">
                {{ a.signed_in ? a.detail || "signed in" : "not signed in" }}
              </UBadge>
            </div>
            <p class="m-0 truncate text-[11px] text-dimmed" :title="a.home">
              {{ a.home || "the CLI's own folder" }}
            </p>
          </div>
          <!-- Every row repeats these two words, so each carries the name of
               the subscription it acts on for anything reading labels. -->
          <UButton
            v-if="!a.is_default"
            size="xs"
            color="neutral"
            variant="ghost"
            label="Make default"
            :disabled="!!busy"
            :aria-label="`Start new ${p.display_name} tasks on ${a.alias}`"
            @click="choose(p.name, a.id)"
          />
          <UButton
            size="xs"
            color="neutral"
            variant="subtle"
            :label="p.direct_login && a.signed_in ? 'Connection' : a.signed_in ? 'Sign in again' : 'Sign in'"
            :disabled="!p.available || !!busy"
            :aria-label="`Sign in to ${a.alias} for ${p.display_name}`"
            @click="p.direct_login ? startConnect(p.name, a) : signIn(p.name, a)"
          />
          <UButton
            size="xs"
            color="neutral"
            variant="ghost"
            icon="i-lucide-pencil"
            :disabled="!!busy"
            :aria-label="`Rename ${a.alias}`"
            @click="startEdit(p.name, a)"
          />
          <UButton
            v-if="!a.system"
            size="xs"
            color="neutral"
            variant="ghost"
            icon="i-lucide-trash-2"
            :disabled="!!busy"
            :aria-label="`Remove ${a.alias}`"
            @click="startRemove(p.name, a)"
          />
        </div>

        <form v-if="connecting[rowKey(p.name, a)]" class="mt-2 space-y-2" @submit.prevent="saveConnection(p.name, a)">
          <p class="text-xs text-dimmed">
            Sign in at <a href="https://opencode.ai/auth" target="_blank" rel="noopener noreferrer" class="text-primary underline">OpenCode</a>,
            subscribe to Go and paste its key here. The key is saved privately in this login folder.
          </p>
          <label class="block text-xs text-muted">
            Run with
            <select v-model="connecting[rowKey(p.name, a)].connection" class="ml-2 rounded border border-default bg-default p-1">
              <option value="auto">Automatic (CLI when installed)</option>
              <option value="direct">Direct (without CLI)</option>
              <option value="cli">OpenCode CLI</option>
            </select>
          </label>
          <label class="block text-xs text-muted">
            OpenCode Go key {{ a.signed_in ? "(leave blank to keep it)" : "" }}
            <UInput v-model="connecting[rowKey(p.name, a)].key" type="password" autocomplete="off" size="xs" class="mt-1 w-full" />
          </label>
          <p class="text-xs text-dimmed">Changing between CLI and Direct starts a new conversation on the next prompt. Direct supports file tools; shell commands require Full access.</p>
          <div class="flex gap-2">
            <UButton type="submit" size="xs" label="Save connection" :disabled="!!busy || (!a.signed_in && !connecting[rowKey(p.name, a)].key.trim())" />
            <UButton size="xs" color="neutral" variant="ghost" label="Cancel" @click="delete connecting[rowKey(p.name, a)]" />
            <UButton v-if="p.cli_installed" size="xs" color="neutral" variant="ghost" label="Sign in with CLI" @click="signIn(p.name, a)" />
          </div>
        </form>

        <!-- Renaming and repointing are one form: both describe the same
             subscription, and a login moved to another folder is usually
             being renamed as well. -->
        <div v-if="editing[`${p.name}:${a.id}`]" class="mt-1.5 flex items-end gap-2">
          <label class="flex-1 text-[11px] text-muted">
            Name
            <UInput v-model="editing[`${p.name}:${a.id}`].alias" size="xs" class="mt-0.5 w-full" />
          </label>
          <label v-if="!a.system" class="flex-[2] text-[11px] text-muted">
            Login folder
            <UInput v-model="editing[`${p.name}:${a.id}`].home" size="xs" class="mt-0.5 w-full" />
          </label>
          <UButton size="xs" label="Save" :disabled="!!busy" @click="submitEdit(p.name, a)" />
          <UButton
            size="xs"
            color="neutral"
            variant="ghost"
            label="Cancel"
            @click="delete editing[`${p.name}:${a.id}`]"
          />
        </div>

        <div v-if="removing[a.id]" class="mt-1.5">
          <p class="m-0 text-[11px] text-warning">
            Remove {{ a.alias }}?
            {{
              removing[a.id].sessions || removing[a.id].schedules
                ? `${removing[a.id].sessions} task(s) and ${removing[a.id].schedules} job(s) in this profile still run on it and will stop until another subscription is chosen for them.`
                : "Nothing in this profile runs on it."
            }}
            Removal affects every profile. Its login folder is left on disk.
          </p>
          <div class="mt-1 flex gap-2">
            <UButton
              size="xs"
              color="error"
              label="Remove"
              :disabled="!!busy"
              @click="confirmRemove(p.name, a)"
            />
            <UButton
              size="xs"
              color="neutral"
              variant="ghost"
              label="Keep"
              @click="delete removing[a.id]"
            />
          </div>
        </div>

        <!-- What the sign-in actually ran, so it can be repeated by hand: a
             machine with no terminal to open, or a browser reading this from
             another one, still gets the command. -->
        <div v-if="command[rowKey(p.name, a)]" class="mt-1.5">
          <p class="m-0 text-[11px] text-dimmed">
            {{
              command[rowKey(p.name, a)].spawned
                ? "A terminal was opened. Finish the login there, then reopen Settings."
                : "No terminal could be opened. Run this where the CLIs are installed:"
            }}
          </p>
          <div class="mt-1 flex items-center gap-1.5">
            <code
              class="min-w-0 flex-1 truncate rounded bg-elevated px-1.5 py-1 text-[11px]"
              :title="command[rowKey(p.name, a)].command"
            >{{ command[rowKey(p.name, a)].command }}</code>
            <UButton
              size="xs"
              color="neutral"
              variant="ghost"
              icon="i-lucide-copy"
              aria-label="Copy the login command"
              @click="copy(command[rowKey(p.name, a)].command)"
            />
          </div>
        </div>
      </div>

      <!-- Only the name is asked for: the folder is one this app manages,
           beside its database, unless a particular place is wanted. -->
      <div v-if="p.multi_account" class="mt-1.5">
        <UButton
          v-if="!adding[p.name]"
          size="xs"
          color="neutral"
          variant="subtle"
          icon="i-lucide-plus"
          :label="`Add a ${p.display_name} subscription`"
          @click="startAdd(p.name)"
        />
        <div v-else class="flex items-end gap-2">
          <label class="flex-1 text-[11px] text-muted">
            Name
            <UInput
              v-model="adding[p.name].alias"
              size="xs"
              placeholder="Personal"
              autofocus
              class="mt-0.5 w-full"
              @keydown.enter="submitAdd(p.name)"
            />
          </label>
          <label class="flex-[2] text-[11px] text-muted">
            Login folder <span class="text-dimmed">(optional)</span>
            <UInput
              v-model="adding[p.name].home"
              size="xs"
              placeholder="managed for you"
              class="mt-0.5 w-full"
              @keydown.enter="submitAdd(p.name)"
            />
          </label>
          <UButton
            size="xs"
            label="Add"
            :disabled="!adding[p.name].alias.trim() || !!busy"
            @click="submitAdd(p.name)"
          />
          <UButton
            size="xs"
            color="neutral"
            variant="ghost"
            label="Cancel"
            @click="delete adding[p.name]"
          />
        </div>
      </div>
      <p v-else class="mt-1 text-[11px] text-dimmed">
        {{ p.display_name }} runs on the one login its CLI is signed in to.
      </p>
    </section>
  </section>
</template>
