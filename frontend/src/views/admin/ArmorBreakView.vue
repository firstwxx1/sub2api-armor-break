<template>
  <AppLayout>
    <div class="space-y-6">
      <section
        class="flex flex-col gap-4 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between"
      >
        <div class="min-w-0">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t("admin.armorBreak.title") }}
          </h2>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.armorBreak.description") }}
          </p>
          <div class="mt-3 flex flex-wrap gap-2 text-xs text-gray-600 dark:text-gray-300">
            <span class="rounded bg-gray-100 px-2 py-1 dark:bg-dark-700">
              {{ t("admin.armorBreak.personaDir") }}: {{ state?.persona_dir || "personas" }}
            </span>
            <span
              v-if="state?.enabled"
              class="rounded bg-emerald-100 px-2 py-1 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300"
            >
              {{ t("admin.armorBreak.statusOn") }}
            </span>
            <span v-else class="rounded bg-gray-100 px-2 py-1 dark:bg-dark-700">
              {{ t("admin.armorBreak.statusOff") }}
            </span>
          </div>
        </div>
        <button
          type="button"
          class="btn btn-secondary flex-shrink-0"
          :disabled="loading"
          :title="t('common.refresh')"
          @click="loadState"
        >
          <Icon name="refresh" size="sm" />
          <span class="sr-only">{{ t("common.refresh") }}</span>
        </button>
      </section>

      <div
        v-if="state?.load_error"
        class="border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200"
      >
        {{ t("admin.armorBreak.loadError") }}: {{ state.load_error }}
      </div>

      <!-- 开关与模式 -->
      <section class="border border-gray-200 p-5 dark:border-dark-700">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t("admin.armorBreak.switchTitle") }}
        </h3>
        <div class="mt-4 grid gap-4 sm:grid-cols-3">
          <label class="flex items-center gap-3 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="form.enabled" type="checkbox" class="h-4 w-4" />
            {{ t("admin.armorBreak.enable") }}
          </label>
          <label class="flex flex-col gap-1 text-sm text-gray-700 dark:text-gray-300">
            <span>{{ t("admin.armorBreak.mode") }}</span>
            <select v-model="form.mode" class="input">
              <option value="replace">{{ t("admin.armorBreak.modeReplace") }}</option>
              <option value="prepend">{{ t("admin.armorBreak.modePrepend") }}</option>
            </select>
          </label>
          <label class="flex flex-col gap-1 text-sm text-gray-700 dark:text-gray-300">
            <span>{{ t("admin.armorBreak.activePersona") }}</span>
            <select v-model="form.persona" class="input">
              <option value="">{{ t("admin.armorBreak.noPersona") }}</option>
              <option v-for="p in personas" :key="p.name" :value="p.name">{{ p.name }}</option>
            </select>
          </label>
        </div>
        <div class="mt-4 flex items-center gap-3">
          <button type="button" class="btn btn-primary" :disabled="saving" @click="saveState">
            {{ saving ? t("common.processing") : t("common.save") }}
          </button>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.armorBreak.hotHint") }}
          </p>
        </div>
      </section>

      <!-- 人格库 -->
      <section class="border border-gray-200 dark:border-dark-700">
        <div class="border-b border-gray-200 px-5 py-3 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t("admin.armorBreak.libraryTitle") }}
          </h3>
        </div>
        <div v-if="loading" class="flex min-h-32 items-center justify-center text-sm text-gray-500">
          {{ t("common.loading") }}
        </div>
        <div
          v-else-if="personas.length === 0"
          class="flex min-h-32 items-center justify-center text-sm text-gray-500"
        >
          {{ t("admin.armorBreak.emptyLibrary") }}
        </div>
        <ul v-else class="divide-y divide-gray-200 dark:divide-dark-700">
          <li
            v-for="p in personas"
            :key="p.name"
            class="flex flex-wrap items-center gap-3 px-5 py-3 text-sm"
          >
            <span
              class="min-w-0 flex-1 truncate font-medium text-gray-800 dark:text-gray-200"
              :title="p.name"
            >
              {{ p.name }}
              <span
                v-if="state?.persona === p.name"
                class="ml-2 rounded bg-emerald-100 px-1.5 py-0.5 text-xs text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300"
              >
                {{ t("admin.armorBreak.activeBadge") }}
              </span>
            </span>
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatSize(p.size) }}</span>
            <span class="flex gap-2">
              <button type="button" class="btn btn-secondary btn-xs" @click="previewPersona(p.name)">
                {{ t("admin.armorBreak.preview") }}
              </button>
              <button
                type="button"
                class="btn btn-secondary btn-xs"
                :disabled="state?.persona === p.name"
                @click="selectPersona(p.name)"
              >
                {{ t("admin.armorBreak.useThis") }}
              </button>
              <button
                type="button"
                class="btn btn-secondary btn-xs text-red-600 dark:text-red-400"
                @click="removePersona(p.name)"
              >
                {{ t("common.delete") }}
              </button>
            </span>
          </li>
        </ul>
      </section>

      <!-- 编辑器 -->
      <section class="border border-gray-200 p-5 dark:border-dark-700">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t("admin.armorBreak.editorTitle") }}
        </h3>
        <div class="mt-4 flex flex-col gap-3">
          <input
            v-model="editor.name"
            type="text"
            class="input max-w-md"
            :placeholder="t('admin.armorBreak.editorNamePlaceholder')"
          />
          <textarea
            v-model="editor.content"
            rows="16"
            class="input font-mono text-xs"
            :placeholder="t('admin.armorBreak.editorContentPlaceholder')"
          />
          <div class="flex items-center gap-3">
            <button type="button" class="btn btn-primary" :disabled="uploading" @click="savePersona">
              {{ uploading ? t("common.processing") : t("admin.armorBreak.editorSave") }}
            </button>
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ editor.content.length.toLocaleString() }} {{ t("admin.armorBreak.chars") }}
            </p>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";

import AppLayout from "@/components/layout/AppLayout.vue";
import Icon from "@/components/icons/Icon.vue";
import {
  deletePersona,
  getArmorBreakState,
  getPersonaContent,
  putPersona,
  updateArmorBreakState,
  type ArmorBreakMode,
  type ArmorBreakState,
  type PersonaInfo,
} from "@/api/admin/armorBreak";
import { useAppStore } from "@/stores";

const { t } = useI18n();
const appStore = useAppStore();

const state = ref<ArmorBreakState | null>(null);
const personas = ref<PersonaInfo[]>([]);
const loading = ref(false);
const saving = ref(false);
const uploading = ref(false);

const form = reactive<{ enabled: boolean; mode: ArmorBreakMode; persona: string }>({
  enabled: false,
  mode: "replace",
  persona: "",
});

const editor = reactive({ name: "", content: "" });

function errorMessage(error: unknown): string {
  if (error && typeof error === "object" && "message" in error) {
    return String((error as { message: unknown }).message);
  }
  return String(error);
}

function formatSize(size: number): string {
  if (size < 1024) return `${size} B`;
  return `${(size / 1024).toFixed(1)} KB`;
}

async function loadState() {
  loading.value = true;
  try {
    const s = await getArmorBreakState();
    state.value = s;
    personas.value = s.personas ?? [];
    form.enabled = s.enabled;
    form.mode = s.mode ?? "replace";
    form.persona = s.persona ?? "";
  } catch (error) {
    appStore.showError(errorMessage(error));
  } finally {
    loading.value = false;
  }
}

async function saveState() {
  saving.value = true;
  try {
    await updateArmorBreakState({ enabled: form.enabled, mode: form.mode, persona: form.persona });
    appStore.showSuccess(t("admin.armorBreak.saveSuccess"));
    await loadState();
  } catch (error) {
    appStore.showError(errorMessage(error));
  } finally {
    saving.value = false;
  }
}

async function selectPersona(name: string) {
  try {
    await updateArmorBreakState({ persona: name });
    appStore.showSuccess(t("admin.armorBreak.selectSuccess", { name }));
    await loadState();
  } catch (error) {
    appStore.showError(errorMessage(error));
  }
}

async function previewPersona(name: string) {
  try {
    const p = await getPersonaContent(name);
    editor.name = p.name;
    editor.content = p.content;
  } catch (error) {
    appStore.showError(errorMessage(error));
  }
}

async function savePersona() {
  if (!editor.name.trim()) {
    appStore.showError(t("admin.armorBreak.nameRequired"));
    return;
  }
  uploading.value = true;
  try {
    await putPersona(editor.name.trim(), editor.content);
    appStore.showSuccess(t("admin.armorBreak.uploadSuccess"));
    await loadState();
  } catch (error) {
    appStore.showError(errorMessage(error));
  } finally {
    uploading.value = false;
  }
}

async function removePersona(name: string) {
  if (!window.confirm(t("admin.armorBreak.deleteConfirm", { name }))) return;
  try {
    await deletePersona(name);
    appStore.showSuccess(t("admin.armorBreak.deleteSuccess"));
    if (editor.name === name) {
      editor.name = "";
      editor.content = "";
    }
    await loadState();
  } catch (error) {
    appStore.showError(errorMessage(error));
  }
}

onMounted(loadState);
</script>
