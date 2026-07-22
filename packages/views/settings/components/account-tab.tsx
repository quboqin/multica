"use client";

import { useEffect, useRef, useState } from "react";
import { Camera, Eye, EyeOff, Loader2, Plus, Save, Trash2 } from "lucide-react";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";
import { useAuthStore } from "@multica/core/auth";
import { api } from "@multica/core/api";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import { useFileUpload } from "@multica/core/hooks/use-file-upload";
import {
  DEFAULT_INTEGRATION_TOKEN_KEYS,
  integrationTokenEnvKey,
  integrationTokenPlaceholder,
} from "@multica/core/integration-tokens";
import type { IntegrationTokens } from "@multica/core/types";
import { useT } from "../../i18n";

// Mirror server/internal/handler/auth.go:MaxProfileDescriptionLen. Counted in
// JS String.length (UTF-16 code units) here while the server counts runes,
// so a profile full of supplementary-plane emoji will trip the client cap
// before the server's — which is the safer direction of drift.
const MAX_PROFILE_DESCRIPTION_LEN = 2000;

const TOKEN_KEY_OPTIONS: string[] = [
  ...DEFAULT_INTEGRATION_TOKEN_KEYS,
  "notion_token",
];

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
  const rawEntries = Array.from(entriesByKey.entries());
  const optionOrder = new Map(TOKEN_KEY_OPTIONS.map((key, index) => [key, index]));
  const entries = rawEntries.sort(([a], [b]) => {
    const aOrder = optionOrder.get(a);
    const bOrder = optionOrder.get(b);
    if (aOrder != null || bOrder != null) {
      return (aOrder ?? Number.MAX_SAFE_INTEGER) - (bOrder ?? Number.MAX_SAFE_INTEGER);
    }
    return a.localeCompare(b);
  });
  return entries.map(([key, value], index) => ({
    id: "saved-" + index + "-" + key,
    key,
    value: value ?? "",
  }));
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
  const [profileSaving, setProfileSaving] = useState(false);
  const { upload, uploading } = useFileUpload(api);
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setProfileName(user?.name ?? "");
    setProfileDescription(user?.profile_description ?? "");
    setTokenRows(tokenRowsFromIntegrationTokens(user?.integration_tokens));
  }, [user]);

  const descriptionTooLong = profileDescription.length > MAX_PROFILE_DESCRIPTION_LEN;
  const duplicateTokenKeys = hasDuplicateTokenKeys(tokenRows);
  const invalidTokenKey = tokenRows.some(
    (row) => row.value.trim() !== "" && integrationTokenEnvKey(row.key) === "",
  );

  const initials = (user?.name ?? "")
    .split(" ")
    .map((w) => w[0])
    .join("")
    .toUpperCase()
    .slice(0, 2);

  const handleAvatarUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    // Reset input so the same file can be re-selected
    e.target.value = "";
    try {
      const result = await upload(file);
      if (!result) return;
      const updated = await api.updateMe({ avatar_url: result.link });
      setUser(updated);
      toast.success(t(($) => $.account.toast_avatar_updated));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t(($) => $.account.toast_avatar_failed));
    }
  };

  const handleProfileSave = async () => {
    if (descriptionTooLong) return;
    setProfileSaving(true);
    try {
      const updated = await api.updateMe({
        name: profileName,
        profile_description: profileDescription,
        integration_tokens: buildIntegrationTokenPatch(tokenRows, user?.integration_tokens),
      });
      setUser(updated);
      toast.success(t(($) => $.account.toast_profile_updated));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.account.toast_profile_failed));
    } finally {
      setProfileSaving(false);
    }
  };

  return (
    <div className="space-y-8">
      <section className="space-y-4">
        <h2 className="text-sm font-semibold">{t(($) => $.account.section_profile)}</h2>

        <Card>
          <CardContent className="space-y-4">
            {/* Avatar upload */}
            <div className="flex items-center gap-4">
              <button
                type="button"
                className="group relative h-16 w-16 shrink-0 rounded-full bg-muted overflow-hidden focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => fileInputRef.current?.click()}
                disabled={uploading}
              >
                {user?.avatar_url ? (
                  <img
                    src={resolvePublicFileUrl(user.avatar_url) ?? undefined}
                    alt={user.name}
                    className="h-full w-full object-cover"
                  />
                ) : (
                  <span className="flex h-full w-full items-center justify-center text-lg font-semibold text-muted-foreground">
                    {initials}
                  </span>
                )}
                <div className="absolute inset-0 flex items-center justify-center bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
                  {uploading ? (
                    <Loader2 className="h-5 w-5 animate-spin text-white" />
                  ) : (
                    <Camera className="h-5 w-5 text-white" />
                  )}
                </div>
              </button>
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                className="hidden"
                onChange={handleAvatarUpload}
              />
              <div className="text-xs text-muted-foreground">
                {t(($) => $.account.click_avatar_hint)}
              </div>
            </div>

            <div>
              <Label className="text-xs text-muted-foreground">{t(($) => $.account.name_label)}</Label>
              <Input
                type="search"
                value={profileName}
                onChange={(e) => setProfileName(e.target.value)}
                className="mt-1"
              />
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">
                {t(($) => $.account.profile_description_label)}
              </Label>
              <Textarea
                value={profileDescription}
                onChange={(e) => setProfileDescription(e.target.value)}
                placeholder={t(($) => $.account.profile_description_placeholder)}
                rows={5}
                maxLength={MAX_PROFILE_DESCRIPTION_LEN}
                className="mt-1 resize-y"
              />
              <div className="mt-1 flex items-start justify-between gap-3 text-xs text-muted-foreground">
                <span>{t(($) => $.account.profile_description_hint)}</span>
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
                  {
                    id: "new-" + tokenRowSeqRef.current,
                    key: nextKey,
                    value: "",
                  },
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
            />
            <div className="flex items-center justify-end gap-2 pt-1">
              <Button
                size="sm"
                onClick={handleProfileSave}
                disabled={
                  profileSaving ||
                  !profileName.trim() ||
                  descriptionTooLong ||
                  duplicateTokenKeys ||
                  invalidTokenKey
                }
              >
                <Save className="h-3 w-3" />
                {profileSaving ? t(($) => $.account.saving) : t(($) => $.account.save)}
              </Button>
            </div>
          </CardContent>
        </Card>
      </section>
    </div>
  );
}

function IntegrationTokenEditor({
  rows,
  duplicateKeys,
  invalidKey,
  onAdd,
  onChange,
  onRemove,
}: {
  rows: TokenRow[];
  duplicateKeys: boolean;
  invalidKey: boolean;
  onAdd: () => void;
  onChange: (id: string, patch: Partial<Pick<TokenRow, "key" | "value">>) => void;
  onRemove: (id: string) => void;
}) {
  const { t } = useT("settings");
  const datalistId = "integration-token-key-options";

  return (
    <div className="space-y-3 border-t pt-4">
      <div className="flex items-start justify-between gap-3">
        <div className="space-y-1">
          <h3 className="text-xs font-medium text-foreground">
            {t(($) => $.account.integration_tokens_title)}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t(($) => $.account.integration_tokens_hint)}
          </p>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={onAdd} className="shrink-0">
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
}: {
  row: TokenRow;
  datalistId: string;
  onChange: (patch: Partial<Pick<TokenRow, "key" | "value">>) => void;
  onRemove: () => void;
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
          onChange={(e) => onChange({ key: e.target.value })}
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
            onChange={(e) => onChange({ value: e.target.value })}
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
