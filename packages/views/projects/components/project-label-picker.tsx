"use client";

import { useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Plus, Settings2, Tag } from "lucide-react";
import { toast } from "sonner";
import { Dialog, DialogContent, DialogTitle } from "@multica/ui/components/ui/dialog";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  labelListOptions,
  projectLabelsOptions,
  useAttachProjectLabel,
  useCreateLabel,
  useDetachProjectLabel,
} from "@multica/core/labels";
import { LabelChip } from "../../labels/label-chip";
import { LabelsPanel } from "../../issues/components/labels-panel";
import {
  PickerEmpty,
  PickerItem,
  PropertyPicker,
} from "../../issues/components/pickers/property-picker";
import { useT } from "../../i18n";

const INLINE_COLORS = [
  "#ef4444", "#f97316", "#eab308", "#22c55e", "#14b8a6",
  "#3b82f6", "#6366f1", "#a855f7", "#ec4899", "#64748b",
] as const;

function pickInlineColor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    hash = (hash * 31 + name.charCodeAt(i)) >>> 0;
  }
  return INLINE_COLORS[hash % INLINE_COLORS.length] ?? INLINE_COLORS[0]!;
}

export function ProjectLabelPicker({ projectId }: { projectId: string }) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const [manageOpen, setManageOpen] = useState(false);
  const creatingRef = useRef(false);

  const { data: allLabels = [] } = useQuery(labelListOptions(wsId));
  const { data: attachedLabels = [] } = useQuery(projectLabelsOptions(wsId, projectId));
  const attach = useAttachProjectLabel(projectId);
  const detach = useDetachProjectLabel(projectId);
  const create = useCreateLabel();

  const attachedIds = useMemo(
    () => new Set(attachedLabels.map((label) => label.id)),
    [attachedLabels],
  );
  const query = filter.trim();
  const queryLower = query.toLowerCase();
  const filtered = allLabels.filter((label) => label.name.toLowerCase().includes(queryLower));
  const exactMatch = allLabels.some((label) => label.name.toLowerCase() === queryLower);
  const canCreate = query.length > 0 && !exactMatch && !create.isPending;

  const toggle = (labelId: string) => {
    if (attachedIds.has(labelId)) detach.mutate(labelId);
    else attach.mutate(labelId);
  };

  const createAndAttach = () => {
    if (!canCreate || creatingRef.current) return;
    creatingRef.current = true;
    const name = query;
    create.mutate(
      { name, color: pickInlineColor(name) },
      {
        onSuccess: (label) => {
          attach.mutate(label.id);
          setFilter("");
        },
        onError: (err: unknown) => {
          toast.error(err instanceof Error ? err.message : t(($) => $.pickers.label.create_failed));
        },
        onSettled: () => {
          creatingRef.current = false;
        },
      },
    );
  };

  const hasLabels = attachedLabels.length > 0;

  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <PropertyPicker
        open={open}
        onOpenChange={(nextOpen) => {
          setOpen(nextOpen);
          if (!nextOpen) setFilter("");
        }}
        width="w-80"
        align="start"
        searchable
        searchPlaceholder={t(($) => $.pickers.label.search_placeholder)}
        onSearchChange={setFilter}
        triggerRender={
          hasLabels ? (
            <div className="flex min-w-0 flex-wrap items-center gap-1 cursor-pointer rounded px-1 -mx-1 hover:bg-accent/30 transition-colors" />
          ) : undefined
        }
        trigger={
          hasLabels ? (
            <>
              {attachedLabels.map((label) => (
                <LabelChip
                  key={label.id}
                  label={label}
                  onRemove={() => detach.mutate(label.id)}
                />
              ))}
            </>
          ) : (
            <>
              <Tag className="h-3.5 w-3.5 text-muted-foreground" />
              <span className="text-muted-foreground">{t(($) => $.pickers.label.trigger_label)}</span>
            </>
          )
        }
        footer={
          <button
            type="button"
            onClick={() => {
              setOpen(false);
              setManageOpen(true);
            }}
            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent transition-colors"
          >
            <Settings2 className="h-3.5 w-3.5" />
            <span>{t(($) => $.pickers.label.manage_action)}</span>
          </button>
        }
      >
        {filtered.map((label) => (
          <PickerItem
            key={label.id}
            selected={attachedIds.has(label.id)}
            onClick={() => toggle(label.id)}
          >
            <span
              className="inline-block h-3 w-3 shrink-0 rounded-full"
              style={{ backgroundColor: label.color }}
              aria-hidden
            />
            <span className="truncate">{label.name}</span>
          </PickerItem>
        ))}
        {filtered.length === 0 && !canCreate && <PickerEmpty />}
        {canCreate && (
          <PickerItem selected={false} onClick={createAndAttach}>
            <Plus className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            <span className="truncate">
              {t(($) => $.pickers.label.create_action)} <span className="font-medium">&ldquo;{query}&rdquo;</span>
            </span>
            <span
              className="ml-auto inline-block h-3 w-3 shrink-0 rounded-full"
              style={{ backgroundColor: pickInlineColor(query) }}
              aria-hidden
            />
          </PickerItem>
        )}
      </PropertyPicker>

      <Dialog open={manageOpen} onOpenChange={setManageOpen}>
        <DialogContent className="max-w-2xl">
          <DialogTitle className="text-lg font-semibold">{t(($) => $.pickers.label.manage_dialog_title)}</DialogTitle>
          <LabelsPanel />
        </DialogContent>
      </Dialog>
    </div>
  );
}
