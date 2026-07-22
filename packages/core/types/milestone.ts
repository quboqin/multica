export type MilestoneStatus = "planned" | "in_progress" | "paused" | "completed" | "cancelled";

export interface Milestone {
  id: string;
  workspace_id: string;
  title: string;
  description: string;
  start_date: string | null;
  end_date: string | null;
  status: MilestoneStatus;
  position: number;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface CreateMilestoneRequest {
  title: string;
  description?: string;
  start_date?: string | null;
  end_date?: string | null;
  status?: MilestoneStatus;
  position?: number;
}

export interface UpdateMilestoneRequest {
  title?: string;
  description?: string;
  start_date?: string | null;
  end_date?: string | null;
  status?: MilestoneStatus;
  position?: number;
}

export interface ListMilestonesResponse {
  milestones: Milestone[];
  total: number;
}
