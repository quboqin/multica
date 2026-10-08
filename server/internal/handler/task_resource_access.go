package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// What a run may do with a document or table.
//
// A task token authenticates as the owner of the runtime the run executes on
// (R). Left alone, that made two mistakes at once: an agent a person (A)
// invited onto their private document got 404 on every read and reply because
// R was outside its audience, while anyone allowed to invoke an agent on a
// shared runtime could reach, through it, private documents and tables that
// only R can see.
//
// Both are settled here, for every document and collection check:
//
//   - With a human originator on the run, the effective permission is the
//     INTERSECTION of R's and A's: an agent never does more than the person who
//     asked, nor more than the runtime it runs on.
//   - On the one document the run was invited onto (the task's own issue), a
//     grant A recorded when mentioning the agent lifts the floor to what A
//     allowed — capped at A's current permission and at edit. Sharing,
//     publishing, moving and deleting stay with people.
//   - A run with no human at the top of its chain (autopilot, system triggers)
//     keeps today's behaviour: R's permission alone.
//
// Permissions are the strings the document and collection queries return:
// "" (no access), "view", "edit", "owner".

var permissionRank = map[string]int{"": 0, "view": 1, "edit": 2, "owner": 3}

func minPermission(a, b string) string {
	if permissionRank[a] <= permissionRank[b] {
		return a
	}
	return b
}

func maxPermission(a, b string) string {
	if permissionRank[a] >= permissionRank[b] {
		return a
	}
	return b
}

// requestTaskID is the task bound to a task-token request, or "". The auth
// middleware stamps both headers for mat_ tokens and strips client copies, so
// a request that merely carries X-Task-ID is not a run.
func requestTaskID(r *http.Request) string {
	if r.Header.Get("X-Actor-Source") != "task_token" {
		return ""
	}
	return r.Header.Get("X-Task-ID")
}

// taskOriginator is the run behind taskID and the human at the top of its
// chain. ok is false when there is no such live run or no human behind it —
// the caller then falls back to the token user's own permission.
func (h *Handler) taskOriginator(ctx context.Context, q *db.Queries, taskID string) (db.AgentTaskQueue, string, bool) {
	if taskID == "" {
		return db.AgentTaskQueue{}, "", false
	}
	id, err := util.ParseUUID(taskID)
	if err != nil {
		return db.AgentTaskQueue{}, "", false
	}
	task, err := q.GetAgentTask(ctx, id)
	if err != nil || isTerminalTaskStatus(task.Status) {
		return db.AgentTaskQueue{}, "", false
	}
	originator := task.OriginatorUserID
	if !originator.Valid {
		originator = task.InitiatorUserID
	}
	if !originator.Valid {
		return task, "", false
	}
	return task, uuidToString(originator), true
}

// requestOriginatorUserID is the human behind a task-token request, or "" for
// a person's own request and for a run with no human at the top of its chain.
func (h *Handler) requestOriginatorUserID(r *http.Request) string {
	_, originator, ok := h.taskOriginator(r.Context(), h.Queries, requestTaskID(r))
	if !ok {
		return ""
	}
	return originator
}

// readableByOriginator narrows a list a run reads to the issues the human
// behind the run can also read. Lists are filtered in SQL against the token
// user; this is the originator's half of the intersection. A person's own
// request, or a run with no human behind it, gets the list unchanged.
func (h *Handler) readableByOriginator(r *http.Request, workspaceID pgtype.UUID, ids []pgtype.UUID) (map[pgtype.UUID]bool, bool, error) {
	originator := h.requestOriginatorUserID(r)
	if originator == "" || len(ids) == 0 {
		return nil, false, nil
	}
	readable, err := h.Queries.ListReadableIssueIDs(r.Context(), db.ListReadableIssueIDsParams{WorkspaceID: workspaceID, IssueIds: ids, UserID: parseUUID(originator)})
	if err != nil {
		return nil, false, err
	}
	allowed := make(map[pgtype.UUID]bool, len(readable))
	for _, id := range readable {
		allowed[id] = true
	}
	return allowed, true, nil
}

func (h *Handler) effectiveDocumentPermission(ctx context.Context, q *db.Queries, doc db.Issue, userID, taskID string) string {
	base := h.documentPermission(ctx, q, doc, userID)
	if doc.Kind != "doc" {
		return base
	}
	task, originator, ok := h.taskOriginator(ctx, q, taskID)
	if !ok {
		return base
	}
	inviter := h.documentPermission(ctx, q, doc, originator)
	effective := minPermission(base, inviter)
	if task.IssueID.Valid && task.IssueID == doc.ID && task.TriggerCommentID.Valid && task.AgentID.Valid {
		grant, err := q.GetCommentAgentGrant(ctx, db.GetCommentAgentGrantParams{CommentID: task.TriggerCommentID, AgentID: task.AgentID, WorkspaceID: doc.WorkspaceID})
		if err == nil {
			effective = maxPermission(effective, minPermission(grant.Permission, minPermission(inviter, "edit")))
		}
	}
	return effective
}

func (h *Handler) effectiveCollectionPermission(ctx context.Context, q *db.Queries, collection db.Collection, userID, taskID string) string {
	base := h.collectionPermission(ctx, q, collection, userID)
	_, originator, ok := h.taskOriginator(ctx, q, taskID)
	if !ok {
		return base
	}
	return minPermission(base, h.collectionPermission(ctx, q, collection, originator))
}

func (h *Handler) requestDocumentPermission(r *http.Request, q *db.Queries, doc db.Issue) string {
	return h.effectiveDocumentPermission(r.Context(), q, doc, requestUserID(r), requestTaskID(r))
}

func (h *Handler) requestCollectionPermission(r *http.Request, q *db.Queries, collection db.Collection) string {
	return h.effectiveCollectionPermission(r.Context(), q, collection, requestUserID(r), requestTaskID(r))
}

// createdResourceOwners decides who owns a document or table this request
// creates. A person's own request: themselves. A run with a human behind it:
// that person, with the runtime owner (the token user) as an edit collaborator
// so the run keeps working on what it made — an empty second value when the
// two are the same person. A run with no human behind it: the token user.
func (h *Handler) createdResourceOwners(r *http.Request, workspaceID pgtype.UUID, tokenUserID string) (owner, runtimeEditor pgtype.UUID) {
	owner = parseUUID(tokenUserID)
	originator := h.requestOriginatorUserID(r)
	if originator == "" || originator == tokenUserID {
		return owner, pgtype.UUID{}
	}
	if _, err := h.Queries.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{UserID: parseUUID(originator), WorkspaceID: workspaceID}); err != nil {
		return owner, pgtype.UUID{}
	}
	return parseUUID(originator), owner
}

// runtimeOwnerOfAgent is the person a run of agent will authenticate as.
func (h *Handler) runtimeOwnerOfAgent(ctx context.Context, agent db.Agent) pgtype.UUID {
	if agent.RuntimeID.Valid {
		if runtime, err := h.Queries.GetAgentRuntime(ctx, agent.RuntimeID); err == nil && runtime.OwnerID.Valid {
			return runtime.OwnerID
		}
	}
	return agent.OwnerID
}

// commentTriggerDocumentAccess tells the composer, for one agent a comment on
// doc would trigger, what its run will be able to do there without a grant and
// the most the poster may grant it.
func (h *Handler) commentTriggerDocumentAccess(ctx context.Context, doc db.Issue, agent db.Agent, posterID string) *CommentTriggerDocumentAccess {
	poster := h.documentPermission(ctx, h.Queries, doc, posterID)
	runtimeOwner := ""
	if owner := h.runtimeOwnerOfAgent(ctx, agent); owner.Valid {
		runtimeOwner = h.documentPermission(ctx, h.Queries, doc, uuidToString(owner))
	}
	access := &CommentTriggerDocumentAccess{RuntimeOwnerPermission: minPermission(runtimeOwner, poster)}
	if maxGrant := minPermission(poster, "edit"); permissionRank[maxGrant] > permissionRank[access.RuntimeOwnerPermission] {
		access.MaxGrant = maxGrant
	}
	return access
}

type commentAgentGrant struct {
	AgentID    pgtype.UUID
	Permission string
}

// validateCommentAgentGrants checks the grants a comment carries: a person
// posting on a document may allow a workspace agent view or edit, never more
// than they hold themselves. Anything else is a 400; a run cannot grant.
func (h *Handler) validateCommentAgentGrants(w http.ResponseWriter, r *http.Request, issue db.Issue, inputs []CommentAgentGrantInput) ([]commentAgentGrant, bool) {
	if len(inputs) == 0 {
		return nil, true
	}
	if issue.Kind != "doc" {
		writeError(w, http.StatusBadRequest, "agent grants apply to documents only")
		return nil, false
	}
	if isMachineCredentialActor(r) {
		writeError(w, http.StatusForbidden, "only a person can grant an agent access")
		return nil, false
	}
	maxGrant := minPermission(h.documentPermission(r.Context(), h.Queries, issue, requestUserID(r)), "edit")
	grants := make([]commentAgentGrant, 0, len(inputs))
	for _, input := range inputs {
		agentID, ok := parseUUIDOrBadRequest(w, input.AgentID, "agent_grants.agent_id")
		if !ok {
			return nil, false
		}
		if _, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: issue.WorkspaceID}); err != nil {
			writeError(w, http.StatusBadRequest, "agent_grants names an unknown agent")
			return nil, false
		}
		if input.Permission != "view" && input.Permission != "edit" {
			writeError(w, http.StatusBadRequest, "agent_grants.permission must be view or edit")
			return nil, false
		}
		if permissionRank[input.Permission] > permissionRank[maxGrant] {
			writeError(w, http.StatusForbidden, "you cannot grant an agent more than your own access")
			return nil, false
		}
		grants = append(grants, commentAgentGrant{AgentID: agentID, Permission: input.Permission})
	}
	return grants, true
}

func (h *Handler) recordCommentAgentGrants(r *http.Request, issue db.Issue, comment db.Comment, grants []commentAgentGrant) {
	for _, grant := range grants {
		err := h.Queries.UpsertCommentAgentGrant(r.Context(), db.UpsertCommentAgentGrantParams{
			CommentID: comment.ID, AgentID: grant.AgentID, WorkspaceID: issue.WorkspaceID, IssueID: issue.ID,
			Permission: grant.Permission, GrantedBy: parseUUID(requestUserID(r)),
		})
		if err != nil {
			slog.Warn("record agent document grant failed", "error", err, "comment_id", uuidToString(comment.ID), "agent_id", uuidToString(grant.AgentID))
		}
	}
}
