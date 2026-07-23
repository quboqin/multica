"use client";

import type { TaskAttribution } from "@multica/core/types";
import { ActorAvatar } from "@multica/ui/components/common/actor-avatar";
import { Badge } from "@multica/ui/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { cn } from "@multica/ui/lib/utils";

function initialsOf(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  const first = parts[0];
  if (!first) return "?";
  const last = parts[parts.length - 1];
  if (parts.length === 1 || !last) return first.slice(0, 2).toUpperCase();
  return (first.charAt(0) + last.charAt(0)).toUpperCase();
}

function sourceLabelOf(source: string): string {
  switch (source) {
    case "direct_human":
      return "Direct human";
    case "delegation":
      return "Delegation";
    case "comment_source":
      return "Comment source";
    case "trigger_owner":
      return "Trigger owner";
    case "rule_owner":
      return "Rule owner";
    case "owner_fallback":
      return "Owner fallback";
    case "backfill":
      return "Backfilled";
    case "unattributed":
      return "Unattributed";
    default:
      return source || "Unattributed";
  }
}

export function AttributionBadge({
  attribution,
  className,
  variant = "badge",
}: {
  attribution?: TaskAttribution;
  className?: string;
  variant?: "badge" | "avatar";
}) {
  const initiator = attribution?.initiator;
  if (!attribution || !initiator) return null;

  const name = initiator.name || "Someone";
  const sourceLabel = sourceLabelOf(attribution.source);
  const uncertain =
    attribution.precise === false && attribution.source !== "backfill";

  if (variant === "avatar") {
    return (
      <Tooltip>
        <TooltipTrigger
          render={
            <span
              className={cn(
                "inline-flex shrink-0",
                uncertain && "rounded-full ring-1 ring-warning/60",
                className
              )}
            >
              <ActorAvatar
                name={name}
                initials={initialsOf(name)}
                avatarUrl={initiator.avatar_url}
                size={18}
              />
            </span>
          }
        />
        <TooltipContent>
          <span>
            On behalf of {name} · {sourceLabel}
          </span>
        </TooltipContent>
      </Tooltip>
    );
  }

  return (
    <Badge
      variant="outline"
      className={cn(
        "max-w-40 min-w-0 gap-1 font-normal",
        uncertain ? "text-warning" : "text-muted-foreground",
        className
      )}
      title={sourceLabel}
    >
      <ActorAvatar
        name={name}
        initials={initialsOf(name)}
        avatarUrl={initiator.avatar_url}
        size={16}
        className="shrink-0"
      />
      <span className="min-w-0 truncate">On behalf of {name}</span>
    </Badge>
  );
}
