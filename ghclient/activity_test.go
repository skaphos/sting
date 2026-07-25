// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

// activityWindow is the window every fixture-backed test below uses.
func activityWindow() (time.Time, time.Time) {
	return time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
}

func activityQuery(repo string) model.ActivityQuery {
	since, until := activityWindow()
	return model.ActivityQuery{
		Provider: model.ProviderGitHub,
		Repo:     repo,
		Since:    since,
		Until:    until,
	}
}

// windowCommit renders one list-commits entry. parents may be empty, which is
// how the provider represents a root commit.
func windowCommit(sha string, parents []string, authorDate, committerDate string) string {
	var p strings.Builder
	p.WriteString("[")
	for i, parent := range parents {
		if i > 0 {
			p.WriteString(",")
		}
		fmt.Fprintf(&p, `{"sha":%q}`, parent)
	}
	p.WriteString("]")

	return fmt.Sprintf(`{
      "sha": %q,
      "html_url": "https://example.com/c/%s",
      "author": {"login": "octocat"},
      "parents": %s,
      "commit": {
        "message": "feat(render): commit %s\n\nbody for %s",
        "author": {"name": "Octo Cat", "email": "octo@example.com", "date": %q},
        "committer": {"name": "Octo Cat", "email": "octo@example.com", "date": %q}
      }
    }`, sha, sha, p.String(), sha, sha, authorDate, committerDate)
}

// activityServer wires the three endpoints an activity query touches. Any
// handler left nil returns a 404, so an unexpected call is loud rather than
// silently absorbed.
type activityServer struct {
	repoBody    string
	commitsBody func(page string) (body string, nextPage int)
	compareBody func(base, head string) (status int, body string)

	repoHits      int
	commitHits    int
	compareHits   int
	detailHits    int
	rateLimitHits int
}

func (a *activityServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4990")

		switch {
		case strings.Contains(p, "/rate_limit"):
			// The pre-flight quota check. GitHub does not charge core quota for
			// it, so it is worth issuing before spending anything.
			a.rateLimitHits++
			_, _ = w.Write([]byte(`{"resources":{"core":{"limit":5000,"remaining":4990,"reset":1784000000}}}`))

		case strings.Contains(p, "/compare/"):
			a.compareHits++
			base, head := splitCompareRange(t, p)
			if a.compareBody == nil {
				t.Errorf("unexpected compare call %q", p)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			code, body := a.compareBody(base, head)
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))

		case strings.HasSuffix(p, "/commits"):
			a.commitHits++
			if a.commitsBody == nil {
				t.Errorf("unexpected list-commits call %q", p)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			body, next := a.commitsBody(r.URL.Query().Get("page"))
			if next > 0 {
				w.Header().Set("Link",
					fmt.Sprintf(`<https://example.invalid/x?page=%d>; rel="next"`, next))
			}
			_, _ = w.Write([]byte(body))

		case strings.Contains(p, "/commits/"):
			// Per-commit detail. US1 must never reach this.
			a.detailHits++
			fmt.Fprintf(w, `{"sha":%q,"files":[]}`, pathBase(p))

		default:
			a.repoHits++
			body := a.repoBody
			if body == "" {
				body = `{"default_branch":"main"}`
			}
			_, _ = w.Write([]byte(body))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func splitCompareRange(t *testing.T, path string) (string, string) {
	t.Helper()
	idx := strings.Index(path, "/compare/")
	rng := path[idx+len("/compare/"):]
	base, head, ok := strings.Cut(rng, "...")
	if !ok {
		t.Fatalf("malformed compare range %q", rng)
	}
	return base, head
}

func pathBase(p string) string {
	idx := strings.LastIndex(p, "/")
	return p[idx+1:]
}

func compareOK(status string, files ...string) string {
	return fmt.Sprintf(`{"status":%q,"ahead_by":3,"behind_by":0,"total_commits":3,"files":[%s]}`,
		status, strings.Join(files, ","))
}

func fileEntry(name, status string, add, del int) string {
	return fmt.Sprintf(`{"filename":%q,"status":%q,"additions":%d,"deletions":%d,"changes":%d,"patch":"@@ -1 +1 @@\n-a\n+b\n"}`,
		name, status, add, del, add+del)
}

// TestCollectActivityHappyPath covers the ordinary window: commits listed,
// boundaries resolved from the parent of the earliest commit, and a change set
// from one comparison.
func TestCollectActivityHappyPath(t *testing.T) {
	srv := &activityServer{
		commitsBody: func(string) (string, int) {
			return "[" + strings.Join([]string{
				windowCommit("head222", []string{"mid111"}, "2026-07-24T10:00:00Z", "2026-07-24T11:00:00Z"),
				windowCommit("mid111", []string{"early00"}, "2026-07-20T10:00:00Z", "2026-07-20T10:00:00Z"),
				windowCommit("early00", []string{"base999"}, "2026-07-19T10:00:00Z", "2026-07-19T10:00:00Z"),
			}, ",") + "]", 0
		},
		compareBody: func(base, head string) (int, string) {
			if base != "base999" {
				t.Errorf("compare base = %q, want base999 (the parent of the earliest in-window commit)", base)
			}
			if head != "head222" {
				t.Errorf("compare head = %q, want head222", head)
			}
			return http.StatusOK, compareOK("ahead",
				fileEntry("z_last.go", "modified", 5, 2),
				fileEntry("a_first.go", "added", 10, 0))
		},
	}
	ts := srv.start(t)
	c := newTestClient(t, ts.URL, 100)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}

	if res.SchemaVersion != model.ActivitySchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", res.SchemaVersion, model.ActivitySchemaVersion)
	}
	if res.Count != 3 || len(res.Commits) != 3 {
		t.Fatalf("Count = %d, len(Commits) = %d, want 3/3", res.Count, len(res.Commits))
	}
	if res.Ref != "main" {
		t.Errorf("Ref = %q, want main (resolved default branch)", res.Ref)
	}
	if res.WindowDateBasis != model.WindowDateBasisCommitter {
		t.Errorf("WindowDateBasis = %q, want committer", res.WindowDateBasis)
	}

	// The boundary is the parent of the earliest commit, not the earliest
	// commit itself. Getting this wrong under-reports every window.
	if res.Boundaries.BaseSHA != "base999" {
		t.Errorf("BaseSHA = %q, want base999", res.Boundaries.BaseSHA)
	}
	if res.Boundaries.HeadSHA != "head222" {
		t.Errorf("HeadSHA = %q, want head222", res.Boundaries.HeadSHA)
	}
	if res.Boundaries.BaseSource != model.BaseSourceParentOfEarliest {
		t.Errorf("BaseSource = %q, want %q", res.Boundaries.BaseSource, model.BaseSourceParentOfEarliest)
	}
	if res.Boundaries.Status != model.StatusAhead {
		t.Errorf("Status = %q, want ahead", res.Boundaries.Status)
	}
	if !res.Boundaries.SharedRoot {
		t.Error("SharedRoot = false, want true for an ahead comparison")
	}

	// Paths are sorted lexicographically for determinism, not left in provider
	// order.
	if len(res.ChangeSet.Paths) != 2 {
		t.Fatalf("len(Paths) = %d, want 2", len(res.ChangeSet.Paths))
	}
	if res.ChangeSet.Paths[0].Path != "a_first.go" || res.ChangeSet.Paths[1].Path != "z_last.go" {
		t.Errorf("paths not sorted: %q, %q", res.ChangeSet.Paths[0].Path, res.ChangeSet.Paths[1].Path)
	}
	if res.ChangeSet.TotalAdditions != 15 || res.ChangeSet.TotalDeletions != 2 {
		t.Errorf("totals = +%d/-%d, want +15/-2", res.ChangeSet.TotalAdditions, res.ChangeSet.TotalDeletions)
	}

	// Parents and both dates come from the list response, at no extra cost.
	head := res.Commits[0]
	if len(head.ParentSHAs) != 1 || head.ParentSHAs[0] != "mid111" {
		t.Errorf("head ParentSHAs = %v, want [mid111]", head.ParentSHAs)
	}
	wantAuthor := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	wantCommitter := time.Date(2026, 7, 24, 11, 0, 0, 0, time.UTC)
	if !head.AuthorDate.Equal(wantAuthor) {
		t.Errorf("AuthorDate = %v, want %v", head.AuthorDate, wantAuthor)
	}
	if !head.CommitterDate.Equal(wantCommitter) {
		t.Errorf("CommitterDate = %v, want %v", head.CommitterDate, wantCommitter)
	}
	if head.Message == "" || !strings.Contains(head.Message, "body for head222") {
		t.Errorf("Message = %q, want the full body preserved", head.Message)
	}

	// No commit was enriched, so nothing may claim observed provenance.
	for _, cm := range res.Commits {
		if cm.Enriched {
			t.Errorf("commit %s marked Enriched without a detail fetch", cm.SHA)
		}
	}
	if srv.detailHits != 0 {
		t.Errorf("per-commit detail requests = %d, want 0", srv.detailHits)
	}
}

// TestCollectActivityUnconditionalDisclosures is FR-018: a clean result still
// carries the blind-spot statements. If these became conditional, a reader
// would treat the ordinary case as more complete than it is.
func TestCollectActivityUnconditionalDisclosures(t *testing.T) {
	srv := &activityServer{
		commitsBody: func(string) (string, int) {
			return "[" + windowCommit("h1", []string{"b1"}, "2026-07-20T10:00:00Z", "2026-07-20T10:00:00Z") + "]", 0
		},
		compareBody: func(string, string) (int, string) {
			return http.StatusOK, compareOK("ahead", fileEntry("a.go", "modified", 1, 1))
		},
	}
	ts := srv.start(t)
	c := newTestClient(t, ts.URL, 100)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}

	for _, want := range []string{model.DisclosureReferenceScoped, model.DisclosureNetComparisonBlindspot} {
		if !hasDisclosure(res.Disclosures, want) {
			t.Errorf("missing unconditional disclosure %q on a clean result; got %v",
				want, disclosureKinds(res.Disclosures))
		}
	}
	// Every disclosure must carry a reason: "bounded" with no explanation is a
	// defect, not a disclosure.
	for _, d := range res.Disclosures {
		if d.Reason == "" {
			t.Errorf("disclosure %q has no reason", d.Kind)
		}
	}
}

func TestCollectActivityEmptyWindow(t *testing.T) {
	srv := &activityServer{
		commitsBody: func(string) (string, int) { return "[]", 0 },
	}
	ts := srv.start(t)
	c := newTestClient(t, ts.URL, 100)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("an empty window must not be an error: %v", err)
	}
	if res.Count != 0 {
		t.Errorf("Count = %d, want 0", res.Count)
	}
	if res.Boundaries.Status != model.StatusIdentical {
		t.Errorf("Status = %q, want identical", res.Boundaries.Status)
	}
	if res.Boundaries.BaseSHA != res.Boundaries.HeadSHA {
		t.Errorf("base %q != head %q; an empty window compares a commit with itself",
			res.Boundaries.BaseSHA, res.Boundaries.HeadSHA)
	}
	if len(res.ChangeSet.Paths) != 0 {
		t.Errorf("len(Paths) = %d, want 0", len(res.ChangeSet.Paths))
	}
	// No comparison is worth issuing when there is nothing to compare.
	if srv.compareHits != 0 {
		t.Errorf("compare requests = %d, want 0 for an empty window", srv.compareHits)
	}
}

// TestCollectActivityRootCommitWindow covers FR-008b: the earliest in-window
// commit is the repository's root, so there is no parent to compare against.
func TestCollectActivityRootCommitWindow(t *testing.T) {
	srv := &activityServer{
		commitsBody: func(string) (string, int) {
			return "[" + strings.Join([]string{
				windowCommit("h1", []string{"root0"}, "2026-07-21T10:00:00Z", "2026-07-21T10:00:00Z"),
				windowCommit("root0", nil, "2026-07-19T10:00:00Z", "2026-07-19T10:00:00Z"),
			}, ",") + "]", 0
		},
		compareBody: func(base, head string) (int, string) {
			if base != "root0" {
				t.Errorf("compare base = %q, want the root commit itself", base)
			}
			if head != "h1" {
				t.Errorf("compare head = %q, want h1", head)
			}
			return http.StatusOK, compareOK("ahead", fileEntry("a.go", "added", 3, 0))
		},
	}
	ts := srv.start(t)
	c := newTestClient(t, ts.URL, 100)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	if res.Boundaries.BaseSource != model.BaseSourceRepositoryRoot {
		t.Errorf("BaseSource = %q, want %q", res.Boundaries.BaseSource, model.BaseSourceRepositoryRoot)
	}
	if res.Boundaries.BaseSHA != "" {
		t.Errorf("BaseSHA = %q, want empty for a root-commit window", res.Boundaries.BaseSHA)
	}

	// The comparison must actually run, starting from the root commit. Earlier
	// this substituted the head for the empty base, compared the head against
	// itself, and silently reported an empty change set for every window
	// reaching the repository's first commit — with the assertions above
	// passing regardless, because the compare handler was never called.
	if srv.compareHits != 1 {
		t.Fatalf("comparison requests = %d, want exactly 1: a root-commit window must "+
			"still be compared, not collapsed to an empty change set", srv.compareHits)
	}
	if len(res.ChangeSet.Paths) != 1 || res.ChangeSet.Paths[0].Path != "a.go" {
		t.Errorf("change set = %+v, want the root-commit window's files", res.ChangeSet.Paths)
	}
	if res.Boundaries.Status != model.StatusAhead {
		t.Errorf("Status = %q, want ahead", res.Boundaries.Status)
	}
	// The reader has to be told the root commit's own contents are outside the
	// comparison, or the change set reads as covering all of history.
	if !hasDisclosure(res.Disclosures, model.DisclosureNetComparisonBlindspot) {
		t.Errorf("root-commit window missing a blind-spot disclosure; got %v",
			disclosureKinds(res.Disclosures))
	}
}

// TestCollectActivityDiverged is FR-009: after a force-push or rebase inside
// the window the boundaries share no ancestry, so a net change set would be
// meaningless and must be suppressed rather than rendered as fact.
func TestCollectActivityDiverged(t *testing.T) {
	srv := &activityServer{
		commitsBody: func(string) (string, int) {
			return "[" + windowCommit("h1", []string{"b1"}, "2026-07-20T10:00:00Z", "2026-07-20T10:00:00Z") + "]", 0
		},
		compareBody: func(string, string) (int, string) {
			return http.StatusOK, compareOK("diverged", fileEntry("a.go", "modified", 9, 9))
		},
	}
	ts := srv.start(t)
	c := newTestClient(t, ts.URL, 100)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("divergence must not be an error: %v", err)
	}
	if res.Boundaries.Status != model.StatusDiverged {
		t.Errorf("Status = %q, want diverged", res.Boundaries.Status)
	}
	if res.Boundaries.SharedRoot {
		t.Error("SharedRoot = true for a diverged comparison")
	}
	if len(res.ChangeSet.Paths) != 0 {
		t.Errorf("len(Paths) = %d, want 0: a diverged change set must be suppressed", len(res.ChangeSet.Paths))
	}
	if !hasDisclosure(res.Disclosures, model.DisclosureAncestryDiverged) {
		t.Errorf("missing ancestry-diverged disclosure; got %v", disclosureKinds(res.Disclosures))
	}
	// The commits themselves are still real evidence and must survive.
	if res.Count != 1 {
		t.Errorf("Count = %d, want 1: divergence suppresses the change set, not the commits", res.Count)
	}
}

// TestCollectActivityProviderFileCap covers FR-019: a comparison clipped at the
// provider's 300-file cap must say so rather than read as exhaustive.
func TestCollectActivityProviderFileCap(t *testing.T) {
	files := make([]string, 0, providerFileCap)
	for i := range providerFileCap {
		files = append(files, fileEntry(fmt.Sprintf("f%03d.go", i), "modified", 1, 1))
	}
	srv := &activityServer{
		commitsBody: func(string) (string, int) {
			return "[" + windowCommit("h1", []string{"b1"}, "2026-07-20T10:00:00Z", "2026-07-20T10:00:00Z") + "]", 0
		},
		compareBody: func(string, string) (int, string) {
			return http.StatusOK, compareOK("ahead", files...)
		},
	}
	ts := srv.start(t)
	c := newTestClient(t, ts.URL, 100)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	if !res.ChangeSet.Truncated {
		t.Error("ChangeSet.Truncated = false at the provider file cap")
	}
	if !hasDisclosure(res.Disclosures, model.DisclosureProviderCapped) {
		t.Errorf("missing provider-capped disclosure; got %v", disclosureKinds(res.Disclosures))
	}
}

// TestCollectActivityPermissionDeniedIsFatal distinguishes the two 403s: a
// genuine permission denial cannot produce evidence, so it is an error rather
// than a bounded result.
func TestCollectActivityPermissionDeniedIsFatal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/commits") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Must have push access to view this"}`))
			return
		}
		_, _ = w.Write([]byte(`{"default_branch":"main"}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := newTestClient(t, ts.URL, 100)
	_, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err == nil {
		t.Fatal("a permission denial must be an error, not an empty result")
	}
	if !strings.Contains(err.Error(), "list commits") {
		t.Errorf("error = %v, want it to name the failing operation", err)
	}
}

// TestCollectActivityRateLimitIsBounded is the other 403: a rate limit is a
// bound, so the gathered evidence is returned with a disclosure instead of
// being discarded.
func TestCollectActivityRateLimitIsBounded(t *testing.T) {
	page := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/commits"):
			page++
			if page == 1 {
				w.Header().Set("X-RateLimit-Limit", "5000")
				w.Header().Set("X-RateLimit-Remaining", "1")
				w.Header().Set("Link", `<https://example.invalid/x?page=2>; rel="next"`)
				_, _ = w.Write([]byte("[" +
					windowCommit("h1", []string{"b1"}, "2026-07-20T10:00:00Z", "2026-07-20T10:00:00Z") + "]"))
				return
			}
			w.Header().Set("X-RateLimit-Limit", "5000")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1784000000")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
		default:
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		}
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c, err := New("test-token", ts.URL+"/", 100, WithRequestBudget(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("a rate limit must return partial evidence, not an error: %v", err)
	}
	if res.Count != 1 {
		t.Errorf("Count = %d, want 1: the first page of commits must survive the stop", res.Count)
	}
	if !hasDisclosure(res.Disclosures, model.DisclosureQuotaExhausted) {
		t.Errorf("missing quota-exhausted disclosure; got %v", disclosureKinds(res.Disclosures))
	}
}

func TestCollectActivityInvalidRepo(t *testing.T) {
	c := newTestClient(t, "http://example.invalid", 100)
	q := activityQuery("not-a-repo")
	if _, err := c.CollectActivity(context.Background(), q); err == nil {
		t.Fatal("expected an error for a malformed repo")
	}
}

// TestCollectActivityExplicitRefSkipsLookup: naming a ref means the default
// branch lookup is unnecessary, so it must not be issued.
func TestCollectActivityExplicitRefSkipsLookup(t *testing.T) {
	srv := &activityServer{
		commitsBody: func(string) (string, int) { return "[]", 0 },
	}
	ts := srv.start(t)
	c := newTestClient(t, ts.URL, 100)

	q := activityQuery("skaphos/sting")
	q.Ref = "release/1.x"
	res, err := c.CollectActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	if res.Ref != "release/1.x" {
		t.Errorf("Ref = %q, want release/1.x", res.Ref)
	}
	if srv.repoHits != 0 {
		t.Errorf("repository lookups = %d, want 0 when a ref is given", srv.repoHits)
	}
}

// TestCollectActivityAuthorFilterDisclosed covers the asymmetry: the commit
// list can be author-filtered but the change set cannot, and hiding that would
// let a reader attribute every changed path to one person.
func TestCollectActivityAuthorFilterDisclosed(t *testing.T) {
	srv := &activityServer{
		commitsBody: func(string) (string, int) {
			return "[" + windowCommit("h1", []string{"b1"}, "2026-07-20T10:00:00Z", "2026-07-20T10:00:00Z") + "]", 0
		},
		compareBody: func(string, string) (int, string) {
			return http.StatusOK, compareOK("ahead", fileEntry("a.go", "modified", 1, 1))
		},
	}
	ts := srv.start(t)
	c := newTestClient(t, ts.URL, 100)

	q := activityQuery("skaphos/sting")
	q.Author = "octocat"
	res, err := c.CollectActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	if !hasDisclosure(res.Disclosures, model.DisclosureAuthorFilterNotApplied) {
		t.Errorf("missing author-filter-not-applied disclosure; got %v", disclosureKinds(res.Disclosures))
	}
}

func TestCollectActivityRenamePreserved(t *testing.T) {
	srv := &activityServer{
		commitsBody: func(string) (string, int) {
			return "[" + windowCommit("h1", []string{"b1"}, "2026-07-20T10:00:00Z", "2026-07-20T10:00:00Z") + "]", 0
		},
		compareBody: func(string, string) (int, string) {
			return http.StatusOK, `{"status":"ahead","files":[
				{"filename":"new/path.go","previous_filename":"old/path.go","status":"renamed","additions":1,"deletions":1,"changes":2}
			]}`
		},
	}
	ts := srv.start(t)
	c := newTestClient(t, ts.URL, 100)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	if len(res.ChangeSet.Paths) != 1 {
		t.Fatalf("len(Paths) = %d, want 1", len(res.ChangeSet.Paths))
	}
	p := res.ChangeSet.Paths[0]
	if p.PreviousPath != "old/path.go" || p.Path != "new/path.go" || p.Status != "renamed" {
		t.Errorf("rename not preserved: %+v", p)
	}
}

// TestCollectActivityDeterministic is FR-023: identical upstream state must
// yield byte-identical output apart from the generation timestamp.
func TestCollectActivityDeterministic(t *testing.T) {
	files := []string{
		fileEntry("m.go", "modified", 1, 1),
		fileEntry("a.go", "added", 2, 0),
		fileEntry("z.go", "removed", 0, 3),
		fileEntry("b.go", "modified", 4, 4),
	}
	srv := &activityServer{
		commitsBody: func(string) (string, int) {
			return "[" + strings.Join([]string{
				windowCommit("h1", []string{"m1"}, "2026-07-22T10:00:00Z", "2026-07-22T10:00:00Z"),
				windowCommit("m1", []string{"b1"}, "2026-07-20T10:00:00Z", "2026-07-20T10:00:00Z"),
			}, ",") + "]", 0
		},
		compareBody: func(string, string) (int, string) {
			return http.StatusOK, compareOK("ahead", files...)
		},
	}
	ts := srv.start(t)
	c := newTestClient(t, ts.URL, 100)

	first, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	for i := range 5 {
		got, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if len(got.ChangeSet.Paths) != len(first.ChangeSet.Paths) {
			t.Fatalf("run %d path count differs", i)
		}
		for j := range got.ChangeSet.Paths {
			if got.ChangeSet.Paths[j].Path != first.ChangeSet.Paths[j].Path {
				t.Fatalf("run %d path %d = %q, want %q (ordering is not stable)",
					i, j, got.ChangeSet.Paths[j].Path, first.ChangeSet.Paths[j].Path)
			}
		}
		if len(got.Disclosures) != len(first.Disclosures) {
			t.Fatalf("run %d disclosure count = %d, want %d", i, len(got.Disclosures), len(first.Disclosures))
		}
	}
}

func TestCollectActivityPaginatesWindow(t *testing.T) {
	srv := &activityServer{
		commitsBody: func(page string) (string, int) {
			switch page {
			case "", "1":
				return "[" + windowCommit("h1", []string{"m1"}, "2026-07-24T10:00:00Z", "2026-07-24T10:00:00Z") + "]", 2
			default:
				return "[" + windowCommit("m1", []string{"b1"}, "2026-07-19T10:00:00Z", "2026-07-19T10:00:00Z") + "]", 0
			}
		},
		compareBody: func(base, head string) (int, string) {
			if base != "b1" || head != "h1" {
				t.Errorf("compare %s...%s, want b1...h1 across pages", base, head)
			}
			return http.StatusOK, compareOK("ahead", fileEntry("a.go", "modified", 1, 1))
		},
	}
	ts := srv.start(t)
	c := newTestClient(t, ts.URL, 100)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	if res.Count != 2 {
		t.Errorf("Count = %d, want 2 across two pages", res.Count)
	}
	if srv.commitHits != 2 {
		t.Errorf("list-commits requests = %d, want 2", srv.commitHits)
	}
}

func hasDisclosure(ds []model.Disclosure, kind string) bool {
	for _, d := range ds {
		if d.Kind == kind {
			return true
		}
	}
	return false
}

func disclosureKinds(ds []model.Disclosure) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Kind)
	}
	return out
}
