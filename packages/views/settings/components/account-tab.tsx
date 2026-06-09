"use client";

import { useEffect, useRef, useState } from "react";
import { Camera, Eye, EyeOff, Loader2, Save } from "lucide-react";
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
import { useT } from "../../i18n";

// Mirror server/internal/handler/auth.go:MaxProfileDescriptionLen. Counted in
// JS String.length (UTF-16 code units) here while the server counts runes,
// so a profile full of supplementary-plane emoji will trip the client cap
// before the server's — which is the safer direction of drift.
const MAX_PROFILE_DESCRIPTION_LEN = 2000;

export function AccountTab() {
  const { t } = useT("settings");
  const user = useAuthStore((s) => s.user);
  const setUser = useAuthStore((s) => s.setUser);

  const [profileName, setProfileName] = useState(user?.name ?? "");
  const [profileDescription, setProfileDescription] = useState(
    user?.profile_description ?? "",
  );
  const [gitToken, setGitToken] = useState(user?.integration_tokens?.git_token ?? "");
  const [feishuMcpToken, setFeishuMcpToken] = useState(
    user?.integration_tokens?.feishu_mcp_token ?? "",
  );
  const [paonesToken, setPaonesToken] = useState(
    user?.integration_tokens?.paones_token ?? "",
  );
  const [jingweiToken, setJingweiToken] = useState(
    user?.integration_tokens?.jingwei_token ?? "",
  );
  const [profileSaving, setProfileSaving] = useState(false);
  const { upload, uploading } = useFileUpload(api);
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setProfileName(user?.name ?? "");
    setProfileDescription(user?.profile_description ?? "");
    setGitToken(user?.integration_tokens?.git_token ?? "");
    setFeishuMcpToken(user?.integration_tokens?.feishu_mcp_token ?? "");
    setPaonesToken(user?.integration_tokens?.paones_token ?? "");
    setJingweiToken(user?.integration_tokens?.jingwei_token ?? "");
  }, [user]);

  const descriptionTooLong = profileDescription.length > MAX_PROFILE_DESCRIPTION_LEN;

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
        integration_tokens: {
          git_token: gitToken,
          feishu_mcp_token: feishuMcpToken,
          paones_token: paonesToken,
          jingwei_token: jingweiToken,
        },
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
            <div className="grid gap-4 border-t pt-4 sm:grid-cols-2">
              <TokenInput
                label="Git 权限 token"
                value={gitToken}
                onChange={setGitToken}
                placeholder="请输入 token"
              />
              <TokenInput
                label="飞书 MCP 的 token"
                value={feishuMcpToken}
                onChange={setFeishuMcpToken}
                placeholder="请输入 token"
              />
              <TokenInput
                label="PAones 发布 token"
                value={paonesToken}
                onChange={setPaonesToken}
                placeholder="请输入 token"
              />
              <TokenInput
                label="精卫 token"
                value={jingweiToken}
                onChange={setJingweiToken}
                placeholder="请输入 token"
              />
            </div>
            <div className="flex items-center justify-end gap-2 pt-1">
              <Button
                size="sm"
                onClick={handleProfileSave}
                disabled={profileSaving || !profileName.trim() || descriptionTooLong}
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

function TokenInput({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}) {
  const [visible, setVisible] = useState(false);

  return (
    <div>
      <Label className="text-xs text-muted-foreground">{label}</Label>
      <div className="relative mt-1">
        <Input
          type={visible ? "text" : "password"}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          autoComplete="off"
          className="pr-10"
        />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="absolute right-1 top-1/2 h-7 w-7 -translate-y-1/2 text-muted-foreground"
          onClick={() => setVisible((next) => !next)}
          aria-label={visible ? "隐藏 token" : "显示 token"}
        >
          {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </Button>
      </div>
    </div>
  );
}
