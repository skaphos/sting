// SPDX-License-Identifier: MIT
package model

import (
	"strings"
	"time"
)

// ActivitySchemaVersion pins the ActivityResult contract, independently of
// SchemaVersion which pins Result. Bump it on any breaking change to
// ActivityResult or the types it contains. The two versions are deliberately
// separate: a repository-activity change must not force downstream consumers
// that only read Result (e.g. a Wake evidence adapter) to re-pin.
const ActivitySchemaVersion = "sting.activity.skaphos.io/v1"

// DefaultMaxRequests bounds the provider requests a single query may consume.
// It is a fixed constant by design: deriving it from remaining quota would make
// the same request return different results on different runs, breaking the
// determinism the evidence contract depends on.
const DefaultMaxRequests = 500

// Window date basis values for ActivityResult.WindowDateBasis. GitHub's
// list-commits since/until filter on the committer date, which diverges from
// the author date after a rebase, cherry-pick, or amend; the result states
// which date bounded the window rather than leaving it implicit.
const (
	WindowDateBasisCommitter = "committer"
	WindowDateBasisAuthor    = "author"
)

// Base sources for Boundaries.BaseSource.
const (
	// BaseSourceParentOfEarliest is the ordinary case: the comparison base is
	// the first parent of the earliest in-window commit.
	BaseSourceParentOfEarliest = "parent-of-earliest"
	// BaseSourceRepositoryRoot means the earliest in-window commit is the
	// repository's root commit, so there is no parent to compare against and
	// the change set covers everything since inception.
	BaseSourceRepositoryRoot = "repository-root"
)

// Comparison statuses reported by the provider for a boundary comparison.
const (
	StatusAhead     = "ahead"
	StatusIdentical = "identical"
	StatusDiverged  = "diverged"
	StatusBehind    = "behind"
)

// Correlation bases. A correlation MUST NOT carry BasisObserved unless
// ActivityCommit.Enriched is true for every SHA it names.
const (
	// BasisObserved means per-commit file data was actually fetched and lists
	// the path. Certain.
	BasisObserved = "observed"
	// BasisInferred means a declared rule produced the association; Rule names
	// which one.
	BasisInferred = "inferred"
)

// Correlation rules, recorded in Correlation.Rule for inferred bases.
const (
	// RulePathMention fires when the commit message body contains the path
	// verbatim. Strong.
	RulePathMention = "path-mention"
	// RuleScopeMatch fires when a Conventional Commit scope matches a leading
	// path segment. Weak: a scope names a component, not a path, and the two
	// only usually coincide.
	RuleScopeMatch = "scope-match"
)

// Disclosure kinds. The two marked unconditional below are emitted whenever a
// change set is produced, including on a completely clean result: a boundary
// comparison always carries the blind spots, so stating them only when
// something went wrong would overstate the ordinary case.
const (
	// DisclosureBudgetBounded: the request ceiling stopped the query.
	DisclosureBudgetBounded = "budget-bounded"
	// DisclosureQuotaExhausted: the provider rate limit stopped the query.
	DisclosureQuotaExhausted = "quota-exhausted"
	// DisclosureProviderCapped: the comparison hit the provider's file cap.
	DisclosureProviderCapped = "provider-capped"
	// DisclosurePatchTruncated: patch text exceeded MaxDiffBytes.
	DisclosurePatchTruncated = "patch-truncated"
	// DisclosureAncestryDiverged: the boundaries do not share ancestry.
	DisclosureAncestryDiverged = "ancestry-diverged"
	// DisclosureNetComparisonBlindspot is unconditional whenever a change set
	// is produced.
	DisclosureNetComparisonBlindspot = "net-comparison-blindspot"
	// DisclosureReferenceScoped is unconditional whenever a change set is
	// produced.
	DisclosureReferenceScoped = "reference-scoped"
	// DisclosureAuthorFilterNotApplied: an author filter narrowed the commit
	// list but cannot narrow the change set.
	DisclosureAuthorFilterNotApplied = "author-filter-not-applied"
	// DisclosureEnrichmentPartial: the enrichment subset delivered was smaller
	// than the one requested.
	DisclosureEnrichmentPartial = "enrichment-partial"
)

// ActivityQuery is the resolved, normalized repository-activity request. It is
// produced once by config.ResolveActivity and never mutated downstream, so the
// window is normalized at exactly one boundary.
type ActivityQuery struct {
	// Provider is the source control provider. Only ProviderGitHub is
	// supported; anything else is rejected at resolve time.
	Provider Provider
	// Repo is the "owner/name" target.
	Repo string
	// Ref is the branch or tag to examine. Empty means the repository's
	// default branch.
	Ref string
	// Since and Until bound the window, normalized to UTC.
	Since time.Time
	Until time.Time
	// Author optionally narrows the commit listing. The change set is not
	// author-filtered — a boundary comparison has no notion of authorship —
	// and a disclosure says so whenever both are present.
	Author string
	// IncludeDiffs requests bounded patch text in the change set.
	IncludeDiffs bool
	// MaxDiffBytes caps patch text; 0 uses DefaultMaxDiffBytes.
	MaxDiffBytes int
	// EnrichCommits is the size of the opt-in per-commit detail subset. It
	// costs one request per commit and is what enables observed (rather than
	// inferred) path attribution. 0 disables enrichment.
	EnrichCommits int
	// MaxRequests caps the provider requests this query may consume. 0
	// disables the ceiling; it defaults to DefaultMaxRequests.
	MaxRequests int
	// EstimateOnly reports projected cost and stops, gathering no evidence.
	EstimateOnly bool
}

// ActivityResult is a repository's activity over a window: the commits, the
// aggregate change set derived from comparing the window's boundary states, the
// correlations between them, and what the whole thing cost. Every field exists
// so the result can be re-derived and audited.
type ActivityResult struct {
	// SchemaVersion pins the ActivityResult contract; it is always
	// ActivitySchemaVersion and never the zero value.
	SchemaVersion string    `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	Provider      Provider  `json:"provider"`
	Repo          string    `json:"repo"`
	// Ref is the reference actually compared, never empty in output.
	Ref   string    `json:"ref"`
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`
	// WindowDateBasis names which commit date bounded the window: "committer"
	// or "author". GitHub filters on the committer date.
	WindowDateBasis string           `json:"window_date_basis"`
	Boundaries      Boundaries       `json:"boundaries"`
	Count           int              `json:"count"`
	Commits         []ActivityCommit `json:"commits"`
	ChangeSet       ChangeSet        `json:"change_set"`
	Correlations    []Correlation    `json:"correlations,omitempty"`
	// Cost is always populated, including on every early-return path: a query
	// that stopped early must still report what it spent.
	Cost CostReport `json:"cost"`
	// Disclosures record everything that bounded or degraded the result, plus
	// the two unconditional blind-spot statements that accompany any change
	// set.
	Disclosures []Disclosure `json:"disclosures,omitempty"`
}

// Boundaries are the two commits a change set was derived from.
type Boundaries struct {
	// BaseSHA is the parent of the earliest in-window commit. Empty when the
	// earliest in-window commit is the repository root.
	BaseSHA string `json:"base_sha"`
	// HeadSHA is the latest in-window commit.
	HeadSHA string `json:"head_sha"`
	// BaseSource records how the base was chosen: BaseSourceParentOfEarliest
	// or BaseSourceRepositoryRoot. Resolution is by ancestry, never by
	// timestamp proximity.
	BaseSource string `json:"base_source"`
	// Status is the provider's comparison status: ahead, identical, diverged,
	// or behind.
	Status string `json:"status"`
	// SharedRoot is false when the boundaries do not share ancestry, in which
	// case the change set is suppressed rather than rendered.
	SharedRoot bool `json:"shared_ancestry"`
}

// ActivityCommit is a commit in an activity window. It is deliberately distinct
// from Commit rather than a reuse: it carries the parent SHAs and committer
// date that boundary resolution needs, and Commit is pinned by SchemaVersion so
// it must not gain fields.
type ActivityCommit struct {
	SHA        string `json:"sha"`
	Repo       string `json:"repo"`
	Author     string `json:"author,omitempty"`
	AuthorName string `json:"author_name"`
	Email      string `json:"email,omitempty"`
	URL        string `json:"url"`
	// Message is the full commit message body, not just the summary.
	Message string `json:"message"`
	// AuthorDate is the git author date; CommitterDate is the git committer
	// date. They diverge after a rebase, cherry-pick, or amend, and GitHub
	// bounds the window by the latter.
	AuthorDate    time.Time `json:"author_date"`
	CommitterDate time.Time `json:"committer_date"`
	// ParentSHAs lists the commit's parents; the first element is the first
	// parent, which is what boundary resolution follows.
	ParentSHAs []string `json:"parent_shas,omitempty"`
	// Enriched is true only when per-commit detail was actually fetched. It is
	// the precondition for any observed correlation naming this commit.
	Enriched bool `json:"enriched,omitempty"`
	// Files is populated only when Enriched.
	Files []File `json:"files,omitempty"`
}

// Summary is the first line of the commit message.
func (c ActivityCommit) Summary() string {
	first, _, _ := strings.Cut(c.Message, "\n")
	return first
}

// ChangeSet is the aggregate per-path delta between the window's boundaries.
type ChangeSet struct {
	// Paths is sorted lexicographically so identical upstream state yields
	// byte-identical output; provider ordering is not guaranteed stable.
	Paths          []ChangedPath `json:"paths"`
	TotalAdditions int           `json:"total_additions"`
	TotalDeletions int           `json:"total_deletions"`
	// Truncated is true when the provider's file cap clipped the comparison.
	Truncated bool `json:"truncated,omitempty"`
}

// ChangedPath is one path's net change across the window.
type ChangedPath struct {
	Path           string `json:"path"`
	PreviousPath   string `json:"previous_path,omitempty"`
	Status         string `json:"status"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
	Patch          string `json:"patch,omitempty"`
	PatchTruncated bool   `json:"patch_truncated,omitempty"`
}

// Correlation links a changed path to the commits that plausibly produced it,
// labeled with how the link was established so a consumer can tell observation
// from inference and filter accordingly.
type Correlation struct {
	Path string `json:"path"`
	// SHAs is sorted; empty means the path is unattributed.
	SHAs []string `json:"shas,omitempty"`
	// Basis is BasisObserved or BasisInferred.
	Basis string `json:"basis"`
	// Rule names the rule that produced an inferred basis.
	Rule string `json:"rule,omitempty"`
}

// CostReport accounts for what a query consumed and what quota remains. It is
// always populated, including on failure paths.
type CostReport struct {
	// Estimated is the projected request count; 0 when no estimate was run.
	Estimated int `json:"estimated"`
	Consumed  int `json:"consumed"`
	// Ceiling is the configured request cap; 0 means disabled.
	Ceiling        int       `json:"ceiling"`
	QuotaRemaining int       `json:"quota_remaining"`
	QuotaLimit     int       `json:"quota_limit"`
	QuotaResetsAt  time.Time `json:"quota_resets_at,omitempty"`
}

// Disclosure states something that bounded or degraded a result, with the
// reason and, where one exists, the next action. A failure or a limit with no
// stated reason is a defect.
type Disclosure struct {
	Kind       string `json:"kind"`
	Reason     string `json:"reason"`
	NextAction string `json:"next_action,omitempty"`
}
