"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ChevronRight,
  Globe2,
  History,
  LoaderCircle,
  Maximize2,
  MessageSquareText,
  Minimize2,
  Monitor,
  PanelsTopLeft,
  RefreshCw,
  Smartphone,
  Usb,
  X,
} from "lucide-react";
import type {
  PreviewSession,
  PreviewSessionListResponse,
} from "@multica/core/types";
import { api } from "@multica/core/api";
import {
  previewSessionKeys,
  previewSessionListOptions,
} from "@multica/core/preview-sessions";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { cn } from "@multica/ui/lib/utils";
import { useT, useTimeAgo } from "../../i18n";

interface PreviewSessionsSectionProps {
  issueId: string;
  onFeedback?: (session: PreviewSession) => void;
}

interface RuntimeDevice {
  serial: string;
  state: string;
  model?: string;
  kind: string;
  health?: string;
  reason?: string;
  battery_level?: number;
  charging?: boolean;
  temperature_c?: number;
}

interface PendingDeviceDeploy {
  resolve: () => void;
  reject: (error: Error) => void;
  timeout: number;
  target: Window;
  origin: string;
}

type DeploymentPhase = "queued" | "preparing" | "installing" | "starting" | "connecting";

const DEPLOYMENT_PHASES = new Set<string>([
  "queued", "preparing", "installing", "starting", "connecting",
]);

const STATUS_ORDER: Record<string, number> = {
  running: 0,
  sleeping: 1,
  creating: 1,
  starting: 1,
  stopping: 1,
  failed: 2,
  stopped: 2,
  expired: 2,
};

function createdAtMillis(value: string): number {
  const timestamp = Date.parse(value);
  return Number.isNaN(timestamp) ? 0 : timestamp;
}

function effectivePreviewStatus(
  session: PreviewSession,
  nowMs: number,
): PreviewSession["status"] {
  const expiresAt = session.expiresAt
    ? Date.parse(session.expiresAt)
    : Number.NaN;
  if (
    isActivePreviewStatus(session.status) &&
    !Number.isNaN(expiresAt) &&
    expiresAt <= nowMs
  ) {
    return "expired";
  }
  return session.status;
}

function isActivePreviewStatus(status: string): boolean {
  return (
    status === "creating" ||
    status === "starting" ||
    status === "running" ||
    status === "sleeping" ||
    status === "stopping"
  );
}

function sortPreviewSessions(
  sessions: PreviewSession[],
  nowMs: number,
): PreviewSession[] {
  return sessions.toSorted((a, b) => {
    const statusDiff =
      (STATUS_ORDER[effectivePreviewStatus(a, nowMs)] ?? 3) -
      (STATUS_ORDER[effectivePreviewStatus(b, nowMs)] ?? 3);
    if (statusDiff !== 0) return statusDiff;
    return createdAtMillis(b.createdAt) - createdAtMillis(a.createdAt);
  });
}

function safeHttpUrl(value: string): URL | null {
  try {
    const url = new URL(value);
    if (url.protocol !== "http:" && url.protocol !== "https:") return null;
    if (url.username || url.password) return null;
    return url;
  } catch {
    return null;
  }
}

function previewEmbedUrl(session: PreviewSession): string | null {
  const url = safeHttpUrl(session.previewUrl);
  if (!url) return null;

  if (session.platform === "android" && session.provider === "local_device") {
    url.searchParams.set("embed", "1");
    url.searchParams.set("session_id", session.id);
  }
  return url.toString();
}

function deviceBindingLabel(session: PreviewSession): string | null {
  if (session.platform !== "android" || session.provider !== "local_device") {
    return null;
  }
  const url = safeHttpUrl(session.previewUrl);
  const serial = url?.searchParams.get("serial")?.trim();
  return serial || null;
}

function PlatformIcon({ platform }: { platform: string }) {
  switch (platform) {
    case "web":
      return <Globe2 aria-hidden="true" />;
    case "desktop":
      return <Monitor aria-hidden="true" />;
    case "android":
    case "ios":
      return <Smartphone aria-hidden="true" />;
    default:
      return <PanelsTopLeft aria-hidden="true" />;
  }
}

function statusDotClass(status: string): string {
  switch (status) {
    case "running":
      return "bg-success";
    case "creating":
    case "starting":
      return "bg-warning";
    case "failed":
      return "bg-destructive";
    default:
      return "bg-muted-foreground/60";
  }
}

function usePreviewStatusLabel(status: string): string {
  const { t } = useT("issues");
  switch (status) {
    case "creating":
      return t(($) => $.preview_sessions.status.creating);
    case "starting":
      return t(($) => $.preview_sessions.status.starting);
    case "running":
      return t(($) => $.preview_sessions.status.running);
    case "sleeping":
      return t(($) => $.preview_sessions.status.sleeping);
    case "stopping":
      return t(($) => $.preview_sessions.status.stopping);
    case "stopped":
      return t(($) => $.preview_sessions.status.stopped);
    case "failed":
      return t(($) => $.preview_sessions.status.failed);
    case "expired":
      return t(($) => $.preview_sessions.status.expired);
    default:
      return t(($) => $.preview_sessions.status.unknown);
  }
}

interface PrimaryPreviewProps {
  session: PreviewSession;
  nowMs: number;
  onOpen: (session: PreviewSession) => void;
}

function PrimaryPreview({ session, nowMs, onOpen }: PrimaryPreviewProps) {
  const { t } = useT("issues");
  const status = effectivePreviewStatus(session, nowMs);
  const statusLabel = usePreviewStatusLabel(status);
  const parsedUrl = safeHttpUrl(session.previewUrl);
  const bindingLabel = deviceBindingLabel(session);
  const canOpen = (status === "running" || status === "sleeping") && parsedUrl !== null;

  return (
    <button
      type="button"
      data-testid="primary-preview"
      className={cn(
        "group/primary-preview flex min-h-14 w-full min-w-0 items-center gap-2.5 rounded-lg border bg-card px-2.5 py-2 text-left transition-colors",
        canOpen
          ? "hover:border-foreground/20 hover:bg-accent/20"
          : "cursor-default text-muted-foreground",
      )}
      disabled={!canOpen}
      onClick={() => canOpen && onOpen(session)}
      aria-label={
        canOpen
          ? t(($) => $.preview_sessions.open)
          : t(($) => $.preview_sessions.unavailable)
      }
    >
      <span className="relative flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground [&>svg]:size-3.5">
        <PlatformIcon platform={session.platform} />
        <span
          aria-hidden="true"
          className={cn(
            "absolute -right-0.5 -top-0.5 size-2 rounded-full border-2 border-card",
            statusDotClass(status),
          )}
        />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-xs font-medium text-foreground">
          {session.title || t(($) => $.preview_sessions.untitled)}
        </span>
        <span className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[11px] text-muted-foreground">
          <span className="shrink-0">{statusLabel}</span>
          <span aria-hidden="true">·</span>
          <span className="truncate">
            {bindingLabel || parsedUrl?.host || t(($) => $.preview_sessions.invalid_url)}
          </span>
        </span>
        {status === "failed" && session.errorMessage && (
          <span className="mt-1 line-clamp-2 block text-[11px] leading-4 text-destructive">
            {session.errorMessage}
          </span>
        )}
      </span>
      {canOpen && (
        <Maximize2
          aria-hidden="true"
          className="size-3.5 shrink-0 text-muted-foreground transition-colors group-hover/primary-preview:text-foreground"
        />
      )}
    </button>
  );
}

interface SecondaryPreviewProps {
  session: PreviewSession;
  nowMs: number;
  onOpen: (session: PreviewSession) => void;
}

function SecondaryPreview({ session, nowMs, onOpen }: SecondaryPreviewProps) {
  const { t } = useT("issues");
  const timeAgo = useTimeAgo();
  const status = effectivePreviewStatus(session, nowMs);
  const statusLabel = usePreviewStatusLabel(status);
  const canOpen =
    (status === "running" || status === "sleeping") &&
    previewEmbedUrl(session) !== null;
  const createdLabel = createdAtMillis(session.createdAt)
    ? timeAgo(session.createdAt)
    : null;

  return (
    <button
      type="button"
      className={cn(
        "flex w-full min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors",
        canOpen ? "hover:bg-accent/50" : "cursor-default text-muted-foreground",
      )}
      disabled={!canOpen}
      onClick={() => canOpen && onOpen(session)}
      aria-label={
        canOpen
          ? t(($) => $.preview_sessions.open_named, {
              title: session.title || t(($) => $.preview_sessions.untitled),
            })
          : undefined
      }
    >
      <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-muted/70 text-muted-foreground [&>svg]:size-3.5">
        <PlatformIcon platform={session.platform} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-xs font-medium">
          {session.title || t(($) => $.preview_sessions.untitled)}
        </span>
        <span className="mt-0.5 flex items-center gap-1 text-[11px] text-muted-foreground">
          <span
            aria-hidden="true"
            className={cn("size-1.5 rounded-full", statusDotClass(status))}
          />
          <span>{statusLabel}</span>
          {createdLabel && (
            <>
              <span aria-hidden="true">·</span>
              <span>{createdLabel}</span>
            </>
          )}
        </span>
      </span>
      {canOpen && <Maximize2 aria-hidden="true" className="size-3.5 shrink-0" />}
    </button>
  );
}

interface PreviewViewerProps {
  session: PreviewSession | null;
  onClose: () => void;
  onHeartbeat: (session: PreviewSession) => Promise<void>;
  onSwitchDevice: (
    session: PreviewSession,
    serial: string,
    deploy: () => Promise<void>,
  ) => Promise<void>;
  onFeedback?: (session: PreviewSession) => void;
}

function PreviewViewer({
  session,
  onClose,
  onHeartbeat,
  onSwitchDevice,
  onFeedback,
}: PreviewViewerProps) {
  const { t } = useT("issues");
  const [retainedSession, setRetainedSession] = useState(session);
  const [frameRevision, setFrameRevision] = useState(0);
  const [isFrameLoading, setIsFrameLoading] = useState(true);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [runtimeDevices, setRuntimeDevices] = useState<RuntimeDevice[]>([]);
  const [switchingSerial, setSwitchingSerial] = useState<string | null>(null);
  const viewerRef = useRef<HTMLDivElement>(null);
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [deploymentPhase, setDeploymentPhase] = useState<DeploymentPhase | null>(null);
  const pendingDeploysRef = useRef(new Map<string, PendingDeviceDeploy>());
  const visibleSession = session ?? retainedSession;
  const embedUrl = visibleSession ? previewEmbedUrl(visibleSession) : null;
  const status = visibleSession
    ? effectivePreviewStatus(visibleSession, Date.now())
    : "stopped";
  const statusLabel = usePreviewStatusLabel(status);
  const bindingLabel = visibleSession ? deviceBindingLabel(visibleSession) : null;
  const runtimeOrigin = visibleSession
    ? safeHttpUrl(visibleSession.previewUrl)?.origin ?? null
    : null;
  const isLocalAndroid =
    visibleSession?.platform === "android" &&
    visibleSession.provider === "local_device";
  const currentIsEmulator = bindingLabel?.startsWith("emulator-") ?? false;
  const emulatorDevice =
    runtimeDevices.find((device) => device.kind === "emulator") ??
    (bindingLabel && currentIsEmulator
      ? { serial: bindingLabel, state: "disconnected", kind: "emulator" }
      : undefined);
  const physicalDevice =
    runtimeDevices.find((device) => device.kind === "physical") ??
    (bindingLabel && !currentIsEmulator
      ? { serial: bindingLabel, state: "disconnected", kind: "physical" }
      : undefined);
  const showDeviceSwitcher =
    isLocalAndroid && emulatorDevice !== undefined && physicalDevice !== undefined;

  const deploymentLabel = deploymentPhase
    ? {
        queued: t(($) => $.preview_sessions.deployment.queued),
        preparing: t(($) => $.preview_sessions.deployment.preparing),
        installing: t(($) => $.preview_sessions.deployment.installing),
        starting: t(($) => $.preview_sessions.deployment.starting),
        connecting: t(($) => $.preview_sessions.deployment.connecting),
      }[deploymentPhase]
    : null;
  const deviceTitle = (device: RuntimeDevice) =>
    [device.model || device.serial, device.reason, typeof device.battery_level === "number" ? `${device.battery_level}%` : null,
      typeof device.temperature_c === "number" ? `${device.temperature_c.toFixed(1)} C` : null].filter(Boolean).join(" · ");
  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(document.fullscreenElement === viewerRef.current);
    };
    document.addEventListener("fullscreenchange", handleFullscreenChange);
    return () =>
      document.removeEventListener("fullscreenchange", handleFullscreenChange);
  }, []);

  useEffect(() => {
    if (session) {
      const sessionChanged = retainedSession?.id !== session.id;
      setRetainedSession(session);
      if (sessionChanged) setIsFrameLoading(true);
    }
  }, [retainedSession?.id, session]);

  useEffect(() => {
    setIsFrameLoading(true);
  }, [frameRevision]);

  useEffect(() => {
    if (!session || session.provider !== "local_device") return;
    const timer = window.setInterval(() => {
      void onHeartbeat(session);
    }, 60_000);
    return () => window.clearInterval(timer);
  }, [onHeartbeat, session]);

  useEffect(() => {
    if (!runtimeOrigin || !isLocalAndroid) {
      setRuntimeDevices([]);
      return;
    }
    const handleRuntimeMessage = (event: MessageEvent<unknown>) => {
      if (
        event.origin !== runtimeOrigin ||
        event.source !== iframeRef.current?.contentWindow ||
        typeof event.data !== "object" ||
        event.data === null
      ) {
        return;
      }
      const payload = event.data as {
        type?: unknown;
        devices?: unknown;
        request_id?: unknown;
        ok?: unknown;
        error?: unknown;
        phase?: unknown;
        status?: unknown;
        install_skipped?: unknown;
      };
      if (payload.type === "multica:device-inventory") {
        const devices = Array.isArray(payload.devices)
          ? payload.devices.filter(
              (device): device is RuntimeDevice =>
                typeof device === "object" &&
                device !== null &&
                typeof device.serial === "string" &&
                typeof device.state === "string" &&
                typeof device.kind === "string",
            )
          : [];
        setRuntimeDevices(devices);
        return;
      }
      if (
        payload.type === "multica:device-deploy-progress" &&
        typeof payload.request_id === "string" &&
        typeof payload.phase === "string" &&
        DEPLOYMENT_PHASES.has(payload.phase) &&
        pendingDeploysRef.current.has(payload.request_id)
      ) {
        setDeploymentPhase(payload.phase as DeploymentPhase);
        return;
      }
      if (
        payload.type !== "multica:device-deploy-result" ||
        typeof payload.request_id !== "string"
      ) {
        return;
      }
      const pending = pendingDeploysRef.current.get(payload.request_id);
      if (!pending) return;
      setDeploymentPhase(null);
      window.clearTimeout(pending.timeout);
      pendingDeploysRef.current.delete(payload.request_id);
      if (payload.ok === true) {
        pending.resolve();
      } else {
        pending.reject(
          new Error(
            typeof payload.error === "string"
              ? payload.error
              : t(($) => $.preview_sessions.device_switch_failed),
          ),
        );
      }
    };
    window.addEventListener("message", handleRuntimeMessage);
    return () => window.removeEventListener("message", handleRuntimeMessage);
  }, [isLocalAndroid, runtimeOrigin, t]);

  useEffect(
    () => () => {
      for (const [requestId, pending] of pendingDeploysRef.current) {
        window.clearTimeout(pending.timeout);
        pending.target.postMessage(
          { type: "multica:device-deploy-cancel", request_id: requestId },
          pending.origin,
        );
        pending.reject(new Error(t(($) => $.preview_sessions.device_switch_failed)));
      }
      pendingDeploysRef.current.clear();
      setDeploymentPhase(null);
    },
    [t],
  );

  const requestRuntimeDeploy = useCallback(
    (serial: string) =>
      new Promise<void>((resolve, reject) => {
        const target = iframeRef.current?.contentWindow;
        if (!target || !runtimeOrigin) {
          reject(new Error(t(($) => $.preview_sessions.device_switch_failed)));
          return;
        }
        const requestId = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
        setDeploymentPhase("queued");
        const timeout = window.setTimeout(() => {
          pendingDeploysRef.current.delete(requestId);
          target.postMessage(
            { type: "multica:device-deploy-cancel", request_id: requestId },
            runtimeOrigin,
          );
          setDeploymentPhase(null);
          reject(new Error(t(($) => $.preview_sessions.device_switch_failed)));
        }, 130_000);
        pendingDeploysRef.current.set(requestId, { resolve, reject, timeout, target, origin: runtimeOrigin });
        target.postMessage(
          {
            type: "multica:device-deploy",
            request_id: requestId,
            serial,
          },
          runtimeOrigin,
        );
      }),
    [runtimeOrigin, t],
  );

  const handleDeviceSwitch = useCallback(
    async (device: RuntimeDevice) => {
      if (
        !visibleSession ||
        device.state !== "device" ||
        device.serial === bindingLabel ||
        switchingSerial !== null
      ) {
        return;
      }
      setSwitchingSerial(device.serial);
      try {
        await onSwitchDevice(visibleSession, device.serial, () =>
          requestRuntimeDeploy(device.serial),
        );
      } catch (error) {
        toast.error(
          error instanceof Error && error.message
            ? error.message
            : t(($) => $.preview_sessions.device_switch_failed),
        );
      } finally {
        setSwitchingSerial(null);
        setDeploymentPhase(null);
      }
    },
    [bindingLabel, onSwitchDevice, requestRuntimeDeploy, switchingSerial, t, visibleSession],
  );

  const closeViewer = useCallback(() => {
    if (document.fullscreenElement === viewerRef.current && document.exitFullscreen) {
      void document.exitFullscreen().finally(onClose);
      return;
    }
    onClose();
  }, [onClose]);

  const toggleFullscreen = useCallback(async () => {
    if (document.fullscreenElement === viewerRef.current) {
      await document.exitFullscreen?.();
      return;
    }
    await viewerRef.current?.requestFullscreen?.();
  }, []);

  const handleFeedback = useCallback(() => {
    if (!visibleSession || !onFeedback) return;
    onFeedback(visibleSession);
    closeViewer();
  }, [closeViewer, onFeedback, visibleSession]);

  if (!visibleSession) return null;

  return (
    <Dialog open={session !== null} onOpenChange={(open) => !open && closeViewer()}>
      <DialogContent
        keepMounted
        showCloseButton={false}
        initialFocus={false}
        className={cn(
          "h-[calc(100dvh-1rem)] w-[calc(100vw-1rem)] !max-w-none gap-0 overflow-hidden rounded-lg p-0 duration-0 data-open:animate-none data-closed:animate-none sm:h-[min(90dvh,900px)] sm:!max-w-none",
          visibleSession.platform === "android" || visibleSession.platform === "ios"
            ? "sm:w-[min(90vw,760px)]"
            : "sm:w-[min(94vw,1440px)]",
        )}
        data-testid="preview-viewer"
      >
        {visibleSession && (
          <div
            ref={viewerRef}
            className="flex size-full min-h-0 flex-col overflow-hidden bg-background"
          >
            <header className="flex h-12 shrink-0 items-center gap-2 border-b px-2.5 sm:px-3">
              <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground [&>svg]:size-3.5">
                <PlatformIcon platform={visibleSession.platform} />
              </span>
              <DialogTitle className="min-w-0 flex-1 truncate text-sm">
                {visibleSession.title || t(($) => $.preview_sessions.untitled)}
              </DialogTitle>
              {deploymentLabel && (
                <span className="hidden shrink-0 items-center gap-1.5 text-[11px] text-muted-foreground sm:inline-flex" aria-live="polite">
                  <LoaderCircle aria-hidden="true" className="size-3 animate-spin" />
                  {deploymentLabel}
                </span>
              )}
              {showDeviceSwitcher ? (
                <div
                  role="group"
                  aria-label={t(($) => $.preview_sessions.device_source)}
                  className="flex shrink-0 items-center rounded-md bg-muted p-0.5"
                >
                  {[emulatorDevice, physicalDevice].map((device) => {
                    const selected = device.serial === bindingLabel;
                    const isSwitching = device.serial === switchingSerial;
                    const isEmulator = device.kind === "emulator";
                    return (
                      <Button
                        key={device.serial}
                        type="button"
                        variant={selected ? "secondary" : "ghost"}
                        size="xs"
                        className="h-6 gap-1 rounded-sm px-1.5 text-[11px]"
                        aria-pressed={selected}
                        aria-label={
                          isEmulator
                            ? t(($) => $.preview_sessions.use_emulator)
                            : t(($) => $.preview_sessions.use_usb_device)
                        }
                        title={deviceTitle(device)}
                        disabled={
                          switchingSerial !== null ||
                          selected ||
                          device.state !== "device" ||
                          device.health === "unavailable"
                        }
                        onClick={() => void handleDeviceSwitch(device)}
                      >
                        {isSwitching ? (
                          <LoaderCircle className="animate-spin" />
                        ) : isEmulator ? (
                          <Monitor />
                        ) : (
                          <Usb />
                        )}
                        <span className="hidden md:inline">
                          {isEmulator
                            ? t(($) => $.preview_sessions.emulator)
                            : t(($) => $.preview_sessions.usb_device)}
                        </span>
                      </Button>
                    );
                  })}
                </div>
              ) : bindingLabel ? (
                <span className="hidden max-w-64 truncate text-xs text-muted-foreground md:block">
                  {bindingLabel}
                </span>
              ) : null}
              <span className="hidden min-w-0 items-center gap-1.5 text-xs text-muted-foreground sm:flex">
                <span
                  aria-hidden="true"
                  className={cn("size-1.5 rounded-full", statusDotClass(status))}
                />
                <span>{statusLabel}</span>
              </span>
              <div className="ml-1 flex shrink-0 items-center gap-0.5">
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t(($) => $.preview_sessions.refresh)}
                        onClick={() => setFrameRevision((value) => value + 1)}
                      >
                        <RefreshCw />
                      </Button>
                    }
                  />
                  <TooltipContent>
                    {t(($) => $.preview_sessions.refresh)}
                  </TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        aria-label={
                          isFullscreen
                            ? t(($) => $.preview_sessions.exit_fullscreen)
                            : t(($) => $.preview_sessions.fullscreen)
                        }
                        onClick={() => void toggleFullscreen()}
                      >
                        {isFullscreen ? <Minimize2 /> : <Maximize2 />}
                      </Button>
                    }
                  />
                  <TooltipContent>
                    {isFullscreen
                      ? t(($) => $.preview_sessions.exit_fullscreen)
                      : t(($) => $.preview_sessions.fullscreen)}
                  </TooltipContent>
                </Tooltip>
                {onFeedback && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="gap-1.5"
                    onClick={handleFeedback}
                  >
                    <MessageSquareText />
                    <span className="hidden sm:inline">
                      {t(($) => $.preview_sessions.feedback)}
                    </span>
                  </Button>
                )}
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t(($) => $.preview_sessions.close)}
                        onClick={closeViewer}
                      >
                        <X />
                      </Button>
                    }
                  />
                  <TooltipContent>
                    {t(($) => $.preview_sessions.close)}
                  </TooltipContent>
                </Tooltip>
              </div>
            </header>

            <div className="relative min-h-0 flex-1 overflow-hidden bg-muted/30">
              {embedUrl && status === "running" ? (
                <iframe
                  ref={iframeRef}
                  key={`${visibleSession.id}:${frameRevision}`}
                  title={visibleSession.title || t(($) => $.preview_sessions.untitled)}
                  src={embedUrl}
                  className="size-full border-0 bg-background"
                  allow="autoplay; camera; clipboard-read; clipboard-write; fullscreen; geolocation; microphone"
                  allowFullScreen
                  onLoad={() => setIsFrameLoading(false)}
                />
              ) : (
                <div className="flex size-full items-center justify-center px-6 text-center text-sm text-muted-foreground">
                  {t(($) => $.preview_sessions.unavailable)}
                </div>
              )}
              {embedUrl && status === "running" && isFrameLoading && (
                <div className="pointer-events-none absolute inset-0 flex items-center justify-center bg-background/80">
                  <span className="inline-flex items-center gap-2 text-xs text-muted-foreground">
                    <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />
                    {t(($) => $.preview_sessions.loading)}
                  </span>
                </div>
              )}
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

export function PreviewSessionsSection({
  issueId,
  onFeedback,
}: PreviewSessionsSectionProps) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(true);
  const [showOtherSessions, setShowOtherSessions] = useState(false);
  const [nowMs, setNowMs] = useState(() => Date.now());
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const autoOpenedIssueRef = useRef<string | null>(null);
  const query = useQuery(previewSessionListOptions(wsId, issueId));
  const sourceSessions = useMemo(() => query.data ?? [], [query.data]);

  useEffect(() => {
    const hasExpiringSession = sourceSessions.some(
      (session) =>
        isActivePreviewStatus(session.status) && session.expiresAt !== null,
    );
    if (!hasExpiringSession) return;

    const timer = window.setInterval(() => setNowMs(Date.now()), 30_000);
    return () => window.clearInterval(timer);
  }, [sourceSessions]);

  const sessions = useMemo(
    () => sortPreviewSessions(sourceSessions, nowMs),
    [sourceSessions, nowMs],
  );
  const primarySession = sessions[0] ?? null;
  const otherSessions = primarySession
    ? sessions.filter((session) => session.id !== primarySession.id)
    : [];
  const activeSession = activeSessionId
    ? sessions.find((session) => session.id === activeSessionId) ?? null
    : null;

  const cacheTouchedSession = useCallback(
    (updated: PreviewSession) => {
      queryClient.setQueryData<PreviewSessionListResponse>(
        previewSessionKeys.issue(wsId, issueId),
        (current) => current
          ? {
              ...current,
              previewSessions: current.previewSessions.map((session) =>
                session.id === updated.id ? updated : session,
              ),
            }
          : current,
      );
    },
    [issueId, queryClient, wsId],
  );

  const touchSession = useCallback(
    async (session: PreviewSession) => {
      if (session.provider !== "local_device") return;
      const updated = await api.touchPreviewSession(session.id);
      cacheTouchedSession(updated);
    },
    [cacheTouchedSession],
  );

  const showLeaseError = useCallback(
    (error: unknown) => {
      toast.error(
        error instanceof Error && error.message
          ? error.message
          : t(($) => $.preview_sessions.lease_failed),
      );
    },
    [t],
  );

  const openSession = useCallback(
    async (session: PreviewSession) => {
      try {
        await touchSession(session);
        setActiveSessionId(session.id);
      } catch (error) {
        showLeaseError(error);
      }
    },
    [showLeaseError, touchSession],
  );

  const heartbeatSession = useCallback(
    async (session: PreviewSession) => {
      try {
        await touchSession(session);
      } catch (error) {
        setActiveSessionId(null);
        showLeaseError(error);
      }
    },
    [showLeaseError, touchSession],
  );

  const switchSessionDevice = useCallback(
    async (
      session: PreviewSession,
      serial: string,
      deploy: () => Promise<void>,
    ) => {
      const previousSerial = deviceBindingLabel(session);
      if (!previousSerial || previousSerial === serial) return;
      await api.switchPreviewSessionDevice(session.id, serial, false);
      try {
        await deploy();
        const confirmed = await api.switchPreviewSessionDevice(
          session.id,
          serial,
          true,
        );
        cacheTouchedSession(confirmed);
      } catch (error) {
        try {
          const reverted = await api.switchPreviewSessionDevice(
            session.id,
            previousSerial,
            true,
          );
          cacheTouchedSession(reverted);
        } catch {
          await queryClient.invalidateQueries({
            queryKey: previewSessionKeys.issue(wsId, issueId),
          });
        }
        throw error;
      }
    },
    [cacheTouchedSession, issueId, queryClient, wsId],
  );

  useEffect(() => {
    autoOpenedIssueRef.current = null;
    setActiveSessionId(null);
    setShowOtherSessions(false);
  }, [issueId]);

  useEffect(() => {
    if (
      !primarySession ||
      autoOpenedIssueRef.current === issueId ||
      effectivePreviewStatus(primarySession, nowMs) !== "running" ||
      previewEmbedUrl(primarySession) === null
    ) {
      return;
    }

    autoOpenedIssueRef.current = issueId;
    void openSession(primarySession);
  }, [issueId, nowMs, openSession, primarySession]);

  if (query.isPending) return null;
  if (!query.isError && sessions.length === 0) return null;

  return (
    <div data-testid="preview-sessions-section">
      <button
        type="button"
        className={cn(
          "mb-2 flex w-full min-w-0 items-center gap-1 rounded-md px-2 py-1 text-xs font-medium transition-colors hover:bg-accent/70",
          !open && "text-muted-foreground hover:text-foreground",
        )}
        onClick={() => setOpen((value) => !value)}
      >
        {t(($) => $.preview_sessions.section)}
        {!query.isError && (
          <span className="font-mono text-[10px] font-normal tabular-nums text-muted-foreground">
            {sessions.length}
          </span>
        )}
        <ChevronRight
          aria-hidden="true"
          className={cn(
            "!size-3 shrink-0 stroke-[2.5] text-muted-foreground transition-transform",
            open && "rotate-90",
          )}
        />
      </button>

      {open && query.isError && (
        <div className="flex items-center gap-2 pl-2 text-xs text-muted-foreground">
          <span className="min-w-0 flex-1">
            {t(($) => $.preview_sessions.load_failed)}
          </span>
          <Button
            type="button"
            variant="ghost"
            size="xs"
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw className={cn(query.isFetching && "animate-spin")} />
            {t(($) => $.preview_sessions.retry)}
          </Button>
        </div>
      )}

      {open && !query.isError && primarySession && (
        <div className="pl-2">
          <PrimaryPreview
            session={primarySession}
            nowMs={nowMs}
            onOpen={(session) => void openSession(session)}
          />

          {otherSessions.length > 0 && (
            <div className="mt-1.5">
              <button
                type="button"
                className="flex w-full items-center gap-1.5 rounded-md px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-accent/40 hover:text-foreground"
                onClick={() => setShowOtherSessions((value) => !value)}
              >
                <History aria-hidden="true" className="size-3" />
                {t(($) => $.preview_sessions.other_sessions, {
                  count: otherSessions.length,
                })}
                <ChevronRight
                  aria-hidden="true"
                  className={cn(
                    "ml-auto size-3 transition-transform",
                    showOtherSessions && "rotate-90",
                  )}
                />
              </button>
              {showOtherSessions && (
                <div className="mt-0.5 space-y-0.5">
                  {otherSessions.map((session) => (
                    <SecondaryPreview
                      key={session.id}
                      session={session}
                      nowMs={nowMs}
                      onOpen={(selected) => void openSession(selected)}
                    />
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      )}

      <PreviewViewer
        key={issueId}
        session={activeSession}
        onClose={() => setActiveSessionId(null)}
        onHeartbeat={heartbeatSession}
        onSwitchDevice={switchSessionDevice}
        onFeedback={onFeedback}
      />
    </div>
  );
}
