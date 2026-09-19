<script setup>
import { computed, reactive, ref, watchEffect } from "vue";
import { S, configureAPIProvider, removeAPIProviderKey } from "../../store";
import { fuzzyAny } from "../../fuzzy";

const props = defineProps({ filter: { type: String, default: "" } });
const emit = defineEmits(["count"]);
const keys = reactive({});
const errors = reactive({});
const busy = ref("");
const providers = computed(() => S.providers.filter((p) => p.kind === "api" &&
  fuzzyAny([p.display_name, p.name, "providers", "api", "key", "connection", "billing"], props.filter) !== null));
watchEffect(() => emit("count", providers.value.length));

async function save(provider, enabled = true, refresh = false) {
  busy.value = provider.name;
  errors[provider.name] = "";
  try {
    await configureAPIProvider(provider.name, { key: refresh || !enabled ? "" : keys[provider.name] || "", enabled, refresh });
    keys[provider.name] = "";
  } catch (error) { errors[provider.name] = error.message; }
  finally { busy.value = ""; }
}
async function remove(provider) {
  busy.value = provider.name;
  errors[provider.name] = "";
  try { await removeAPIProviderKey(provider.name); keys[provider.name] = ""; }
  catch (error) { errors[provider.name] = error.message; }
  finally { busy.value = ""; }
}
</script>

<template>
  <div>
    <h3 class="font-semibold text-highlighted">API Providers</h3>
    <p class="mb-4 text-xs text-dimmed">
      Connect an API key for all profiles. Choose visible and favourite models in each profile. API usage is billed separately from subscriptions.
      Only enabled connections appear in Models. Keys are stored on the Agenttik host and are never shown again.
    </p>
    <section v-for="provider in providers" :key="provider.name" role="group" :aria-label="provider.display_name" class="mb-4 rounded-lg border border-default p-3">
      <div class="mb-2 flex items-center justify-between gap-2">
        <h4 class="font-medium text-highlighted">{{ provider.display_name }}</h4>
        <UBadge :color="provider.api.enabled ? 'success' : 'neutral'" variant="soft" size="sm">
          {{ provider.api.enabled ? `${provider.models.length} ${provider.models.length === 1 ? 'model' : 'models'}` : 'Disabled' }}
        </UBadge>
      </div>
      <form @submit.prevent="save(provider)">
        <label :for="`key-${provider.name}`" class="mb-1 block text-xs text-muted">API key</label>
        <UInput :id="`key-${provider.name}`" v-model="keys[provider.name]" type="password" autocomplete="new-password"
          :aria-label="`${provider.display_name} key`" :placeholder="provider.api.key_set ? 'Leave blank to keep saved key' : 'Enter API key'"
          class="mb-2 w-full" :disabled="!!busy" />
        <div class="flex flex-wrap items-center gap-2">
          <UButton type="submit" size="xs" :label="provider.api.enabled ? 'Save key' : 'Enable'"
            :loading="busy === provider.name" :disabled="!!busy || (!provider.api.key_set && !keys[provider.name]?.trim())" />
          <UButton v-if="provider.api.enabled" type="button" size="xs" color="neutral" variant="soft" label="Refresh models" :disabled="!!busy" @click="save(provider, true, true)" />
          <UButton v-if="provider.api.enabled" type="button" size="xs" color="neutral" variant="ghost" label="Disable" :disabled="!!busy" @click="save(provider, false)" />
          <UButton v-if="provider.api.key_set" type="button" size="xs" color="error" variant="ghost" label="Remove key" :disabled="!!busy" @click="remove(provider)" />
          <UButton :to="provider.api.key_url" target="_blank" rel="noopener noreferrer" size="xs" color="neutral" variant="link" label="Get API key" trailing-icon="i-lucide-external-link" />
        </div>
      </form>
      <p v-if="errors[provider.name]" role="alert" class="mt-2 text-xs text-error">{{ errors[provider.name] }}</p>
    </section>
    <p class="text-xs text-dimmed">
      API tasks can read project instructions and use local tools. Plan permits reads; Workspace adds file edits;
      Choose Full access in the task’s Tool access menu to use installed shells and Python. Each turn is limited to 64 steps. Set spending limits with your provider.
    </p>
  </div>
</template>
