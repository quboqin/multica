export type PreviewSessionPlatform =
  | "web"
  | "desktop"
  | "android"
  | "ios"
  | (string & Record<never, never>);

export type PreviewSessionStatus =
  | "creating"
  | "starting"
  | "running"
  | "sleeping"
  | "stopping"
  | "stopped"
  | "failed"
  | "expired"
  | (string & Record<never, never>);

export interface PreviewSession {
  id: string;
  workspaceId: string;
  issueId: string;
  taskId: string | null;
  platform: PreviewSessionPlatform;
  provider: string;
  title: string;
  previewUrl: string;
  status: PreviewSessionStatus;
  creatorType: string;
  creatorId: string;
  errorMessage: string | null;
  expiresAt: string | null;
  lastActiveAt: string | null;
  leaseExpiresAt: string | null;
  startedAt: string | null;
  stoppedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface PreviewSessionListResponse {
  previewSessions: PreviewSession[];
  total: number;
}

export interface CreatePreviewSessionRequest {
  title?: string;
  previewUrl: string;
  platform?: PreviewSessionPlatform;
  provider?: string;
  expiresAt?: string | null;
}
