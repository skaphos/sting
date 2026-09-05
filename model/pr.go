// SPDX-License-Identifier: MIT
package model

import "time"

// PRSchemaVersion and PRInboxSchemaVersion independently version the PR workflows.
const (
	PRSchemaVersion      = "sting.prs.skaphos.io/v1"
	PRInboxSchemaVersion = "sting.pr-inbox.skaphos.io/v1"
)

// PRState is current provider state; closed excludes merged work.
type PRState string

const (
	PRStateOpen   PRState = "open"
	PRStateMerged PRState = "merged"
	PRStateClosed PRState = "closed"
	PRStateAll    PRState = "all"
)

// PRTimeBasis identifies the activity timestamps selected by a query.
type PRTimeBasis string

const (
	PRTimeCreated PRTimeBasis = "created"
	PRTimeUpdated PRTimeBasis = "updated"
	PRTimeMerged  PRTimeBasis = "merged"
	PRTimeClosed  PRTimeBasis = "closed"
	PRTimeAny     PRTimeBasis = "any"
)

// PRDraftFilter distinguishes omission from explicit draft selection.
type PRDraftFilter string

const (
	PRDraftAll     PRDraftFilter = "all"
	PRDraftOnly    PRDraftFilter = "only"
	PRDraftExclude PRDraftFilter = "exclude"
)

// PRRelationship is a current personal inclusion reason, not a past review action.
type PRRelationship string

const (
	PRAuthored        PRRelationship = "authored"
	PRAssigned        PRRelationship = "assigned"
	PRReviewRequested PRRelationship = "review-requested"
)

// PRQuery is a resolved, inclusive UTC activity selection.
type PRQuery struct {
	Provider    Provider      `json:"provider"`
	Scope       Scope         `json:"scope"`
	Repos       []string      `json:"repos"`
	Org         string        `json:"org"`
	Author      string        `json:"author"`
	Role        string        `json:"role"`
	State       PRState       `json:"state"`
	Draft       PRDraftFilter `json:"draft"`
	TimeBasis   PRTimeBasis   `json:"time_basis"`
	Since       time.Time     `json:"since"`
	Until       time.Time     `json:"until"`
	MaxPRs      int           `json:"max_prs"`
	MaxRequests int           `json:"max_requests"`
}

// PRInboxQuery has no activity window. Empty User is resolved by the collector.
type PRInboxQuery struct {
	Provider       Provider         `json:"provider"`
	Scope          Scope            `json:"scope"`
	Repos          []string         `json:"repos"`
	Org            string           `json:"org"`
	User           string           `json:"user"`
	IdentitySource string           `json:"identity_source"`
	Relationships  []PRRelationship `json:"relationships"`
	State          PRState          `json:"state"`
	Draft          PRDraftFilter    `json:"draft"`
	MaxPRs         int              `json:"max_prs"`
	MaxRequests    int              `json:"max_requests"`
}

// PullRequest contains available provider facts. MissingFields distinguishes unknown
// nulls from known absence; nil slices are unknown and empty slices are known empty.
type PullRequest struct {
	Repo               string     `json:"repo"`
	Number             int        `json:"number"`
	URL                string     `json:"url"`
	Title              string     `json:"title"`
	Author             string     `json:"author"`
	State              *PRState   `json:"state"`
	Draft              *bool      `json:"draft"`
	CreatedAt          *time.Time `json:"created_at"`
	UpdatedAt          *time.Time `json:"updated_at"`
	MergedAt           *time.Time `json:"merged_at"`
	ClosedAt           *time.Time `json:"closed_at"`
	Assignees          []string   `json:"assignees"`
	RequestedReviewers []string   `json:"requested_reviewers"`
	RequestedTeams     []string   `json:"requested_teams"`
	BaseRef            *string    `json:"base_ref"`
	HeadRef            *string    `json:"head_ref"`
	Labels             []string   `json:"labels"`
	Milestone          *string    `json:"milestone"`
	Additions          *int       `json:"additions"`
	Deletions          *int       `json:"deletions"`
	ChangedFiles       *int       `json:"changed_files"`
	MissingFields      []string   `json:"missing_fields"`
}

// PRMatch is one distinct confirmed occurrence, independent of current state.
type PRMatch struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"`
	At      time.Time `json:"at"`
	Source  string    `json:"source"`
	EventID *int64    `json:"event_id"`
}

// PRActivityEntry carries only confirmed selected actions.
type PRActivityEntry struct {
	PR              PullRequest `json:"pr"`
	Matches         []PRMatch   `json:"matches"`
	MatchesComplete bool        `json:"matches_complete"`
}

// PRInboxEntry carries all confirmed current personal reasons in fixed order.
type PRInboxEntry struct {
	PR              PullRequest      `json:"pr"`
	Reasons         []PRRelationship `json:"reasons"`
	ReasonsComplete bool             `json:"reasons_complete"`
}

// PRCoverage separates candidate discovery from matching-evidence evaluation.
type PRCoverage struct {
	DiscoveryComplete bool `json:"discovery_complete"`
	EvidenceComplete  bool `json:"evidence_complete"`
}

// PRDisclosure is a sanitized and attributable limitation, error, or selection fact.
type PRDisclosure struct {
	Kind       string `json:"kind"`
	Reason     string `json:"reason"`
	NextAction string `json:"next_action,omitempty"`
	Repo       string `json:"repo,omitempty"`
	Number     int    `json:"number,omitempty"`
	Stream     string `json:"stream,omitempty"`
}

// PRCostReport counts actual dispatched requests; categories sum to Consumed.
type PRCostReport struct {
	Consumed          int      `json:"consumed"`
	Ceiling           int      `json:"ceiling"`
	IdentityRequests  int      `json:"identity_requests"`
	DiscoveryRequests int      `json:"discovery_requests"`
	HistoryRequests   int      `json:"history_requests"`
	Quota             *PRQuota `json:"quota,omitempty"`
}

// PRResult is the versioned activity evidence envelope, including partial failures.
type PRResult struct {
	SchemaVersion string            `json:"schema_version"`
	Workflow      string            `json:"workflow"`
	Provider      Provider          `json:"provider"`
	GeneratedAt   time.Time         `json:"generated_at"`
	Query         PRQuery           `json:"query"`
	PRs           []PRActivityEntry `json:"prs"`
	Count         int               `json:"count"`
	Coverage      PRCoverage        `json:"coverage"`
	Truncated     bool              `json:"truncated"`
	Cost          PRCostReport      `json:"cost"`
	Disclosures   []PRDisclosure    `json:"disclosures"`
}

// PRInboxResult is the independently versioned current-work evidence envelope.
type PRInboxResult struct {
	SchemaVersion string         `json:"schema_version"`
	Workflow      string         `json:"workflow"`
	Provider      Provider       `json:"provider"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Query         PRInboxQuery   `json:"query"`
	PRs           []PRInboxEntry `json:"prs"`
	Count         int            `json:"count"`
	Coverage      PRCoverage     `json:"coverage"`
	Truncated     bool           `json:"truncated"`
	Cost          PRCostReport   `json:"cost"`
	Disclosures   []PRDisclosure `json:"disclosures"`
}

// PRQuota is the last observed provider quota, not a combined search/core quota.
type PRQuota struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	Reset     time.Time `json:"reset"`
}
