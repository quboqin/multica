"use client";

import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { paths } from "@multica/core/paths";
import { workspaceKeys } from "@multica/core/workspace/queries";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@multica/ui/components/ui/card";
import { Loader2 } from "lucide-react";
import { setLoggedInCookie } from "@/features/auth/auth-cookie";

type State = { kind: "loading"; message: string } | { kind: "error"; message: string };

function safeNext(raw: string | null): string {
  if (!raw || !raw.startsWith("/") || raw.startsWith("//")) return "";
  return raw;
}

function buildRedirectUri(): string {
  return new URL("/lark/start", window.location.origin).toString();
}

function LarkStartContent() {
  const params = useSearchParams();
  const router = useRouter();
  const qc = useQueryClient();
  const installationId = params.get("installation_id") ?? "";
  const code = params.get("code") ?? "";
  const providedState = params.get("state") ?? "";
  const next = safeNext(params.get("next"));
  const [state, setState] = useState<State>({
    kind: "loading",
    message: "正在处理飞书登录...",
  });

  useEffect(() => {
    if (!installationId && !providedState) {
      setState({ kind: "error", message: "缺少 installation_id 或 state。" });
      return;
    }

    let cancelled = false;
    async function run() {
      try {
        if (!code) {
          if (!installationId) {
            setState({ kind: "error", message: "缺少飞书安装信息。" });
            return;
          }
          const resp = await api.createLarkLoginState(installationId, next, buildRedirectUri());
          if (!resp.authorize_url) {
            setState({ kind: "error", message: "无法生成飞书授权链接。" });
            return;
          }
          if (!cancelled) {
            setState({ kind: "loading", message: "正在跳转到飞书授权..." });
            window.location.assign(resp.authorize_url);
          }
          return;
        }

        if (!providedState) {
          setState({ kind: "error", message: "飞书授权回调缺少 state。" });
          return;
        }

        const resp = await api.larkLogin(code, providedState);
        api.setToken(resp.token);
        useAuthStore.getState().setUser(resp.user);
        setLoggedInCookie();
        const workspaces = await api.listWorkspaces();
        qc.setQueryData(workspaceKeys.list(), workspaces);
        const destination = resp.next || next || (resp.workspace_slug ? paths.workspace(resp.workspace_slug).root() : paths.login());
        router.replace(destination);
      } catch (err) {
        if (!cancelled) {
          setState({
            kind: "error",
            message: err instanceof Error ? err.message : "飞书登录失败。",
          });
        }
      }
    }

    void run();
    return () => {
      cancelled = true;
    };
  }, [code, installationId, next, providedState, qc, router]);

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>飞书登录</CardTitle>
          <CardDescription>使用飞书身份进入 Multica。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {state.kind === "loading" ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              {state.message}
            </div>
          ) : (
            <>
              <p className="text-sm text-destructive">{state.message}</p>
              <Button variant="outline" onClick={() => router.replace(paths.login())}>
                返回登录页
              </Button>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

export default function LarkStartPage() {
  return (
    <Suspense fallback={null}>
      <LarkStartContent />
    </Suspense>
  );
}
