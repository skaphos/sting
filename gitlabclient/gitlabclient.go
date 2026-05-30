// SPDX-License-Identifier: MIT
// Package gitlabclient retrieves commits from GitLab and normalizes them into
// the shared model types used by sting.
package gitlabclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/skaphos/sting/model"
)

const defaultBaseURL = "https://gitlab.com/api/v4/"

// Client retrieves commits from GitLab for an author over a time window.
type Client struct {
	http    *http.Client
	baseURL string
	token   string
	perPage int
}

// New builds a Client. token may be empty for public data. baseURL, when set,
// targets a GitLab API v4 root (e.g. "https://gitlab.example.com/api/v4/").
// perPage is clamped to the API's 1-100 range.
func New(token, baseURL string, perPage int) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("configure gitlab URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("configure gitlab URL: missing scheme or host")
	}
	base := u.String()
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	if perPage < 1 {
		perPage = 100
	}
	if perPage > 100 {
		perPage = 100
	}
	return &Client{
		http:    http.DefaultClient,
		baseURL: base,
		token:   token,
		perPage: perPage,
	}, nil
}

// Collect runs a query using its scope and returns the normalized result.
func (c *Client) Collect(ctx context.Context, q model.Query) (model.Result, error) {
	until := q.Until
	if until.IsZero() {
		until = time.Now()
	}
	q.Until = until

	var (
		commits []model.Commit
		err     error
	)
	switch q.Scope {
	case model.ScopeRepos:
		commits, err = c.listRepos(ctx, q)
	case model.ScopeOrg:
		commits, err = c.listGroup(ctx, q)
	case model.ScopeSearch:
		return model.Result{}, fmt.Errorf("provider %q does not support scope %q (use repos or org)", model.ProviderGitLab, q.Scope)
	default:
		return model.Result{}, fmt.Errorf("unsupported scope %q", q.Scope)
	}
	if err != nil {
		return model.Result{}, err
	}

	truncated := false
	if q.MaxCommits > 0 && len(commits) >= q.MaxCommits {
		commits = commits[:q.MaxCommits]
		truncated = true
	}

	return model.Result{
		SchemaVersion: model.SchemaVersion,
		GeneratedAt:   time.Now(),
		Provider:      model.ProviderGitLab,
		Author:        q.Author,
		Scope:         q.Scope,
		Since:         q.Since,
		Until:         q.Until,
		Count:         len(commits),
		Commits:       commits,
		Truncated:     truncated,
	}, nil
}

func (c *Client) listRepos(ctx context.Context, q model.Query) ([]model.Commit, error) {
	if len(q.Repos) == 0 {
		return nil, fmt.Errorf("scope %q requires at least one repo", model.ScopeRepos)
	}
	var out []model.Commit
	for _, target := range q.Repos {
		project := strings.TrimSpace(target)
		if project == "" {
			return nil, fmt.Errorf("invalid repo %q", target)
		}
		commits, err := c.listProjectCommits(ctx, project, project, q)
		if err != nil {
			return nil, err
		}
		out = append(out, commits...)
		if q.MaxCommits > 0 && len(out) >= q.MaxCommits {
			break
		}
	}
	return out, nil
}

func (c *Client) listGroup(ctx context.Context, q model.Query) ([]model.Commit, error) {
	if strings.TrimSpace(q.Org) == "" {
		return nil, fmt.Errorf("scope %q requires an org", model.ScopeOrg)
	}
	projects, err := c.groupProjects(ctx, q.Org)
	if err != nil {
		return nil, err
	}
	var out []model.Commit
	for _, project := range projects {
		target := strconv.FormatInt(project.ID, 10)
		label := project.PathWithNamespace
		if label == "" {
			label = target
		}
		commits, err := c.listProjectCommits(ctx, target, label, q)
		if err != nil {
			return nil, err
		}
		out = append(out, commits...)
		if q.MaxCommits > 0 && len(out) >= q.MaxCommits {
			break
		}
	}
	return out, nil
}

func (c *Client) listProjectCommits(ctx context.Context, project, repoLabel string, q model.Query) ([]model.Commit, error) {
	values := url.Values{}
	values.Set("author", q.Author)
	values.Set("since", q.Since.UTC().Format(time.RFC3339))
	values.Set("until", q.Until.UTC().Format(time.RFC3339))
	values.Set("per_page", strconv.Itoa(c.perPage))
	if q.IncludeStats {
		values.Set("with_stats", "true")
	}

	endpoint := "projects/" + url.PathEscape(project) + "/repository/commits"
	var out []model.Commit
	for page := 1; ; page++ {
		values.Set("page", strconv.Itoa(page))
		var commits []gitlabCommit
		next, err := c.get(ctx, "list commits "+repoLabel, endpoint, values, &commits)
		if err != nil {
			return nil, err
		}
		for _, gc := range commits {
			cm := fromCommit(repoLabel, gc)
			if q.IncludeFiles || q.IncludeDiffs {
				if err := c.fillDiffDetails(ctx, project, repoLabel, &cm, q); err != nil {
					return nil, err
				}
			}
			out = append(out, cm)
			if q.MaxCommits > 0 && len(out) >= q.MaxCommits {
				return out, nil
			}
		}
		if next == "" {
			break
		}
	}
	return out, nil
}

func (c *Client) groupProjects(ctx context.Context, group string) ([]gitlabProject, error) {
	values := url.Values{}
	values.Set("include_subgroups", "true")
	values.Set("simple", "true")
	values.Set("per_page", strconv.Itoa(c.perPage))

	endpoint := "groups/" + url.PathEscape(strings.TrimSpace(group)) + "/projects"
	var out []gitlabProject
	for page := 1; ; page++ {
		values.Set("page", strconv.Itoa(page))
		var projects []gitlabProject
		next, err := c.get(ctx, "list group projects "+group, endpoint, values, &projects)
		if err != nil {
			return nil, err
		}
		out = append(out, projects...)
		if next == "" {
			break
		}
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, op, endpoint string, values url.Values, dest any) (string, error) {
	u := c.baseURL + endpoint
	if len(values) > 0 {
		u += "?" + values.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("%s: build request: %w", op, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "sting")
	if c.token != "" {
		req.Header.Set("PRIVATE-TOKEN", c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return "", fmt.Errorf("%s: gitlab api status %d: %s", op, resp.StatusCode, msg)
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return "", fmt.Errorf("%s: decode response: %w", op, err)
	}
	return resp.Header.Get("X-Next-Page"), nil
}

type gitlabProject struct {
	ID                int64  `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
}

type gitlabCommit struct {
	ID           string       `json:"id"`
	ShortID      string       `json:"short_id"`
	Title        string       `json:"title"`
	Message      string       `json:"message"`
	AuthorName   string       `json:"author_name"`
	AuthorEmail  string       `json:"author_email"`
	AuthoredDate string       `json:"authored_date"`
	WebURL       string       `json:"web_url"`
	Stats        *commitStats `json:"stats"`
}

type commitStats struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	Total     int `json:"total"`
}

type gitlabDiff struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	Diff        string `json:"diff"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
	TooLarge    bool   `json:"too_large"`
	Collapsed   bool   `json:"collapsed"`
}

func fromCommit(repo string, gc gitlabCommit) model.Commit {
	message := gc.Message
	if message == "" {
		message = gc.Title
	}
	cm := model.Commit{
		SHA:        gc.ID,
		Repo:       repo,
		AuthorName: gc.AuthorName,
		Email:      gc.AuthorEmail,
		Message:    message,
		URL:        gc.WebURL,
	}
	if t, err := time.Parse(time.RFC3339Nano, gc.AuthoredDate); err == nil {
		cm.Date = t
	}
	if gc.Stats != nil {
		cm.Additions = gc.Stats.Additions
		cm.Deletions = gc.Stats.Deletions
		cm.Changes = gc.Stats.Total
		if cm.Changes == 0 {
			cm.Changes = cm.Additions + cm.Deletions
		}
	}
	return cm
}

func (c *Client) fillDiffDetails(ctx context.Context, project, repoLabel string, cm *model.Commit, q model.Query) error {
	endpoint := "projects/" + url.PathEscape(project) + "/repository/commits/" + url.PathEscape(cm.SHA) + "/diff"
	values := url.Values{}
	values.Set("per_page", strconv.Itoa(c.perPage))

	var out []model.File
	budget := q.MaxDiffBytes
	if budget <= 0 {
		budget = model.DefaultMaxDiffBytes
	}
	for page := 1; ; page++ {
		values.Set("page", strconv.Itoa(page))
		var diffs []gitlabDiff
		next, err := c.get(ctx, "get commit diff "+repoLabel+"@"+cm.SHA, endpoint, values, &diffs)
		if err != nil {
			return err
		}
		for _, gd := range diffs {
			additions, deletions := countPatchLines(gd.Diff)
			mf := model.File{
				Path:         gd.NewPath,
				PreviousPath: previousPath(gd),
				Status:       diffStatus(gd),
				Additions:    additions,
				Deletions:    deletions,
				Changes:      additions + deletions,
			}
			if mf.Path == "" {
				mf.Path = gd.OldPath
			}
			if q.IncludeDiffs {
				mf.Patch, mf.PatchTruncated, budget = consumePatchBudget(gd.Diff, budget)
				mf.PatchTruncated = mf.PatchTruncated || gd.TooLarge || gd.Collapsed
			} else if gd.TooLarge || gd.Collapsed {
				mf.PatchTruncated = true
			}
			out = append(out, mf)
		}
		if next == "" {
			break
		}
	}
	cm.Files = out
	if cm.Additions == 0 && cm.Deletions == 0 {
		for _, f := range out {
			cm.Additions += f.Additions
			cm.Deletions += f.Deletions
		}
		cm.Changes = cm.Additions + cm.Deletions
	}
	return nil
}

func previousPath(gd gitlabDiff) string {
	if gd.RenamedFile && gd.OldPath != gd.NewPath {
		return gd.OldPath
	}
	return ""
}

func diffStatus(gd gitlabDiff) string {
	switch {
	case gd.NewFile:
		return "added"
	case gd.DeletedFile:
		return "removed"
	case gd.RenamedFile:
		return "renamed"
	default:
		return "modified"
	}
}

func countPatchLines(patch string) (additions, deletions int) {
	for line := range strings.SplitSeq(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			additions++
		case strings.HasPrefix(line, "-"):
			deletions++
		}
	}
	return additions, deletions
}

func consumePatchBudget(patch string, budget int) (string, bool, int) {
	if patch == "" {
		return "", false, budget
	}
	if budget <= 0 {
		return "", true, 0
	}
	if len(patch) <= budget {
		return patch, false, budget - len(patch)
	}
	return patch[:budget], true, 0
}
