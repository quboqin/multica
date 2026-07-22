"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Eye, EyeOff, Plus, Trash2 } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";
import { useAuthStore } from "@multica/core/auth";
import { api } from "@multica/core/api";
import {
  DEFAULT_INTEGRATION_TOKEN_KEYS,
  integrationTokenEnvKey,
  integrationTokenPlaceholder,
} from "@multica/core/integration-tokens";
import type { IntegrationTokens } from "@multica/core/types";
import { AvatarUploadControl } from "../../common/avatar-upload-control";
import { useT } from "../../i18n";
import {
  SettingsCard,
  SettingsRow,
  SettingsSaveState,
  SettingsSection,
  SettingsTab,
} from "./settings-layout";
import { useAutoSave } from "./use-auto-save";

// Mirror server/internal/handler/auth.go:MaxProfileDescriptionLen. Counted in
// JS String.length (UTF-16 code units) here while the server counts runes,
// so a profile full of supplementary-plane emoji will trip the client cap
// before the server's — which is the safer direction of drift.
const MAX_PROFILE_DESCRIPTION_LEN = 2000;
const TOKEN_KEY_OPTIONS = [...DEFAULT_INTEGRATION_TOKEN_KEYS, "notion_token"];

interface ProfileDraft {
  name: string;
  profileDescription: string;
  integrationTokens: IntegrationTokens;
}

function profilesEqual(left: ProfileDraft, right: ProfileDraft) {
  return (
    left.name === right.name &&
    left.profileDescription === right.profileDescription &&
    integrationTokensEqual(left.integrationTokens, right.integrationTokens)
  );
}

type TokenRow = {
  id: string;
  key: string;
  value: string;
};

function tokenRowsFromIntegrationTokens(tokens: IntegrationTokens | undefined): TokenRow[] {
  const entriesByKey = new Map<string, string>();
  for (const [key, value] of Object.entries(tokens ?? {})) {
    const trimmedKey = key.trim();
    const trimmedValue = (value ?? "").trim();
    if (trimmedKey !== "" && trimmedValue !== "") {
      entriesByKey.set(trimmedKey, trimmedValue);
    }
  }
  const optionOrder = new Map(TOKEN_KEY_OPTIONS.map((key, index) => [key, index]));
  return Array.from(entriesByKey.entries())
    .sort(([a], [b]) => {
      const aOrder = optionOrder.get(a);
      const bOrder = optionOrder.get(b);
      if (aOrder != null || bOrder != null) {
        return (aOrder ?? Number.MAX_SAFE_INTEGER) - (bOrder ?? Number.MAX_SAFE_INTEGER);
      }
      return a.localeCompare(b);
    })
    .map(([key, value], index) => ({ id: "saved-" + index + "-" + key, key, value }));
}

function buildIntegrationTokenPatch(
  rows: TokenRow[],
  original: IntegrationTokens | undefined,
): IntegrationTokens {
  const next: IntegrationTokens = {};
  for (const row of rows) {
    const key = row.key.trim();
    const value = row.value.trim();
    if (key === "" || value === "") continue;
    next[key] = value;
  }
  for (const key of Object.keys(original ?? {})) {
    if (key.trim() !== "" && next[key] == null) {
      next[key] = "";
    }
  }
  return next;
}

function integrationTokensEqual(left: IntegrationTokens, right: IntegrationTokens) {
  const leftEntries = Object.entries(left).filter(([, value]) => (value ?? "").trim() !== "");
  const rightEntries = Object.entries(right).filter(([, value]) => (value ?? "").trim() !== "");
  if (leftEntries.length !== rightEntries.length) return false;
  return leftEntries.every(([key, value]) => right[key] === value);
}

function hasDuplicateTokenKeys(rows: TokenRow[]): boolean {
  const seen = new Set<string>();
  for (const row of rows) {
    const key = row.key.trim();
    if (key === "") continue;
    if (seen.has(key)) return true;
    seen.add(key);
  }
  return false;
}

export function AccountTab() {
  const { t } = useT("settings");
  const user = useAuthStore((s) => s.user);
  const setUser = useAuthStore((s) => s.setUser);

  const [profileName, setProfileName] = useState(user?.name ?? "");
  const [profileDescription, setProfileDescription] = useState(
    user?.profile_description ?? "",
  );
  const [tokenRows, setTokenRows] = useState<TokenRow[]>(() =>
    tokenRowsFromIntegrationTokens(user?.integration_tokens),
  );
  const tokenRowSeqRef = useRef(0);

  useEffect(() => {
    setProfileName(user?.name ?? "");
    setProfileDescription(user?.profile_description ?? "");
    setTokenRows(tokenRowsFromIntegrationTokens(user?.integration_tokens));
    // Preserve in-progress edits when an avatar upload or auto-save replaces
    // the current user object in the auth store.
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentionally keyed on user identity
  }, [user?.id]);

  const descriptionTooLong = profileDescription.length > MAX_PROFILE_DESCRIPTION_LEN;
  const duplicateTokenKeys = hasDuplicateTokenKeys(tokenRows);
  const invalidTokenKey = tokenRows.some(
    (row) => row.value.trim() !== "" && integrationTokenEnvKey(row.key) === "",
  );

  const draft = useMemo(
    () => ({
      name: profileName,
      profileDescription,
      integrationTokens: buildIntegrationTokenPatch(tokenRows, user?.integration_tokens),
    }),
    [profileDescription, profileName, tokenRows, user?.integration_tokens],
  );
  const savedDraft = useMemo(
    () => ({
      name: user?.name ?? "",
      profileDescription: user?.profile_description ?? "",
      integrationTokens: user?.integration_tokens ?? {},
    }),
    [user?.integration_tokens, user?.name, user?.profile_description],
  );
  const saveProfile = useCallback(
    async (next: ProfileDraft) => {
      const updated = await api.updateMe({
        name: next.name,
        profile_description: next.profileDescription,
        integration_tokens: next.integrationTokens,
      });
      setUser(updated);
    },
    [setUser],
  );
  const autoSave = useAutoSave({
    value: draft,
    savedValue: savedDraft,
    onSave: saveProfile,
    onSuccess: () =>
      toast.success(t(($) => $.account.toast_profile_updated), {
        id: "settings-auto-save",
      }),
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t(($) => $.account.toast_profile_failed),
      ),
    enabled:
      !!user &&
      !!profileName.trim() &&
      !descriptionTooLong &&
      !duplicateTokenKeys &&
      !invalidTokenKey,
    isEqual: profilesEqual,
  });

  return (
    <SettingsTab title={t(($) => $.page.tabs.profile)}>
      <SettingsSection
        title={t(($) => $.account.section_profile)}
        action={
          <SettingsSaveState
            status={autoSave.status}
            savingLabel={t(($) => $.auto_save.saving)}
            savedLabel={t(($) => $.auto_save.saved)}
            errorLabel={t(($) => $.auto_save.failed)}
          />
        }
      >
        <SettingsCard>
          <SettingsRow
            label={t(($) => $.account.avatar_label)}
            description={t(($) => $.account.click_avatar_hint)}
            size="none"
          >
            <div className="flex justify-start sm:justify-end">
              <AvatarUploadControl
                variant="user"
                value={user?.avatar_url ?? null}
                name={user?.name ?? ""}
                size={64}
                onUploaded={async (url) => {
                  try {
                    const updated = await api.updateMe({ avatar_url: url });
                    setUser(updated);
                    toast.success(t(($) => $.account.toast_avatar_updated), {
                      id: "settings-auto-save",
                    });
                  } catch (error) {
                    toast.error(
                      error instanceof Error
                        ? error.message
                        : t(($) => $.account.toast_avatar_failed),
                    );
                  }
                }}
              />
            </div>
          </SettingsRow>

          <SettingsRow
            label={t(($) => $.account.name_label)}
            size="text"
          >
            <Input
              type="text"
              name="profile-name"
              autoComplete="name"
              aria-label={t(($) => $.account.name_label)}
              value={profileName}
              onChange={(event) => setProfileName(event.target.value)}
              onBlur={autoSave.flush}
            />
          </SettingsRow>

          <SettingsRow
            label={t(($) => $.account.profile_description_label)}
            description={t(($) => $.account.profile_description_hint)}
            size="text"
            align="start"
          >
            <div>
              <Textarea
                name="profile-description"
                autoComplete="off"
                aria-label={t(($) => $.account.profile_description_label)}
                value={profileDescription}
                onChange={(event) => setProfileDescription(event.target.value)}
                onBlur={autoSave.flush}
                placeholder={t(($) => $.account.profile_description_placeholder)}
                rows={5}
                maxLength={MAX_PROFILE_DESCRIPTION_LEN}
                aria-invalid={descriptionTooLong}
                className="resize-y"
              />
              <div className="mt-1 flex justify-end text-xs text-muted-foreground">
                <span
                  className={descriptionTooLong ? "text-destructive shrink-0" : "shrink-0"}
                  aria-live="polite"
                >
                  {profileDescription.length}/{MAX_PROFILE_DESCRIPTION_LEN}
                </span>
              </div>
              {descriptionTooLong ? (
                <p className="mt-1 text-xs text-destructive">
                  {t(($) => $.account.profile_description_too_long, {
                    max: MAX_PROFILE_DESCRIPTION_LEN,
                    count: profileDescription.length,
                  })}
                </p>
              ) : null}
            </div>
          </SettingsRow>

          <SettingsRow
            label={t(($) => $.account.integration_tokens_title)}
            description={t(($) => $.account.integration_tokens_hint)}
            size="text"
            align="start"
          >
            <IntegrationTokenEditor
              rows={tokenRows}
              duplicateKeys={duplicateTokenKeys}
              invalidKey={invalidTokenKey}
              onAdd={() => {
                const used = new Set(tokenRows.map((row) => row.key.trim()).filter(Boolean));
                const nextKey = TOKEN_KEY_OPTIONS.find((key) => !used.has(key)) ?? "";
                tokenRowSeqRef.current += 1;
                setTokenRows((rows) => [
                  ...rows,
                  { id: "new-" + tokenRowSeqRef.current, key: nextKey, value: "" },
                ]);
              }}
              onChange={(id, patch) => {
                setTokenRows((rows) =>
                  rows.map((row) => (row.id === id ? { ...row, ...patch } : row)),
                );
              }}
              onRemove={(id) => {
                setTokenRows((rows) => rows.filter((row) => row.id !== id));
              }}
              onBlur={autoSave.flush}
            />
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
    </SettingsTab>
  );
}

function IntegrationTokenEditor({
  rows,
  duplicateKeys,
  invalidKey,
  onAdd,
  onChange,
  onRemove,
  onBlur,
}: {
  rows: TokenRow[];
  duplicateKeys: boolean;
  invalidKey: boolean;
  onAdd: () => void;
  onChange: (id: string, patch: Partial<Pick<TokenRow, "key" | "value">>) => void;
  onRemove: (id: string) => void;
  onBlur: () => void;
}) {
  const { t } = useT("settings");
  const datalistId = "integration-token-key-options";

  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <Button type="button" variant="outline" size="sm" onClick={onAdd}>
          <Plus className="h-3 w-3" />
          {t(($) => $.account.integration_tokens_add)}
        </Button>
      </div>

      <datalist id={datalistId}>
        {TOKEN_KEY_OPTIONS.map((key) => (
          <option key={key} value={key} />
        ))}
      </datalist>

      {rows.length === 0 ? (
        <div className="rounded-md border border-dashed px-3 py-4 text-xs text-muted-foreground">
          {t(($) => $.account.integration_tokens_empty)}
        </div>
      ) : (
        <div className="space-y-2">
          {rows.map((row) => (
            <TokenInput
              key={row.id}
              row={row}
              datalistId={datalistId}
              onChange={(patch) => onChange(row.id, patch)}
              onRemove={() => onRemove(row.id)}
              onBlur={onBlur}
            />
          ))}
        </div>
      )}

      {duplicateKeys ? (
        <p className="text-xs text-destructive">
          {t(($) => $.account.integration_tokens_duplicate_error)}
        </p>
      ) : null}
      {invalidKey ? (
        <p className="text-xs text-destructive">
          {t(($) => $.account.integration_tokens_invalid_key_error)}
        </p>
      ) : null}
    </div>
  );
}

function TokenInput({
  row,
  datalistId,
  onChange,
  onRemove,
  onBlur,
}: {
  row: TokenRow;
  datalistId: string;
  onChange: (patch: Partial<Pick<TokenRow, "key" | "value">>) => void;
  onRemove: () => void;
  onBlur: () => void;
}) {
  const { t } = useT("settings");
  const [visible, setVisible] = useState(false);
  const placeholder = integrationTokenPlaceholder(row.key);

  return (
    <div
      data-testid="integration-token-row"
      className="grid gap-2 rounded-md border bg-muted/20 p-3 sm:grid-cols-[minmax(160px,220px)_1fr_auto] sm:items-start"
    >
      <div className="space-y-1">
        <Label className="text-xs text-muted-foreground">
          {t(($) => $.account.integration_tokens_key_label)}
        </Label>
        <Input
          value={row.key}
          onChange={(event) => onChange({ key: event.target.value })}
          onBlur={onBlur}
          placeholder={t(($) => $.account.integration_tokens_key_placeholder)}
          autoComplete="off"
          list={datalistId}
          className="font-mono text-xs"
        />
        {placeholder ? (
          <p className="break-all text-[11px] text-muted-foreground">
            {t(($) => $.account.integration_tokens_env_hint, {
              env: placeholder.slice(2, -1),
            })}
          </p>
        ) : null}
      </div>

      <div className="space-y-1">
        <Label className="text-xs text-muted-foreground">
          {t(($) => $.account.integration_tokens_token_label)}
        </Label>
        <div className="relative">
          <Input
            type={visible ? "text" : "password"}
            value={row.value}
            onChange={(event) => onChange({ value: event.target.value })}
            onBlur={onBlur}
            placeholder={t(($) => $.account.integration_tokens_value_placeholder)}
            autoComplete="off"
            className="pr-10"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="absolute right-1 top-1/2 h-7 w-7 -translate-y-1/2 text-muted-foreground"
            onClick={() => setVisible((next) => !next)}
            aria-label={
              visible
                ? t(($) => $.account.integration_tokens_hide_aria)
                : t(($) => $.account.integration_tokens_show_aria)
            }
          >
            {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
          </Button>
        </div>
      </div>

      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="text-muted-foreground sm:mt-6"
        onClick={onRemove}
        aria-label={t(($) => $.account.integration_tokens_delete_aria, {
          key: row.key || t(($) => $.account.integration_tokens_key_fallback),
        })}
      >
        <Trash2 className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}
