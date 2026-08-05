package main

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeMigrationSource struct {
	data             []byte
	dataByKey        map[string][]byte
	validateErr      error
	validateErrByKey map[string]error
	readErr          error
	validateCalls    []string
	readCalls        []string
	deleteCalls      []string
	deleteErr        error
}

func (s *fakeMigrationSource) Validate(key string) error {
	s.validateCalls = append(s.validateCalls, key)
	if err, ok := s.validateErrByKey[key]; ok {
		return err
	}
	return s.validateErr
}

func (s *fakeMigrationSource) Read(key string) ([]byte, error) {
	s.readCalls = append(s.readCalls, key)
	if data, ok := s.dataByKey[key]; ok {
		return data, s.readErr
	}
	return s.data, s.readErr
}

func (s *fakeMigrationSource) Delete(key string) error {
	s.deleteCalls = append(s.deleteCalls, key)
	return s.deleteErr
}

type fakeMigrationDestination struct {
	ownedURLs  map[string]bool
	uploadURL  string
	uploadErr  error
	uploadKeys []string
	verifyKeys []string
	verifyErr  error
}

type fakeMigrationQueryRow struct {
	value bool
	err   error
}

func (r fakeMigrationQueryRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*bool)) = r.value
	return nil
}

type fakeMigrationQuerier struct {
	query string
	args  []any
	row   fakeMigrationQueryRow
}

func (q *fakeMigrationQuerier) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	q.query = sql
	q.args = args
	return q.row
}

func (d *fakeMigrationDestination) Upload(_ context.Context, key string, _ []byte, _, _ string) (string, error) {
	d.uploadKeys = append(d.uploadKeys, key)
	return d.uploadURL, d.uploadErr
}

func (d *fakeMigrationDestination) IsURL(rawURL string) bool {
	return d.ownedURLs[rawURL]
}

func (d *fakeMigrationDestination) KeyFromURL(rawURL string) string {
	if i := len("https://static.example.com/"); len(rawURL) > i && rawURL[:i] == "https://static.example.com/" {
		return rawURL[i:]
	}
	return rawURL
}

func (d *fakeMigrationDestination) Verify(_ context.Context, key string, _ []byte) error {
	d.verifyKeys = append(d.verifyKeys, key)
	return d.verifyErr
}

func TestLocalAttachmentKey(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		baseURL string
		want    string
		ok      bool
	}{
		{name: "relative", rawURL: "/uploads/workspace/a.png", want: "workspace/a.png", ok: true},
		{name: "absolute", rawURL: "https://api.example.com/uploads/workspace/a%20b.png", baseURL: "https://api.example.com", want: "workspace/a b.png", ok: true},
		{name: "different host", rawURL: "https://other.example.com/uploads/a.png", baseURL: "https://api.example.com", ok: false},
		{name: "absolute requires configured local host", rawURL: "https://cdn.example.com/uploads/a.png", ok: false},
		{name: "object store URL", rawURL: "https://bucket.example.com/files/a.png", ok: false},
		{name: "traversal", rawURL: "/uploads/../secrets.txt", ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := localAttachmentKey(tc.rawURL, tc.baseURL)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("localAttachmentKey() = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestUpdateAttachmentURLAndReferencesUsesOneAtomicStatement(t *testing.T) {
	querier := &fakeMigrationQuerier{row: fakeMigrationQueryRow{value: true}}
	row := attachmentRow{ID: "00000000-0000-0000-0000-000000000001", URL: "/uploads/workspace/file.png"}
	updated, err := updateAttachmentURLAndReferences(
		context.Background(), querier, row, "https://static.example.com/workspace/file.png", "https://api.example.com",
	)
	if err != nil || !updated {
		t.Fatalf("updateAttachmentURLAndReferences() = (%v, %v)", updated, err)
	}
	for _, table := range []string{
		"UPDATE attachment", "creative_material_candidate", "UPDATE issue", "UPDATE comment", "UPDATE chat_message",
		"UPDATE creative_issue_context", "UPDATE creative_order",
	} {
		if !strings.Contains(querier.query, table) {
			t.Fatalf("atomic update query does not include %q", table)
		}
	}
	aliases, ok := querier.args[3].([]string)
	if !ok || !reflect.DeepEqual(aliases, []string{"/uploads/workspace/file.png", "https://api.example.com/uploads/workspace/file.png"}) {
		t.Fatalf("URL aliases = %#v", querier.args[3])
	}
	if got := querier.args[4]; got != "https://api.example.com/uploads/workspace/file.png" {
		t.Fatalf("absolute content URL = %#v", got)
	}
}

func TestUpdateCandidateArchiveURLAndReferencesCASConflict(t *testing.T) {
	querier := &fakeMigrationQuerier{row: fakeMigrationQueryRow{value: false}}
	updated, err := updateCandidateArchiveURLAndReferences(
		context.Background(), querier,
		attachmentRow{ID: "00000000-0000-0000-0000-000000000001", URL: "https://api.example.com/uploads/creative/source.png"},
		"https://static.example.com/creative/source.png", "", false,
	)
	if err != nil || updated {
		t.Fatalf("updateCandidateArchiveURLAndReferences() = (%v, %v)", updated, err)
	}
	for _, fragment := range []string{
		"preview_url = ANY($3::text[])",
		"archived_url = ANY($3::text[])",
		"archive_status = CASE",
		"archive_error = CASE",
		"archived_at = CASE",
	} {
		if !strings.Contains(querier.query, fragment) {
			t.Fatalf("candidate stored-object update is missing %q", fragment)
		}
	}
}

func TestCandidateLocalURLsPrefersDurableSourcesAndDeduplicates(t *testing.T) {
	row := candidateStoredObjectRow{
		ArchivedURL: "/uploads/material/archive.png",
		OriginalURL: "/uploads/material/archive.png",
		ResourceURL: "https://cdn.example.test/material.png",
		PreviewURL:  "https://api.example.com/uploads/material/preview.png",
		PosterURL:   "https://other.example.com/uploads/material/poster.png",
	}
	want := []string{
		"/uploads/material/archive.png",
		"https://api.example.com/uploads/material/preview.png",
	}
	if got := candidateLocalURLs(row, "https://api.example.com"); !reflect.DeepEqual(got, want) {
		t.Fatalf("candidateLocalURLs() = %#v, want %#v", got, want)
	}
}

func TestMigrateCandidateStoredObjectsPromotesFirstExistingLocalSource(t *testing.T) {
	source := &fakeMigrationSource{
		dataByKey: map[string][]byte{"material/preview.png": []byte("preview")},
		validateErrByKey: map[string]error{
			"material/original.png": os.ErrNotExist,
		},
	}
	destination := &fakeMigrationDestination{uploadURL: "https://static.example.com/material/preview.png"}
	querier := &fakeMigrationQuerier{row: fakeMigrationQueryRow{value: true}}
	status, err := migrateCandidateStoredObjects(
		context.Background(),
		candidateStoredObjectRow{
			ID:          "00000000-0000-0000-0000-000000000001",
			OriginalURL: "/uploads/material/original.png",
			PreviewURL:  "/uploads/material/preview.png",
		},
		true, false, "", source, destination, querier,
	)
	if err == nil || status != "" {
		t.Fatalf("migrateCandidateStoredObjects() = (%q, %v), want partial success reported with error", status, err)
	}
	if !reflect.DeepEqual(destination.uploadKeys, []string{"material/preview.png"}) {
		t.Fatalf("uploaded keys = %#v", destination.uploadKeys)
	}
	if got, ok := querier.args[4].(bool); !ok || !got {
		t.Fatalf("promote archive argument = %#v, want true", querier.args[4])
	}
}

func TestStoredObjectMetadata(t *testing.T) {
	filename, contentType := storedObjectMetadata("https://api.example.com/uploads/material/source.webp")
	if filename != "source.webp" || contentType != "image/webp" {
		t.Fatalf("storedObjectMetadata() = (%q, %q)", filename, contentType)
	}
}

func TestReferenceUpdateSQLAgainstPostgres(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	row := attachmentRow{ID: firstUUID, URL: "https://api.example.com/uploads/missing.png"}
	for name, update := range map[string]func(context.Context, migrationQuerier, attachmentRow, string, string) (bool, error){
		"attachment": updateAttachmentURLAndReferences,
		"candidate": func(ctx context.Context, querier migrationQuerier, row attachmentRow, newURL, localBaseURL string) (bool, error) {
			return updateCandidateArchiveURLAndReferences(ctx, querier, row, newURL, localBaseURL, false)
		},
	} {
		t.Run(name, func(t *testing.T) {
			updated, updateErr := update(ctx, pool, row, "https://static.example.com/missing.png", "")
			if updateErr != nil {
				t.Fatal(updateErr)
			}
			if updated {
				t.Fatal("nonexistent row unexpectedly updated")
			}
		})
	}
}

func TestMigrateAttachmentDryRun(t *testing.T) {
	source := &fakeMigrationSource{}
	destination := &fakeMigrationDestination{}
	updateCalled := false
	status, err := migrateAttachment(
		context.Background(),
		attachmentRow{ID: "attachment-1", URL: "/uploads/workspace/file.png"},
		false,
		false,
		"",
		source,
		destination,
		func(context.Context, attachmentRow, string) (bool, error) {
			updateCalled = true
			return true, nil
		},
	)
	if err != nil || status != statusEligible {
		t.Fatalf("migrateAttachment() = (%q, %v)", status, err)
	}
	if !reflect.DeepEqual(source.validateCalls, []string{"workspace/file.png"}) {
		t.Fatalf("validate calls = %#v", source.validateCalls)
	}
	if len(source.readCalls) != 0 || len(destination.uploadKeys) != 0 || updateCalled {
		t.Fatal("dry-run must not read, upload, or update")
	}
}

func TestMigrateAttachmentUploadsBeforeURLUpdate(t *testing.T) {
	source := &fakeMigrationSource{data: []byte("asset")}
	destination := &fakeMigrationDestination{uploadURL: "https://static.example.com/workspace/file.png"}
	var updatedURL string
	status, err := migrateAttachment(
		context.Background(),
		attachmentRow{
			ID:          "attachment-1",
			URL:         "/uploads/workspace/file.png",
			Filename:    "file.png",
			ContentType: "image/png",
		},
		true,
		false,
		"",
		source,
		destination,
		func(_ context.Context, _ attachmentRow, newURL string) (bool, error) {
			if len(destination.uploadKeys) != 1 {
				t.Fatal("URL update happened before a successful upload")
			}
			updatedURL = newURL
			return true, nil
		},
	)
	if err != nil || status != statusMigrated {
		t.Fatalf("migrateAttachment() = (%q, %v)", status, err)
	}
	if updatedURL != destination.uploadURL {
		t.Fatalf("updated URL = %q", updatedURL)
	}
}

func TestMigrateAttachmentUploadFailureDoesNotUpdate(t *testing.T) {
	source := &fakeMigrationSource{data: []byte("asset")}
	destination := &fakeMigrationDestination{uploadErr: errors.New("denied")}
	updateCalled := false
	_, err := migrateAttachment(
		context.Background(),
		attachmentRow{ID: "attachment-1", URL: "/uploads/file.png"},
		true,
		false,
		"",
		source,
		destination,
		func(context.Context, attachmentRow, string) (bool, error) {
			updateCalled = true
			return true, nil
		},
	)
	if err == nil {
		t.Fatal("expected upload failure")
	}
	if updateCalled {
		t.Fatal("failed upload must not update attachment URL")
	}
}

func TestMigrateAttachmentSkipsAlreadyMigratedURL(t *testing.T) {
	const rawURL = "https://static.example.com/file.png"
	source := &fakeMigrationSource{}
	destination := &fakeMigrationDestination{ownedURLs: map[string]bool{rawURL: true}}
	status, err := migrateAttachment(
		context.Background(),
		attachmentRow{ID: "attachment-1", URL: rawURL},
		true,
		false,
		"",
		source,
		destination,
		func(context.Context, attachmentRow, string) (bool, error) {
			t.Fatal("already-migrated row must not be updated")
			return false, nil
		},
	)
	if err != nil || status != statusAlreadyMigrated {
		t.Fatalf("migrateAttachment() = (%q, %v)", status, err)
	}
	if len(source.validateCalls) != 0 || len(source.readCalls) != 0 || len(destination.uploadKeys) != 0 {
		t.Fatal("already-migrated row must not touch local or OSS data")
	}
}

func TestMigrateAttachmentReadbackMismatchDoesNotUpdateOrDelete(t *testing.T) {
	source := &fakeMigrationSource{data: []byte("asset")}
	destination := &fakeMigrationDestination{
		uploadURL: "https://static.example.com/file.png",
		verifyErr: errors.New("SHA-256 mismatch"),
	}
	updateCalled := false
	_, err := migrateAttachment(
		context.Background(),
		attachmentRow{ID: "attachment-1", URL: "/uploads/file.png"},
		true,
		true,
		"",
		source,
		destination,
		func(context.Context, attachmentRow, string) (bool, error) {
			updateCalled = true
			return true, nil
		},
	)
	if err == nil {
		t.Fatal("expected readback verification failure")
	}
	if updateCalled || len(source.deleteCalls) != 0 {
		t.Fatal("readback mismatch must not update the database or delete local data")
	}
}

func TestMigrateAttachmentCASConflictDoesNotDelete(t *testing.T) {
	source := &fakeMigrationSource{data: []byte("asset")}
	destination := &fakeMigrationDestination{uploadURL: "https://static.example.com/file.png"}
	status, err := migrateAttachment(
		context.Background(),
		attachmentRow{ID: "attachment-1", URL: "/uploads/file.png"},
		true,
		true,
		"",
		source,
		destination,
		func(context.Context, attachmentRow, string) (bool, error) {
			return false, nil
		},
	)
	if err != nil || status != statusChanged {
		t.Fatalf("migrateAttachment() = (%q, %v)", status, err)
	}
	if len(source.deleteCalls) != 0 {
		t.Fatal("CAS conflict must not delete local data")
	}
}

func TestMigrateAttachmentDeletesOnlyAfterVerifyAndCAS(t *testing.T) {
	source := &fakeMigrationSource{data: []byte("asset")}
	destination := &fakeMigrationDestination{uploadURL: "https://static.example.com/file.png"}
	status, err := migrateAttachment(
		context.Background(),
		attachmentRow{ID: "attachment-1", URL: "/uploads/file.png"},
		true,
		true,
		"",
		source,
		destination,
		func(context.Context, attachmentRow, string) (bool, error) {
			if len(destination.verifyKeys) != 1 {
				t.Fatal("database update happened before OSS verification")
			}
			return true, nil
		},
	)
	if err != nil || status != statusLocalDeleted {
		t.Fatalf("migrateAttachment() = (%q, %v)", status, err)
	}
	if !reflect.DeepEqual(source.deleteCalls, []string{"file.png"}) {
		t.Fatalf("delete calls = %#v", source.deleteCalls)
	}
}
