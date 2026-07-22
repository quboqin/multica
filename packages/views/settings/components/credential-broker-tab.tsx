"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ExternalLink,
  KeyRound,
  MonitorUp,
  RefreshCw,
  ShieldCheck,
  Trash2,
  X,
} from "lucide-react";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { memberListOptions } from "@multica/core/workspace/queries";
import {
  credentialConnectorsOptions,
  credentialKeys,
  credentialProfilesOptions,
} from "@multica/core/credential";
import type {
  CredentialCrawlResult,
  CredentialLoginSession,
  CredentialProfile,
} from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";

export function CredentialBrokerTab() {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const user = useAuthStore((s) => s.user);
  const qc = useQueryClient();
  const [label, setLabel] = useState("Manual Acceptance");
  const [selectedConnectorID, setSelectedConnectorID] = useState("appgrowing");
  const [activeSession, setActiveSession] = useState<CredentialLoginSession | null>(null);
  const [sessionDialogOpen, setSessionDialogOpen] = useState(false);
  const [verification, setVerification] = useState<CredentialCrawlResult | null>(null);

  const connectorsQuery = useQuery(credentialConnectorsOptions(wsId));
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const profilesQuery = useQuery({
    ...credentialProfilesOptions(wsId),
    refetchInterval: activeSession ? 3000 : false,
  });

  const connectors = connectorsQuery.data?.connectors ?? [];
  const selectedConnector = useMemo(
    () => connectors.find((connector) => connector.id === selectedConnectorID) ?? connectors[0] ?? null,
    [connectors, selectedConnectorID],
  );
  const profiles = profilesQuery.data?.profiles ?? [];
  const activeProfiles = profiles.filter((profile) => profile.status === "active");
  const currentMember = members.find((member) => member.user_id === user?.id) ?? null;
  const canManage = currentMember?.role === "owner" || currentMember?.role === "admin";
  const pendingProfileID = activeSession?.profile_id ?? "";
  const pendingProfile = pendingProfileID
    ? profiles.find((profile) => profile.id === pendingProfileID)
    : null;

  const startBinding = useMutation({
    mutationFn: ({ connectorID, profileID, profileLabel }: { connectorID: string; profileID?: string; profileLabel?: string }) =>
      api.startCredentialLoginSession({
        connector_id: connectorID,
        profile_id: profileID,
        label: profileLabel?.trim() || label.trim() || connectorID,
      }),
    onSuccess: async (result) => {
      setActiveSession(result.session);
      setSessionDialogOpen(true);
      setVerification(null);
      await qc.invalidateQueries({ queryKey: credentialKeys.profiles(wsId) });
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t(($) => $.credential.toast_bind_failed));
    },
  });

  const revokeProfile = useMutation({
    mutationFn: (profileID: string) => api.deleteCredentialProfile(profileID),
    onSuccess: async (_profile, profileID) => {
      setVerification(null);
      if (activeSession?.profile_id === profileID) {
        setActiveSession(null);
        setSessionDialogOpen(false);
      }
      toast.success(t(($) => $.credential.toast_revoked));
      await qc.invalidateQueries({ queryKey: credentialKeys.profiles(wsId) });
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t(($) => $.credential.toast_revoke_failed));
    },
  });

  const verifyProfile = useMutation({
    mutationFn: (profileID: string) =>
      api.runCredentialCrawl({
        profile_id: profileID,
        capability: "profile_verify",
        params: {},
      }),
    onSuccess: async (result) => {
      setVerification(result);
      const authCheck = readAuthCheck(result);
      if (result.status === "completed" && authCheck?.authenticated === true) {
        toast.success(t(($) => $.credential.toast_verified));
      } else {
        toast.error(t(($) => $.credential.toast_verify_needs_reauth));
      }
      await qc.invalidateQueries({ queryKey: credentialKeys.profiles(wsId) });
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t(($) => $.credential.toast_verify_failed));
    },
  });

  const latestActiveProfile = activeProfiles[0] ?? null;
  const verifyingProfileID = verifyProfile.variables ?? "";
  const authCheck = readAuthCheck(verification);

  const handleSessionDialogOpenChange = (open: boolean) => {
    setSessionDialogOpen(open);
    if (!open) {
      closeRemoteBrowser(activeSession);
    }
  };

  useEffect(() => {
    if (selectedConnector && selectedConnector.id !== selectedConnectorID) {
      setSelectedConnectorID(selectedConnector.id);
    }
  }, [selectedConnector, selectedConnectorID]);

  useEffect(() => {
    if (!activeSession) {
      return;
    }
    let expectedOrigin = "";
    try {
      expectedOrigin = new URL(activeSession.browser_url).origin;
    } catch {
      expectedOrigin = "";
    }

    const handleMessage = (event: MessageEvent) => {
      if (expectedOrigin && event.origin !== expectedOrigin) {
        return;
      }
      const data = event.data as { type?: string; profile_id?: string };
      if (data?.type !== "multica:credential-session-completed") {
        return;
      }
      if (data.profile_id && data.profile_id !== activeSession.profile_id) {
        return;
      }
      setSessionDialogOpen(false);
      setActiveSession(null);
      toast.success(t(($) => $.credential.toast_bound));
      void qc.invalidateQueries({ queryKey: credentialKeys.profiles(wsId) });
    };

    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [activeSession, qc, t, wsId]);

  useEffect(() => {
    if (!activeSession || pendingProfile?.status !== "active") {
      return;
    }
    setSessionDialogOpen(false);
    setActiveSession(null);
  }, [activeSession, pendingProfile?.status]);

  return (
    <div className="space-y-6">
      <Card>
        <CardContent className="space-y-5">
          <div className="flex items-start justify-between gap-4">
            <div className="flex items-start gap-3">
              <div className="rounded-md border bg-muted/50 p-2 text-muted-foreground">
                <KeyRound className="h-4 w-4" />
              </div>
              <div className="space-y-1">
                <h3 className="text-sm font-semibold">{t(($) => $.credential.connection_title)}</h3>
                <p className="text-sm text-muted-foreground">
                  {t(($) => $.credential.connection_description)}
                </p>
              </div>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => profilesQuery.refetch()}
              disabled={profilesQuery.isFetching}
            >
              <RefreshCw className={cn("h-4 w-4", profilesQuery.isFetching && "animate-spin")} />
              {t(($) => $.credential.refresh)}
            </Button>
          </div>

          {canManage && <div className="grid gap-3 md:grid-cols-[minmax(180px,0.45fr)_minmax(0,1fr)_auto] md:items-end">
            <div className="space-y-2">
              <Label htmlFor="credential-connector">
                {t(($) => $.credential.connector_label)}
              </Label>
              <NativeSelect
                id="credential-connector"
                value={selectedConnector?.id ?? ""}
                onChange={(event) => setSelectedConnectorID(event.target.value)}
                disabled={connectors.length === 0}
              >
                {connectors.map((connector) => (
                  <NativeSelectOption key={connector.id} value={connector.id}>
                    {connector.display_name}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <div className="space-y-2">
              <Label htmlFor="credential-profile-label">
                {t(($) => $.credential.profile_label)}
              </Label>
              <Input
                id="credential-profile-label"
                value={label}
                onChange={(event) => setLabel(event.target.value)}
                placeholder={t(($) => $.credential.profile_label_placeholder)}
              />
            </div>
            <Button
              onClick={() => selectedConnector && startBinding.mutate({ connectorID: selectedConnector.id, profileLabel: label })}
              disabled={startBinding.isPending || connectorsQuery.isLoading || !selectedConnector}
            >
              <MonitorUp className="h-4 w-4" />
              {startBinding.isPending
                ? t(($) => $.credential.binding)
                : t(($) => $.credential.bind_connector, { name: selectedConnector?.display_name ?? "" })}
            </Button>
          </div>
          }

          {!canManage && (
            <p className="text-xs text-muted-foreground">{t(($) => $.credential.manage_hint)}</p>
          )}

          {!connectorsQuery.isLoading && connectors.length === 0 && (
            <p className="text-xs text-destructive">{t(($) => $.credential.connector_missing)}</p>
          )}

          {activeSession && (
            <div className="rounded-md border bg-muted/30 p-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <p className="text-sm font-medium">{t(($) => $.credential.session_title)}</p>
                    <StatusBadge status={pendingProfile?.status ?? activeSession.status} />
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {t(($) => $.credential.session_expires, {
                      when: formatDate(activeSession.expires_at),
                    })}
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setSessionDialogOpen(true)}
                  >
                    <MonitorUp className="h-4 w-4" />
                    {t(($) => $.credential.open_panel)}
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => window.open(activeSession.browser_url, "_blank", "noopener")}
                  >
                    <ExternalLink className="h-4 w-4" />
                    {t(($) => $.credential.open_new_tab)}
                  </Button>
                </div>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <section className="space-y-3">
        <div className="flex items-center justify-between gap-3">
          <h3 className="text-sm font-semibold">{t(($) => $.credential.profiles_title)}</h3>
          {latestActiveProfile && canManage && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => verifyProfile.mutate(latestActiveProfile.id)}
              disabled={verifyProfile.isPending}
            >
              <ShieldCheck className="h-4 w-4" />
              {verifyProfile.isPending
                ? t(($) => $.credential.verifying)
                : t(($) => $.credential.verify_latest)}
            </Button>
          )}
        </div>

        {profilesQuery.isLoading ? (
          <Card>
            <CardContent>
              <p className="text-sm text-muted-foreground">{t(($) => $.credential.loading)}</p>
            </CardContent>
          </Card>
        ) : profiles.length === 0 ? (
          <Card>
            <CardContent>
              <p className="text-sm text-muted-foreground">{t(($) => $.credential.empty_profiles)}</p>
            </CardContent>
          </Card>
        ) : (
          <Card>
            <CardContent className="divide-y">
              {profiles.map((profile) => (
                <ProfileRow
                  key={profile.id}
                  profile={profile}
                  verifying={verifyProfile.isPending && verifyingProfileID === profile.id}
                  binding={startBinding.isPending}
                  revoking={revokeProfile.isPending && revokeProfile.variables === profile.id}
                  canManage={canManage}
                  onVerify={() => verifyProfile.mutate(profile.id)}
                  onRebind={() =>
                    startBinding.mutate({
                      connectorID: profile.connector_id,
                      profileID: profile.id,
                      profileLabel: profile.label || profile.connector_id,
                    })}
                  onRevoke={() => revokeProfile.mutate(profile.id)}
                />
              ))}
            </CardContent>
          </Card>
        )}
      </section>

      {verification && (
        <Card>
          <CardContent className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="space-y-1">
                <h3 className="text-sm font-semibold">{t(($) => $.credential.verification_title)}</h3>
                <p className="text-xs text-muted-foreground">
                  {verification.message || t(($) => $.credential.verification_no_message)}
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <StatusBadge status={verification.status} />
                {authCheck && (
                  <Badge variant={authCheck.authenticated ? "default" : "destructive"}>
                    {authCheck.authenticated
                      ? t(($) => $.credential.authenticated)
                      : t(($) => $.credential.not_authenticated)}
                  </Badge>
                )}
              </div>
            </div>
            {authCheck && (
              <div className="grid gap-2 text-xs md:grid-cols-2">
                <Metric label={t(($) => $.credential.auth_user_id)} value={yesNo(authCheck.user_id_present)} />
                <Metric label={t(($) => $.credential.auth_http_status)} value={String(authCheck.http_status ?? "")} />
                <Metric label={t(($) => $.credential.auth_method)} value={String(authCheck.method ?? "")} />
                <Metric label={t(($) => $.credential.auth_observed_at)} value={formatDate(String(authCheck.observed_at ?? ""))} />
              </div>
            )}
            <pre className="max-h-72 overflow-auto rounded-md border bg-muted/40 p-3 text-xs">
              {JSON.stringify(verification.raw ?? verification, null, 2)}
            </pre>
          </CardContent>
        </Card>
      )}

      <Dialog open={sessionDialogOpen} onOpenChange={handleSessionDialogOpenChange}>
        <DialogContent
          showCloseButton={false}
          className="!h-[calc(100vh-24px)] !w-[calc(100vw-24px)] !max-w-none overflow-hidden !rounded-lg !p-0"
        >
          <div className="flex h-full min-h-0 flex-col">
            <DialogHeader className="flex-row items-center justify-between gap-3 border-b bg-background px-3 py-2">
              <div className="min-w-0 space-y-1">
                <DialogTitle className="truncate text-sm">
                  {t(($) => $.credential.remote_browser_title)}
                </DialogTitle>
                <DialogDescription className="sr-only">
                  {t(($) => $.credential.remote_browser_description)}
                </DialogDescription>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                {activeSession && (
                  <Button
                    variant="outline"
                    size="icon-sm"
                    onClick={() => window.open(activeSession.browser_url, "_blank", "noopener")}
                  >
                    <ExternalLink className="h-4 w-4" />
                    <span className="sr-only">{t(($) => $.credential.open_new_tab)}</span>
                  </Button>
                )}
                <DialogClose
                  render={
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => closeRemoteBrowser(activeSession)}
                    />
                  }
                >
                  <X className="h-4 w-4" />
                  <span className="sr-only">Close</span>
                </DialogClose>
              </div>
            </DialogHeader>
            <div className="min-h-0 flex-1 bg-black">
              {activeSession && (
                <iframe
                  title={t(($) => $.credential.remote_browser_title)}
                  src={activeSession.browser_url}
                  allow="clipboard-read; clipboard-write"
                  className="h-full w-full border-0"
                />
              )}
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function closeRemoteBrowser(session: CredentialLoginSession | null) {
  if (!session?.browser_url || typeof window === "undefined") {
    return;
  }
  let closeURL: URL;
  try {
    closeURL = new URL(session.browser_url, window.location.href);
  } catch {
    return;
  }
  closeURL.pathname = `${closeURL.pathname.replace(/\/$/, "")}/close`;
  closeURL.search = "";
  fetch(closeURL.toString(), {
    method: "POST",
    body: "{}",
    headers: { "content-type": "application/json" },
    keepalive: true,
  }).catch(() => {});
}

function ProfileRow({
  profile,
  verifying,
  binding,
  revoking,
  canManage,
  onVerify,
  onRebind,
  onRevoke,
}: {
  profile: CredentialProfile;
  verifying: boolean;
  binding: boolean;
  revoking: boolean;
  canManage: boolean;
  onVerify: () => void;
  onRebind: () => void;
  onRevoke: () => void;
}) {
  const { t } = useT("settings");
  const active = profile.status === "active";
  const needsReauth = profile.status === "need_reauth";
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 py-3 first:pt-0 last:pb-0">
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="truncate text-sm font-medium">{profile.label || profile.connector_id}</p>
          <StatusBadge status={profile.status} />
        </div>
        <p className="text-xs text-muted-foreground">
          {profile.connector_id} · {t(($) => $.credential.updated_at, {
            when: formatDate(profile.updated_at),
          })}
          {profile.last_used_at
            ? ` · ${t(($) => $.credential.last_used_at, {
              when: formatDate(profile.last_used_at),
            })}`
            : ""}
        </p>
      </div>
      {canManage && <div className="flex items-center gap-2">
        {needsReauth && (
          <Button
            variant="default"
            size="sm"
            onClick={onRebind}
            disabled={binding}
          >
            <MonitorUp className="h-4 w-4" />
            {binding ? t(($) => $.credential.binding) : t(($) => $.credential.rebind)}
          </Button>
        )}
        <Button
          variant="outline"
          size="sm"
          onClick={onVerify}
          disabled={!active || verifying}
        >
          <ShieldCheck className="h-4 w-4" />
          {verifying ? t(($) => $.credential.verifying) : t(($) => $.credential.verify)}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={onRevoke}
          disabled={revoking}
        >
          <Trash2 className="h-4 w-4" />
          {revoking ? t(($) => $.credential.revoking) : t(($) => $.credential.revoke)}
        </Button>
      </div>
      }
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const variant = status === "active"
    ? "default"
    : status === "need_reauth" || status === "revoked"
      ? "destructive"
      : "secondary";
  return <Badge variant={variant}>{status || "unknown"}</Badge>;
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border bg-background px-3 py-2">
      <p className="text-[10px] uppercase text-muted-foreground">{label}</p>
      <p className="truncate text-sm font-medium">{value || "-"}</p>
    </div>
  );
}

function readAuthCheck(result: CredentialCrawlResult | null) {
  const raw = asRecord(result?.raw);
  const probe = asRecord(raw?.auth_probe);
  return asRecord(probe?.auth_check) as {
    authenticated?: boolean;
    user_id_present?: boolean;
    http_status?: number;
    method?: string;
    observed_at?: string;
  } | null;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" ? value as Record<string, unknown> : null;
}

function yesNo(value: unknown): string {
  return value === true ? "true" : value === false ? "false" : "";
}

function formatDate(value: string): string {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}
