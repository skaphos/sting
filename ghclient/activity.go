// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/skaphos/sting/internal/activity"
	"github.com/skaphos/sting/internal/apibudget"
	"github.com/skaphos/sting/internal/patch"
	"github.com/skaphos/sting/model"
	"golang.org/x/sync/errgroup"
)

// providerFileCap is the number of files GitHub returns from a comparison
// before truncating. Hitting it means the change set is incomplete and must say
// so rather than read as exhaustive.
const providerFileCap = 300

// CollectActivity gathers a repository's activity for a window: the commits,
// the aggregate change set between the window's boundary states, the
// correlations between them, and what the whole thing cost.
//
// It returns a populated ActivityResult on every non-fatal path — budget stops,
// quota exhaustion, and ancestry divergence all produce a result plus a
// disclosure. The error return is reserved for failures that occur before
// evidence gathering begins, because discarding gathered evidence to report a
// bound is the blindness Constitution VI forbids.
func (c *Client) CollectActivity(ctx context.Context, q model.ActivityQuery) (model.ActivityResult, error) {
	owner, repo, ok := splitRepo(q.Repo)
	if !ok {
		return model.ActivityResult{}, fmt.Errorf("invalid repo %q (want owner/name)", q.Repo)
	}

	res := c.newActivityResult(q)

	// A quota that is already exhausted means no evidence can be gathered at
	// all. Reporting that up front — from an endpoint that costs nothing —
	// beats spending requests to discover it (FR-016).
	if exhausted, resetsAt := c.preflightQuota(ctx); exhausted {
		var stamp string
		if !resetsAt.IsZero() {
			stamp = resetsAt.Format(time.RFC3339)
		}
		res.Disclosures = append(res.Disclosures, activity.QuotaExhausted(stamp))
		res.Cost = c.Cost()
		if !resetsAt.IsZero() {
			res.Cost.QuotaResetsAt = resetsAt
		}
		return res, nil
	}

	// Estimate-only stops here: report the projected cost and gather nothing.
	if q.EstimateOnly {
		report, err := c.EstimateActivity(ctx, q)
		res.Cost = report
		if err != nil {
			if stop, d := c.classifyStop(err); stop {
				res.Disclosures = append(res.Disclosures, d)
				return res, nil
			}
			return model.ActivityResult{}, err
		}
		return res, nil
	}

	// Resolve the reference before anything else so the result can name what it
	// actually compared. An empty Ref means the repository's default branch,
	// and echoing "" back would leave the reader unable to re-derive the query.
	ref, err := c.resolveRef(ctx, owner, repo, q.Ref)
	if err != nil {
		// A budget or quota stop here is still a result: nothing was gathered,
		// but the cost report and the reason are worth returning.
		if stop, d := c.classifyStop(err); stop {
			res.Ref = q.Ref
			res.Disclosures = append(res.Disclosures, d)
			res.Cost = c.Cost()
			return res, nil
		}
		return model.ActivityResult{}, err
	}
	res.Ref = ref

	commits, listErr := c.listWindowCommits(ctx, owner, repo, q, ref)
	stopped, stopDisclosure := c.classifyStop(listErr)
	if listErr != nil && !stopped {
		return model.ActivityResult{}, listErr
	}

	res.Commits = commits
	res.Count = len(commits)
	res.Boundaries = resolveBoundaries(commits)

	var (
		disclosureInput = activity.DisclosureInput{
			Ref:            ref,
			AuthorFilter:   q.Author,
			RootCommitBase: res.Boundaries.BaseSource == model.BaseSourceRepositoryRoot,
		}
		changeSetErr error
	)

	// Enrich before comparing so that a budget stop during enrichment still
	// leaves capacity reserved for the change set, and so correlations can use
	// whatever observation was actually obtained.
	if !stopped && len(commits) > 0 && q.EnrichCommits > 0 {
		delivered, enrichErr := c.enrichActivityCommits(ctx, res.Commits, q)
		if enrichErr != nil {
			if s, d := c.classifyStop(enrichErr); s {
				stopped, stopDisclosure = true, d
			} else {
				return model.ActivityResult{}, enrichErr
			}
		}
		if requested := min(q.EnrichCommits, len(commits)); delivered < requested {
			res.Disclosures = append(res.Disclosures, activity.EnrichmentPartial(delivered, requested))
		}
	}

	// Only compare when there is something to compare. An empty window is a
	// legitimate, empty answer rather than an error, and a budget stop during
	// listing means the boundaries are not trustworthy enough to compare from.
	if !stopped && len(commits) > 0 {
		var cs model.ChangeSet
		var status string
		earliestSHA := commits[len(commits)-1].SHA
		cs, status, changeSetErr = c.compareBoundaries(ctx, owner, repo, res.Boundaries, earliestSHA, q)
		if changeSetErr != nil {
			if s, d := c.classifyStop(changeSetErr); s {
				stopped, stopDisclosure = true, d
			} else {
				return model.ActivityResult{}, changeSetErr
			}
		} else {
			res.Boundaries.Status = status
			res.Boundaries.SharedRoot = status != model.StatusDiverged
			if status == model.StatusDiverged {
				// Divergence means a net comparison would be meaningless, so
				// the change set is suppressed rather than rendered as fact.
				disclosureInput.Diverged = true
			} else {
				res.ChangeSet = cs
				disclosureInput.ChangeSetProduced = true
				disclosureInput.ProviderCapped = cs.Truncated
				disclosureInput.PatchTruncated = anyPatchTruncated(cs)
				// Correlations are derived from what was actually gathered, so
				// they can only claim observation for commits that really were
				// enriched.
				res.Correlations = activity.Correlate(cs.Paths, res.Commits)
			}
		}
	}

	res.Disclosures = append(res.Disclosures, activity.Build(disclosureInput)...)
	if stopped {
		res.Disclosures = append(res.Disclosures, stopDisclosure)
	}
	res.Cost = c.Cost()
	return res, nil
}

// EstimateActivity reports the projected cost of a query without gathering its
// evidence.
//
// It issues exactly one probe: a per_page=1 list-commits request whose Link
// header yields LastPage, which with a page size of 1 *is* the exact number of
// commits in the window (research R4). That turns the estimate from a heuristic
// into arithmetic. The probe itself is counted in the reported cost — hiding it
// would make the accounting dishonest about its own overhead.
//
// The probe gathers no evidence: the single commit it returns is discarded and
// only the Link header is read.
func (c *Client) EstimateActivity(ctx context.Context, q model.ActivityQuery) (model.CostReport, error) {
	owner, repo, ok := splitRepo(q.Repo)
	if !ok {
		return c.Cost(), fmt.Errorf("invalid repo %q (want owner/name)", q.Repo)
	}

	commits, err := c.probeCommitCount(ctx, owner, repo, q)
	if err != nil {
		// Report what the attempt cost even when it failed.
		return c.Cost(), err
	}

	report := c.Cost()
	report.Estimated = estimateRequests(commits, c.perPage, q)
	report.Ceiling = q.MaxRequests
	return report, nil
}

// probeCommitCount returns the exact number of commits in the window using one
// request.
func (c *Client) probeCommitCount(ctx context.Context, owner, repo string, q model.ActivityQuery) (int, error) {
	opts := &github.CommitsListOptions{
		SHA:         q.Ref,
		Author:      q.Author,
		Since:       q.Since,
		Until:       q.Until,
		ListOptions: github.ListOptions{PerPage: 1},
	}
	commits, resp, err := c.gh.Repositories.ListCommits(ctx, owner, repo, opts)
	if err != nil {
		return 0, apiError(fmt.Sprintf("estimate commits %s/%s", owner, repo), err)
	}
	// With per_page=1 the last page number is the commit count. A window that
	// fits on one page carries no Link header, so LastPage is 0 and the count
	// is simply what came back.
	if resp != nil && resp.LastPage > 0 {
		return resp.LastPage, nil
	}
	return len(commits), nil
}

// estimateRequests is the documented cost formula:
//
//	1 (probe) + ceil(commits/per_page) + 1 (comparison) + enrichment subset
//
// plus one reference lookup when the query left the reference implicit. Every
// term is exact, so the only source of drift is upstream state changing between
// the estimate and the run.
func estimateRequests(commits, perPage int, q model.ActivityQuery) int {
	if perPage < 1 {
		perPage = 100
	}
	estimate := 1 // the probe itself
	if q.Ref == "" {
		estimate++ // default-branch lookup
	}
	if commits > 0 {
		estimate += (commits + perPage - 1) / perPage // listing pages
		estimate++                                    // one boundary comparison
	}
	enrich := min(q.EnrichCommits, commits)
	if enrich > 0 {
		estimate += enrich
	}
	return estimate
}

// preflightQuota reports whether the provider's quota is already exhausted
// before any evidence gathering begins (FR-016).
//
// GitHub's rate-limit endpoint does not count against the core quota, which
// makes this check genuinely free — it is worth knowing that a query cannot
// succeed before spending requests discovering it.
func (c *Client) preflightQuota(ctx context.Context) (exhausted bool, resetsAt time.Time) {
	// GitHub does not charge core quota for /rate_limit, so it must not be
	// charged against the caller's ceiling either — otherwise a small explicit
	// ceiling would be spent before any evidence could be gathered.
	limits, _, err := c.gh.RateLimit.Get(apibudget.Unmetered(ctx))
	if err != nil || limits == nil {
		// A failed pre-flight is not itself a failure: fall through and let the
		// real requests report the truth.
		return false, time.Time{}
	}
	core := limits.GetCore()
	if core == nil {
		return false, time.Time{}
	}
	if core.Remaining <= 0 {
		return true, core.Reset.UTC()
	}
	return false, time.Time{}
}

// newActivityResult seeds the invariant fields every result carries, so no
// early-return path can produce one missing its schema version or its echo of
// the resolved query.
func (c *Client) newActivityResult(q model.ActivityQuery) model.ActivityResult {
	return model.ActivityResult{
		SchemaVersion: model.ActivitySchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Provider:      model.ProviderGitHub,
		Repo:          q.Repo,
		Ref:           q.Ref,
		Since:         q.Since,
		Until:         q.Until,
		// GitHub's list-commits since/until filter on the committer date,
		// verified against the live API (research R2). Stating it keeps the
		// boundary meaningful in rebase-heavy repositories, where the author
		// and committer dates diverge.
		WindowDateBasis: model.WindowDateBasisCommitter,
		Commits:         []model.ActivityCommit{},
		ChangeSet:       model.ChangeSet{Paths: []model.ChangedPath{}},
		Cost:            c.Cost(),
	}
}

// classifyStop reports whether err is a budget or quota stop — a bound rather
// than a failure — and returns the disclosure that explains it. A stop returns a
// populated result; only a genuine failure returns an error.
func (c *Client) classifyStop(err error) (bool, model.Disclosure) {
	if err == nil {
		return false, model.Disclosure{}
	}
	if errors.Is(err, apibudget.ErrBudgetExceeded) {
		cost := c.Cost()
		return true, activity.BudgetBounded(cost.Consumed, cost.Ceiling)
	}
	if isRateLimitError(err) {
		var resetsAt string
		if r := c.Cost().QuotaResetsAt; !r.IsZero() {
			resetsAt = r.Format(time.RFC3339)
		}
		return true, activity.QuotaExhausted(resetsAt)
	}
	return false, model.Disclosure{}
}

// isRateLimitError reports whether err is a provider rate limit in any of the
// shapes go-github produces, including the 403 that it does not map to a typed
// rate-limit error.
func isRateLimitError(err error) bool {
	var rl *github.RateLimitError
	if errors.As(err, &rl) {
		return true
	}
	var ab *github.AbuseRateLimitError
	if errors.As(err, &ab) {
		return true
	}
	var er *github.ErrorResponse
	if errors.As(err, &er) && er.Response != nil &&
		er.Response.StatusCode == 403 && isRateLimited(er) {
		return true
	}
	return false
}

// resolveRef returns the reference to examine, filling in the repository's
// default branch when the query did not name one. The lookup costs one request
// and only happens when the caller left the reference implicit.
func (c *Client) resolveRef(ctx context.Context, owner, repo, ref string) (string, error) {
	if ref != "" {
		return ref, nil
	}
	r, _, err := c.gh.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return "", apiError(fmt.Sprintf("get repository %s/%s", owner, repo), err)
	}
	if b := r.GetDefaultBranch(); b != "" {
		return b, nil
	}
	return "", fmt.Errorf("repository %s/%s reports no default branch", owner, repo)
}

// listWindowCommits paginates the window's commits, newest first, preserving
// the provider's ordering. The cost grows with commit *pages*, not with commit
// count — which is the whole point of the digest.
func (c *Client) listWindowCommits(ctx context.Context, owner, repo string, q model.ActivityQuery, ref string) ([]model.ActivityCommit, error) {
	opts := &github.CommitsListOptions{
		SHA:         ref,
		Author:      q.Author,
		Since:       q.Since,
		Until:       q.Until,
		ListOptions: github.ListOptions{PerPage: c.perPage},
	}
	full := owner + "/" + repo
	out := []model.ActivityCommit{}

	for page := 1; ; page++ {
		if page > maxPages {
			return out, fmt.Errorf("list commits %s: exceeded max pages (%d)", full, maxPages)
		}
		commits, resp, err := c.gh.Repositories.ListCommits(ctx, owner, repo, opts)
		if err != nil {
			// Return what was gathered alongside the error: the caller decides
			// whether this is a bound (partial result) or a failure.
			return out, apiError("list commits "+full, err)
		}
		for _, rc := range commits {
			out = append(out, fromRepoActivityCommit(full, rc))
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return out, nil
}

// resolveBoundaries derives the comparison endpoints from the window's commits.
//
// Resolution is by ancestry, never by timestamp proximity: the base is the
// first parent of the *earliest* in-window commit, which is the reference's
// state immediately before the window. Choosing the earliest in-window commit
// itself would omit its own changes — the off-by-one that would under-report
// every window.
//
// First-parent is deliberate when the earliest commit is a merge: it follows
// the mainline of the compared reference rather than the tip of a merged side
// branch.
func resolveBoundaries(commits []model.ActivityCommit) model.Boundaries {
	// An empty window is a legitimate answer, not an error: base equals head
	// and the comparison is identical.
	if len(commits) == 0 {
		return model.Boundaries{
			Status:     model.StatusIdentical,
			SharedRoot: true,
		}
	}

	// The provider returns newest first, so the earliest in-window commit is
	// the last element.
	head := commits[0]
	earliest := commits[len(commits)-1]

	b := model.Boundaries{
		HeadSHA:    head.SHA,
		SharedRoot: true,
	}
	if len(earliest.ParentSHAs) > 0 {
		b.BaseSHA = earliest.ParentSHAs[0]
		b.BaseSource = model.BaseSourceParentOfEarliest
	} else {
		// The earliest in-window commit is the repository's root: there is no
		// parent to compare against, so the comparison starts at the root
		// itself and a disclosure explains what that excludes.
		b.BaseSHA = ""
		b.BaseSource = model.BaseSourceRepositoryRoot
	}
	return b
}

// compareBoundaries derives the aggregate change set from a single comparison
// call, independent of how many commits the window holds.
//
// ListOptions paginates the comparison's *commits*, not its files, so PerPage:1
// minimizes the payload while leaving the file list intact (research R7). The
// commit list is ignored outright — it already came from the cheaper listing
// path.
// earliestSHA is the SHA of the earliest in-window commit. It is needed
// separately from Boundaries.BaseSHA because a root-commit window reports an
// empty base (there is no parent) but must still compare *from* that commit.
func (c *Client) compareBoundaries(ctx context.Context, owner, repo string, b model.Boundaries, earliestSHA string, q model.ActivityQuery) (model.ChangeSet, string, error) {
	base := b.BaseSHA
	if base == "" {
		// Root-commit window: there is no parent to start from, so the
		// comparison runs from the root commit itself. Its own contents are
		// outside the comparison, which the disclosure states.
		//
		// Substituting the head here instead would compare the head against
		// itself and silently report an empty change set for every window that
		// reaches the repository's first commit.
		base = earliestSHA
	}
	if base == "" || base == b.HeadSHA {
		// Nothing to compare — an empty window, or a lone root commit whose own
		// contents are outside a net comparison by definition.
		return model.ChangeSet{Paths: []model.ChangedPath{}}, model.StatusIdentical, nil
	}

	cmp, _, err := c.gh.Repositories.CompareCommits(ctx, owner, repo, base, b.HeadSHA,
		&github.ListOptions{PerPage: 1})
	if err != nil {
		return model.ChangeSet{}, "", apiError(
			fmt.Sprintf("compare %s/%s %s...%s", owner, repo, base, b.HeadSHA), err)
	}

	status := cmp.GetStatus()
	if status == model.StatusDiverged {
		return model.ChangeSet{Paths: []model.ChangedPath{}}, status, nil
	}
	return changeSetFromFiles(cmp.Files, q), status, nil
}

// changeSetFromFiles maps a comparison's file list into the change set,
// including renames, and sorts by path so identical upstream state yields
// byte-identical output — provider ordering is not guaranteed stable.
func changeSetFromFiles(files []*github.CommitFile, q model.ActivityQuery) model.ChangeSet {
	cs := model.ChangeSet{Paths: make([]model.ChangedPath, 0, len(files))}
	budget := q.MaxDiffBytes
	if budget <= 0 {
		budget = model.DefaultMaxDiffBytes
	}

	for _, f := range files {
		cp := model.ChangedPath{
			Path:         f.GetFilename(),
			PreviousPath: f.GetPreviousFilename(),
			Status:       f.GetStatus(),
			Additions:    f.GetAdditions(),
			Deletions:    f.GetDeletions(),
		}
		if q.IncludeDiffs {
			cp.Patch, cp.PatchTruncated, budget = patch.ConsumePatchBudget(f.GetPatch(), budget)
		}
		cs.TotalAdditions += cp.Additions
		cs.TotalDeletions += cp.Deletions
		cs.Paths = append(cs.Paths, cp)
	}

	sort.SliceStable(cs.Paths, func(i, j int) bool { return cs.Paths[i].Path < cs.Paths[j].Path })

	// The provider caps a comparison's file list; hitting the cap exactly is
	// the only signal available that it was clipped.
	if len(files) >= providerFileCap {
		cs.Truncated = true
	}
	return cs
}

// enrichActivityCommits fetches per-commit file data for the first n commits in
// the result's existing order, which is what turns inferred attribution into
// observed attribution. It returns how many were actually enriched.
//
// Two rules keep this deterministic, and both matter (research R3):
//
//  1. **Check before dispatch.** The budget is asked how much capacity remains
//     and only a batch that can be fully afforded is dispatched, rather than
//     firing requests and letting the losers fail. If the ceiling were enforced
//     purely inside the transport, *which* requests won the race would decide
//     which commits got enriched.
//  2. **Clip by commit order, not completion order.** When capacity is short,
//     the first n commits in the existing deterministic order are enriched and
//     the rest are left alone.
//
// Without these, the same query against unchanged upstream state could return
// different results run to run.
func (c *Client) enrichActivityCommits(ctx context.Context, commits []model.ActivityCommit, q model.ActivityQuery) (int, error) {
	requested := min(q.EnrichCommits, len(commits))
	if requested <= 0 {
		return 0, nil
	}

	// Reserve capacity for the comparison that still has to happen, so
	// enrichment cannot starve the change set.
	const reserveForComparison = 1
	affordable := c.budgetRemaining() - reserveForComparison
	if affordable < 0 {
		affordable = 0
	}
	subset := min(requested, affordable)
	if subset <= 0 {
		return 0, nil
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(c.concurrency)
	for i := range subset {
		cm := &commits[i]
		g.Go(func() error {
			owner, repo, ok := splitRepo(cm.Repo)
			if !ok {
				return fmt.Errorf("invalid repo %q", cm.Repo)
			}
			files, err := c.commitFiles(gctx, owner, repo, cm.SHA, q)
			if err != nil {
				return err
			}
			cm.Files = files
			// Enriched is set only after a successful fetch: it is the
			// precondition for any observed attribution naming this commit.
			cm.Enriched = true
			return nil
		})
	}
	err := g.Wait()

	delivered := 0
	for i := range subset {
		if commits[i].Enriched {
			delivered++
		}
	}
	return delivered, err
}

// commitFiles fetches one commit's file list.
func (c *Client) commitFiles(ctx context.Context, owner, repo, sha string, q model.ActivityQuery) ([]model.File, error) {
	opts := &github.ListOptions{PerPage: c.perPage}
	var files []*github.CommitFile
	for page := 1; ; page++ {
		if page > maxPages {
			return nil, fmt.Errorf("get commit details %s/%s@%s: exceeded max pages (%d)", owner, repo, sha, maxPages)
		}
		rc, resp, err := c.gh.Repositories.GetCommit(ctx, owner, repo, sha, opts)
		if err != nil {
			return nil, apiError(fmt.Sprintf("get commit details %s/%s@%s", owner, repo, sha), err)
		}
		files = append(files, rc.Files...)
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Patch text is deliberately not carried on enriched commits: the change
	// set already bounds and reports patches, and duplicating them here would
	// multiply output for no extra evidence.
	out := make([]model.File, 0, len(files))
	for _, f := range files {
		out = append(out, model.File{
			Path:         f.GetFilename(),
			PreviousPath: f.GetPreviousFilename(),
			Status:       f.GetStatus(),
			Additions:    f.GetAdditions(),
			Deletions:    f.GetDeletions(),
			Changes:      f.GetChanges(),
		})
	}
	return out, nil
}

func anyPatchTruncated(cs model.ChangeSet) bool {
	for _, p := range cs.Paths {
		if p.PatchTruncated {
			return true
		}
	}
	return false
}
