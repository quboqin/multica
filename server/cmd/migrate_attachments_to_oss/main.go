package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/storage"
)

const firstUUID = "00000000-0000-0000-0000-000000000000"

type attachmentRow struct {
	ID          string
	Filename    string
	URL         string
	ContentType string
	SizeBytes   int64
}

type candidateStoredObjectRow struct {
	ID          string
	PreviewURL  string
	ResourceURL string
	PosterURL   string
	OriginalURL string
	ArchivedURL string
}

type migrationSource interface {
	Validate(key string) error
	Read(key string) ([]byte, error)
	Delete(key string) error
}

type migrationDestination interface {
	Upload(ctx context.Context, key string, data []byte, contentType string, filename string) (string, error)
	IsURL(rawURL string) bool
	KeyFromURL(rawURL string) string
	Verify(ctx context.Context, key string, expected []byte) error
}

type migrationUpdater func(ctx context.Context, row attachmentRow, newURL string) (bool, error)

type migrationQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type localFileSource struct {
	root string
}

func (s localFileSource) path(key string) (string, error) {
	if key == "" || filepath.IsAbs(key) {
		return "", fmt.Errorf("invalid local attachment key %q", key)
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", fmt.Errorf("resolve upload root: %w", err)
	}
	target, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(key)))
	if err != nil {
		return "", fmt.Errorf("resolve attachment path: %w", err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local attachment key escapes upload root: %q", key)
	}
	return target, nil
}

func (s localFileSource) Validate(key string) error {
	target, err := s.path(key)
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	return nil
}

func (s localFileSource) Read(key string) ([]byte, error) {
	target, err := s.path(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s localFileSource) Delete(key string) error {
	target, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target + ".meta.json"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete local metadata sidecar: %w", err)
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

type migrationStatus string

const (
	statusAlreadyMigrated migrationStatus = "already_migrated"
	statusNotLocal        migrationStatus = "not_local"
	statusEligible        migrationStatus = "eligible"
	statusMigrated        migrationStatus = "migrated"
	statusChanged         migrationStatus = "changed"
	statusLocalDeleted    migrationStatus = "local_deleted"
)

type migrationResult struct {
	row    attachmentRow
	status migrationStatus
	err    error
}

func migrateRows(
	ctx context.Context,
	rows []attachmentRow,
	concurrency int,
	migrate func(context.Context, attachmentRow) (migrationStatus, error),
) <-chan migrationResult {
	results := make(chan migrationResult, len(rows))
	if len(rows) == 0 {
		close(results)
		return results
	}
	if concurrency > len(rows) {
		concurrency = len(rows)
	}
	jobs := make(chan attachmentRow)
	var workers sync.WaitGroup
	workers.Add(concurrency)
	for range concurrency {
		go func() {
			defer workers.Done()
			for row := range jobs {
				status, err := migrate(ctx, row)
				results <- migrationResult{row: row, status: status, err: err}
			}
		}()
	}
	go func() {
		for _, row := range rows {
			jobs <- row
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()
	return results
}

func localAttachmentKey(rawURL, configuredBaseURL string) (string, bool) {
	if strings.HasPrefix(rawURL, "/uploads/") {
		return cleanLocalKey(strings.TrimPrefix(rawURL, "/uploads/"))
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	if configuredBaseURL == "" || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	base, baseErr := url.Parse(strings.TrimRight(configuredBaseURL, "/"))
	if baseErr != nil || base.Scheme == "" || base.Host == "" ||
		!strings.EqualFold(parsed.Scheme, base.Scheme) || !strings.EqualFold(parsed.Host, base.Host) {
		return "", false
	}
	if !strings.HasPrefix(parsed.Path, "/uploads/") {
		return "", false
	}
	key, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/uploads/"))
	if err != nil {
		return "", false
	}
	return cleanLocalKey(key)
}

func cleanLocalKey(key string) (string, bool) {
	key = filepath.ToSlash(filepath.Clean(filepath.FromSlash(key)))
	if key == "." || key == "" || key == ".." || strings.HasPrefix(key, "../") || filepath.IsAbs(key) {
		return "", false
	}
	return key, true
}

func localURLAliases(rawURL, configuredBaseURL string) []string {
	aliases := []string{rawURL}
	if strings.HasPrefix(rawURL, "/uploads/") {
		base := strings.TrimRight(configuredBaseURL, "/")
		if parsed, err := url.Parse(base); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
			aliases = append(aliases, base+rawURL)
		}
	}
	return aliases
}

func absoluteLocalURL(rawURL, configuredBaseURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
		return rawURL
	}
	aliases := localURLAliases(rawURL, configuredBaseURL)
	if len(aliases) > 1 {
		return aliases[1]
	}
	return ""
}

func updateAttachmentURLAndReferences(
	ctx context.Context,
	querier migrationQuerier,
	row attachmentRow,
	newURL string,
	localBaseURL string,
) (bool, error) {
	aliases := localURLAliases(row.URL, localBaseURL)
	contentURL := absoluteLocalURL(row.URL, localBaseURL)
	var updated bool
	err := querier.QueryRow(ctx, `
WITH changed AS (
  UPDATE attachment
  SET url = $1
  WHERE id = $2::uuid AND url = $3
  RETURNING id, workspace_id
), candidate_cache AS (
  UPDATE creative_material_candidate c
  SET preview_url = CASE WHEN c.preview_url = ANY($4::text[]) THEN $1 ELSE c.preview_url END,
      resource_url = CASE WHEN c.resource_url = ANY($4::text[]) THEN $1 ELSE c.resource_url END,
      poster_url = CASE WHEN c.poster_url = ANY($4::text[]) THEN $1 ELSE c.poster_url END,
      original_url = CASE WHEN c.original_url = ANY($4::text[]) THEN $1 ELSE c.original_url END,
      archived_url = CASE WHEN c.archived_url = ANY($4::text[]) THEN $1 ELSE c.archived_url END,
      updated_at = now()
  FROM changed
  WHERE c.source_attachment_id = changed.id
    AND (c.preview_url = ANY($4::text[]) OR c.resource_url = ANY($4::text[])
      OR c.poster_url = ANY($4::text[]) OR c.original_url = ANY($4::text[])
      OR c.archived_url = ANY($4::text[]))
), issue_cache AS (
  UPDATE issue i
  SET description = replace(i.description, $5, $1)
  FROM changed
  WHERE $5 <> '' AND i.workspace_id = changed.workspace_id
    AND position($5 in COALESCE(i.description, '')) > 0
), comment_cache AS (
  UPDATE comment c
  SET content = replace(c.content, $5, $1)
  FROM changed
  WHERE $5 <> '' AND c.workspace_id = changed.workspace_id
    AND position($5 in c.content) > 0
), chat_cache AS (
  UPDATE chat_message m
  SET content = replace(m.content, $5, $1)
  FROM chat_session s, changed
  WHERE $5 <> '' AND m.chat_session_id = s.id
    AND s.workspace_id = changed.workspace_id
    AND position($5 in m.content) > 0
), creative_context_cache AS (
  UPDATE creative_issue_context c
  SET snapshot = replace(c.snapshot::text, $5, $1)::jsonb
  FROM changed
  WHERE $5 <> '' AND c.workspace_id = changed.workspace_id
    AND position($5 in c.snapshot::text) > 0
), creative_order_cache AS (
  UPDATE creative_order o
  SET input_snapshot = replace(o.input_snapshot::text, $5, $1)::jsonb
  FROM changed
  WHERE $5 <> '' AND o.workspace_id = changed.workspace_id
    AND position($5 in o.input_snapshot::text) > 0
)
SELECT EXISTS(SELECT 1 FROM changed)
`, newURL, row.ID, row.URL, aliases, contentURL).Scan(&updated)
	return updated, err
}

func updateCandidateArchiveURLAndReferences(
	ctx context.Context,
	querier migrationQuerier,
	row attachmentRow,
	newURL string,
	localBaseURL string,
	promoteArchive bool,
) (bool, error) {
	aliases := localURLAliases(row.URL, localBaseURL)
	contentURL := absoluteLocalURL(row.URL, localBaseURL)
	var updated bool
	err := querier.QueryRow(ctx, `
WITH changed AS (
  UPDATE creative_material_candidate
  SET preview_url = CASE WHEN preview_url = ANY($3::text[]) THEN $1 ELSE preview_url END,
      resource_url = CASE WHEN resource_url = ANY($3::text[]) THEN $1 ELSE resource_url END,
      poster_url = CASE WHEN poster_url = ANY($3::text[]) THEN $1 ELSE poster_url END,
      original_url = CASE WHEN original_url = ANY($3::text[]) THEN $1 ELSE original_url END,
      archived_url = CASE
        WHEN archived_url = ANY($3::text[]) OR (archived_url = '' AND $5::boolean) THEN $1
        ELSE archived_url
      END,
      archive_status = CASE
        WHEN archived_url = ANY($3::text[]) OR (archived_url = '' AND $5::boolean) THEN 'completed'
        ELSE archive_status
      END,
      archive_error = CASE
        WHEN archived_url = ANY($3::text[]) OR (archived_url = '' AND $5::boolean) THEN ''
        ELSE archive_error
      END,
      archived_at = CASE
        WHEN archived_url = ANY($3::text[]) OR (archived_url = '' AND $5::boolean) THEN now()
        ELSE archived_at
      END,
      updated_at = now()
  WHERE id = $2::uuid
    AND (preview_url = ANY($3::text[]) OR resource_url = ANY($3::text[])
      OR poster_url = ANY($3::text[]) OR original_url = ANY($3::text[])
      OR archived_url = ANY($3::text[]))
  RETURNING workspace_id
), issue_cache AS (
  UPDATE issue i
  SET description = replace(i.description, $4, $1)
  FROM changed
  WHERE $4 <> '' AND i.workspace_id = changed.workspace_id
    AND position($4 in COALESCE(i.description, '')) > 0
), comment_cache AS (
  UPDATE comment c
  SET content = replace(c.content, $4, $1)
  FROM changed
  WHERE $4 <> '' AND c.workspace_id = changed.workspace_id
    AND position($4 in c.content) > 0
), chat_cache AS (
  UPDATE chat_message m
  SET content = replace(m.content, $4, $1)
  FROM chat_session s, changed
  WHERE $4 <> '' AND m.chat_session_id = s.id
    AND s.workspace_id = changed.workspace_id
    AND position($4 in m.content) > 0
), creative_context_cache AS (
  UPDATE creative_issue_context c
  SET snapshot = replace(c.snapshot::text, $4, $1)::jsonb
  FROM changed
  WHERE $4 <> '' AND c.workspace_id = changed.workspace_id
    AND position($4 in c.snapshot::text) > 0
), creative_order_cache AS (
  UPDATE creative_order o
  SET input_snapshot = replace(o.input_snapshot::text, $4, $1)::jsonb
  FROM changed
  WHERE $4 <> '' AND o.workspace_id = changed.workspace_id
    AND position($4 in o.input_snapshot::text) > 0
)
SELECT EXISTS(SELECT 1 FROM changed)
`, newURL, row.ID, aliases, contentURL, promoteArchive).Scan(&updated)
	return updated, err
}

func candidateLocalURLs(row candidateStoredObjectRow, localBaseURL string) []string {
	// Prefer the durable archive and original source before derived preview/poster caches.
	ordered := []string{row.ArchivedURL, row.OriginalURL, row.ResourceURL, row.PreviewURL, row.PosterURL}
	seen := make(map[string]struct{}, len(ordered))
	local := make([]string, 0, len(ordered))
	for _, rawURL := range ordered {
		if rawURL == "" {
			continue
		}
		if _, ok := localAttachmentKey(rawURL, localBaseURL); !ok {
			continue
		}
		if _, ok := seen[rawURL]; ok {
			continue
		}
		seen[rawURL] = struct{}{}
		local = append(local, rawURL)
	}
	return local
}

func migrateCandidateStoredObjects(
	ctx context.Context,
	row candidateStoredObjectRow,
	execute bool,
	deleteLocalAfterVerify bool,
	localBaseURL string,
	source migrationSource,
	destination migrationDestination,
	querier migrationQuerier,
) (migrationStatus, error) {
	localURLs := candidateLocalURLs(row, localBaseURL)
	if len(localURLs) == 0 {
		return statusNotLocal, nil
	}

	statuses := make([]migrationStatus, 0, len(localURLs))
	errs := make([]error, 0)
	archivePromoted := false
	for _, rawURL := range localURLs {
		stored := attachmentRow{ID: row.ID, URL: rawURL}
		stored.Filename, stored.ContentType = storedObjectMetadata(rawURL)
		update := func(ctx context.Context, stored attachmentRow, newURL string) (bool, error) {
			promoteArchive := row.ArchivedURL == "" && !archivePromoted
			updated, err := updateCandidateArchiveURLAndReferences(
				ctx, querier, stored, newURL, localBaseURL, promoteArchive,
			)
			if err == nil && updated && promoteArchive {
				archivePromoted = true
			}
			return updated, err
		}
		status, err := migrateAttachment(
			ctx, stored, execute, deleteLocalAfterVerify, localBaseURL, source, destination, update,
		)
		if err != nil {
			errs = append(errs, fmt.Errorf("migrate candidate URL %q: %w", rawURL, err))
			continue
		}
		statuses = append(statuses, status)
	}
	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}
	return aggregateMigrationStatus(statuses), nil
}

func aggregateMigrationStatus(statuses []migrationStatus) migrationStatus {
	priority := []migrationStatus{
		statusLocalDeleted,
		statusMigrated,
		statusEligible,
		statusChanged,
		statusAlreadyMigrated,
		statusNotLocal,
	}
	for _, wanted := range priority {
		for _, status := range statuses {
			if status == wanted {
				return status
			}
		}
	}
	return statusNotLocal
}

func storedObjectMetadata(rawURL string) (filename, contentType string) {
	filename = "asset"
	if parsed, err := url.Parse(rawURL); err == nil {
		if base := filepath.Base(parsed.Path); base != "." && base != "/" && base != "" {
			filename = base
		}
	}
	contentType = mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return filename, contentType
}

func migrateAttachment(
	ctx context.Context,
	row attachmentRow,
	execute bool,
	deleteLocalAfterVerify bool,
	localBaseURL string,
	source migrationSource,
	destination migrationDestination,
	update migrationUpdater,
) (migrationStatus, error) {
	if destination.IsURL(row.URL) {
		if !execute || !deleteLocalAfterVerify {
			return statusAlreadyMigrated, nil
		}
		key := destination.KeyFromURL(row.URL)
		if err := source.Validate(key); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return statusAlreadyMigrated, nil
			}
			return "", fmt.Errorf("validate residual local source %q: %w", key, err)
		}
		data, err := source.Read(key)
		if err != nil {
			return "", fmt.Errorf("read residual local source %q: %w", key, err)
		}
		if err := destination.Verify(ctx, key, data); err != nil {
			return "", err
		}
		if err := source.Delete(key); err != nil {
			return "", fmt.Errorf("delete verified residual local source %q: %w", key, err)
		}
		return statusLocalDeleted, nil
	}
	key, ok := localAttachmentKey(row.URL, localBaseURL)
	if !ok {
		return statusNotLocal, nil
	}
	if err := source.Validate(key); err != nil {
		return "", fmt.Errorf("validate local source %q: %w", key, err)
	}
	if !execute {
		return statusEligible, nil
	}

	data, err := source.Read(key)
	if err != nil {
		return "", fmt.Errorf("read local source %q: %w", key, err)
	}
	newURL, err := destination.Upload(ctx, key, data, row.ContentType, row.Filename)
	if err != nil {
		return "", fmt.Errorf("upload %q: %w", key, err)
	}
	if err := destination.Verify(ctx, key, data); err != nil {
		return "", err
	}
	updated, err := update(ctx, row, newURL)
	if err != nil {
		return "", fmt.Errorf("update attachment URL: %w", err)
	}
	if !updated {
		return statusChanged, nil
	}
	if deleteLocalAfterVerify {
		if err := source.Delete(key); err != nil {
			return "", fmt.Errorf("delete verified local source %q: %w", key, err)
		}
		return statusLocalDeleted, nil
	}
	return statusMigrated, nil
}

func main() {
	logger.Init()
	var (
		execute                = flag.Bool("execute", false, "upload eligible local attachments and update their URLs (default is dry-run)")
		deleteLocalAfterVerify = flag.Bool("delete-local-after-verify", false, "after OSS readback verification and a successful URL update, delete the local source file")
		batchSize              = flag.Int("batch-size", 100, "number of attachment rows read per database page")
		concurrency            = flag.Int("concurrency", 8, "parallel OSS upload and readback workers")
		limit                  = flag.Int("limit", 0, "maximum rows to inspect (0 = all rows)")
		scope                  = flag.String("scope", "all", "rows to inspect: all, attachments, or candidate-archives")
		verbose                = flag.Bool("verbose", false, "print one line for every inspected attachment")
	)
	flag.Parse()
	if *batchSize <= 0 || *batchSize > 1000 {
		slog.Error("invalid --batch-size", "value", *batchSize, "allowed", "1..1000")
		os.Exit(2)
	}
	if *concurrency <= 0 || *concurrency > 32 {
		slog.Error("invalid --concurrency", "value", *concurrency, "allowed", "1..32")
		os.Exit(2)
	}
	if *limit < 0 {
		slog.Error("invalid --limit", "value", *limit)
		os.Exit(2)
	}
	if *scope != "all" && *scope != "attachments" && *scope != "candidate-archives" {
		slog.Error("invalid --scope", "value", *scope, "allowed", "all|attachments|candidate-archives")
		os.Exit(2)
	}

	destination := storage.NewOSSStorageFromEnv()
	if destination == nil {
		slog.Error("OSS storage is not fully configured")
		os.Exit(1)
	}
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	uploadRoot := strings.TrimSpace(os.Getenv("LOCAL_UPLOAD_DIR"))
	if uploadRoot == "" {
		uploadRoot = "./data/uploads"
	}
	localBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("LOCAL_UPLOAD_BASE_URL")), "/")

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		slog.Error("ping database", "error", err)
		os.Exit(1)
	}

	mode := "dry-run"
	localPolicy := "local files will be retained"
	if *execute {
		mode = "execute"
		if *deleteLocalAfterVerify {
			localPolicy = "verified local files will be deleted after URL updates"
		}
	}
	fmt.Printf("Local stored-object to OSS migration (%s); %s.\n", mode, localPolicy)

	source := localFileSource{root: uploadRoot}
	update := func(ctx context.Context, row attachmentRow, newURL string) (bool, error) {
		return updateAttachmentURLAndReferences(ctx, pool, row, newURL, localBaseURL)
	}

	counts := map[migrationStatus]int{}
	failures := 0
	inspected := 0
	attachmentsInspected := 0
	candidateArchivesInspected := 0
	afterID := firstUUID
	for *scope != "candidate-archives" {
		pageSize := *batchSize
		if *limit > 0 && *limit-inspected < pageSize {
			pageSize = *limit - inspected
		}
		if pageSize <= 0 {
			break
		}
		rows, err := pool.Query(ctx, `
SELECT id::text, filename, url, content_type, size_bytes
FROM attachment
WHERE id > $1::uuid
ORDER BY id
LIMIT $2`, afterID, pageSize)
		if err != nil {
			slog.Error("list attachments", "error", err)
			os.Exit(1)
		}
		page := make([]attachmentRow, 0, pageSize)
		for rows.Next() {
			var row attachmentRow
			if err := rows.Scan(&row.ID, &row.Filename, &row.URL, &row.ContentType, &row.SizeBytes); err != nil {
				rows.Close()
				slog.Error("scan attachment", "error", err)
				os.Exit(1)
			}
			page = append(page, row)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			slog.Error("iterate attachments", "error", err)
			os.Exit(1)
		}
		rows.Close()
		if len(page) == 0 {
			break
		}

		migrate := func(ctx context.Context, row attachmentRow) (migrationStatus, error) {
			return migrateAttachment(ctx, row, *execute, *deleteLocalAfterVerify, localBaseURL, source, destination, update)
		}
		for result := range migrateRows(ctx, page, *concurrency, migrate) {
			inspected++
			attachmentsInspected++
			if result.err != nil {
				failures++
				slog.Error("attachment migration failed", "attachment_id", result.row.ID, "error", result.err)
			} else {
				counts[result.status]++
				if *verbose {
					fmt.Printf("%s %s\n", result.status, result.row.ID)
				}
			}
		}
		afterID = page[len(page)-1].ID
		if len(page) < pageSize {
			break
		}
	}

	afterCandidateID := firstUUID
	for *scope != "attachments" {
		pageSize := *batchSize
		if *limit > 0 && *limit-inspected < pageSize {
			pageSize = *limit - inspected
		}
		if pageSize <= 0 {
			break
		}
		rows, err := pool.Query(ctx, `
SELECT c.id::text, c.preview_url, c.resource_url, c.poster_url, c.original_url, c.archived_url
FROM creative_material_candidate c
WHERE c.id > $1::uuid
  AND (c.preview_url LIKE '/uploads/%' OR c.preview_url LIKE '%/uploads/%'
    OR c.resource_url LIKE '/uploads/%' OR c.resource_url LIKE '%/uploads/%'
    OR c.poster_url LIKE '/uploads/%' OR c.poster_url LIKE '%/uploads/%'
    OR c.original_url LIKE '/uploads/%' OR c.original_url LIKE '%/uploads/%'
    OR c.archived_url LIKE '/uploads/%' OR c.archived_url LIKE '%/uploads/%')
ORDER BY c.id
LIMIT $2`, afterCandidateID, pageSize)
		if err != nil {
			slog.Error("list creative material stored objects", "error", err)
			os.Exit(1)
		}
		page := make([]candidateStoredObjectRow, 0, pageSize)
		for rows.Next() {
			var row candidateStoredObjectRow
			if err := rows.Scan(
				&row.ID, &row.PreviewURL, &row.ResourceURL, &row.PosterURL, &row.OriginalURL, &row.ArchivedURL,
			); err != nil {
				rows.Close()
				slog.Error("scan creative material stored object", "error", err)
				os.Exit(1)
			}
			page = append(page, row)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			slog.Error("iterate creative material stored objects", "error", err)
			os.Exit(1)
		}
		rows.Close()
		if len(page) == 0 {
			break
		}

		results := make(chan migrationResult, len(page))
		jobs := make(chan candidateStoredObjectRow)
		workerCount := *concurrency
		if workerCount > len(page) {
			workerCount = len(page)
		}
		var workers sync.WaitGroup
		workers.Add(workerCount)
		for range workerCount {
			go func() {
				defer workers.Done()
				for row := range jobs {
					status, err := migrateCandidateStoredObjects(
						ctx, row, *execute, *deleteLocalAfterVerify, localBaseURL, source, destination, pool,
					)
					results <- migrationResult{row: attachmentRow{ID: row.ID}, status: status, err: err}
				}
			}()
		}
		go func() {
			for _, row := range page {
				jobs <- row
			}
			close(jobs)
			workers.Wait()
			close(results)
		}()
		for result := range results {
			inspected++
			candidateArchivesInspected++
			if result.err != nil {
				failures++
				slog.Error("creative material stored-object migration failed", "candidate_id", result.row.ID, "error", result.err)
			} else {
				counts[result.status]++
				if *verbose {
					fmt.Printf("%s creative_material_candidate %s\n", result.status, result.row.ID)
				}
			}
		}
		afterCandidateID = page[len(page)-1].ID
		if len(page) < pageSize {
			break
		}
	}

	fmt.Printf(
		"Inspected=%d attachments=%d candidate_archives=%d eligible=%d migrated=%d local_deleted=%d already_migrated=%d not_local=%d changed=%d failures=%d\n",
		inspected,
		attachmentsInspected,
		candidateArchivesInspected,
		counts[statusEligible],
		counts[statusMigrated],
		counts[statusLocalDeleted],
		counts[statusAlreadyMigrated],
		counts[statusNotLocal],
		counts[statusChanged],
		failures,
	)
	if failures > 0 {
		os.Exit(1)
	}
}
