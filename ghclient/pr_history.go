// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v91/github"
	"github.com/skaphos/sting/model"
)

type prLifecycle struct {
	id   int64
	kind string
	at   time.Time
}

func (s *prSession) history(ctx context.Context, p model.PullRequest, q model.PRQuery) (model.PullRequest, []model.PRMatch, bool, error) {
	owner, repo, _ := splitRepo(p.Repo)
	events := []prLifecycle{}
	complete := true
	var stopped error
	opts := &github.ListOptions{PerPage: s.client.perPage, Page: 1}
	for pages := 0; ; pages++ {
		if pages >= maxPages {
			s.gap("pagination-limit", "Lifecycle pagination safety limit reached.", p.Repo, p.Number)
			complete = false
			break
		}
		var raw []*github.IssueEvent
		var resp *github.Response
		err := s.do("history", func() error {
			var e error
			raw, resp, e = s.client.gh.Issues.ListIssueEvents(ctx, owner, repo, p.Number, opts)
			return e
		})
		if err != nil {
			stopped = err
			complete = false
			s.gap("history-incomplete", "Lifecycle retrieval stopped: "+prErrorReason(err), p.Repo, p.Number)
			break
		}
		for _, e := range raw {
			kind := e.GetEvent()
			if kind != "closed" && kind != "merged" && kind != "reopened" {
				continue
			}
			if e.GetID() <= 0 || e.CreatedAt == nil || e.CreatedAt.IsZero() {
				s.gap("history-ambiguous", "Lifecycle event lacks a valid identity or time.", p.Repo, p.Number)
				complete = false
				continue
			}
			events = append(events, prLifecycle{id: e.GetID(), kind: kind, at: e.CreatedAt.UTC()})
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	p, matches, classified := s.classifyHistory(p, events, complete)
	return p, selectedPRMatches(matches, q), complete && classified, stopped
}
func (s *prSession) classifyHistory(p model.PullRequest, events []prLifecycle, complete bool) (model.PullRequest, []model.PRMatch, bool) {
	out := recordPRMatches(p)
	valid := true
	byID := map[int64]prLifecycle{}
	invalid := map[int64]bool{}
	for _, e := range events {
		if old, ok := byID[e.id]; ok && old != e {
			invalid[e.id] = true
			valid = false
			s.gap("history-ambiguous", "Conflicting payloads for one lifecycle event.", p.Repo, p.Number)
		}
		byID[e.id] = e
	}
	events = events[:0]
	for id, e := range byID {
		if !invalid[id] {
			events = append(events, e)
		}
	}
	slices.SortFunc(events, func(a, b prLifecycle) int {
		if n := a.at.Compare(b.at); n != 0 {
			return n
		}
		return strings.Compare(a.kind, b.kind)
	})
	var merge *prLifecycle
	mergeConflict := false
	for i := range events {
		e := &events[i]
		if e.kind != "merged" {
			continue
		}
		if merge != nil && !merge.at.Equal(e.at) {
			mergeConflict = true
		}
		if merge == nil || e.id < merge.id {
			merge = e
		}
	}
	if merge != nil {
		if p.MergedAt != nil && !p.MergedAt.Equal(merge.at) {
			mergeConflict = true
		}
		p.State = new(model.PRStateMerged)
		out = slices.DeleteFunc(out, func(m model.PRMatch) bool { return m.Kind == "merged" })
		if !mergeConflict {
			out = append(out, lifecycleMatch(*merge))
			p.MergedAt = &merge.at
		}
	}
	if mergeConflict {
		valid = false
		s.gap("history-ambiguous", "Record and lifecycle evidence disagree on the merge time.", p.Repo, p.Number)
	}
	if complete && valid && merge == nil && p.MergedAt == nil && p.State == nil && s.closedIssues[prIdentity(p)] {
		p.State = new(model.PRStateClosed)
	}
	historyConsistent := valid
	for _, e := range events {
		if e.kind != "closed" {
			continue
		}
		safe := complete && historyConsistent && merge == nil && p.MergedAt == nil
		ambiguous := false
		for _, other := range events {
			if other.kind == "reopened" && other.at.Equal(e.at) {
				ambiguous = true
			}
		}
		mergeAt := p.MergedAt
		if merge != nil {
			mergeAt = &merge.at
		}
		if mergeAt != nil {
			if e.at.Equal(*mergeAt) {
				continue
			}
			for _, later := range events {
				if later.kind == "reopened" && later.at.After(e.at) && later.at.Before(*mergeAt) {
					safe = true
				}
			}
		}
		if safe && !ambiguous && !mergeConflict {
			out = append(out, lifecycleMatch(e))
		} else {
			valid = false
			s.gap("history-ambiguous", "An unmerged closure cannot be established from the available lifecycle evidence.", p.Repo, p.Number)
		}
	}
	if complete && p.ClosedAt != nil && p.MergedAt == nil && !slices.ContainsFunc(events, func(e prLifecycle) bool { return e.kind == "closed" }) {
		valid = false
		s.gap("history-incomplete", "Current closure has no corresponding lifecycle occurrence.", p.Repo, p.Number)
	}
	if p.State != nil {
		p.MissingFields = slices.DeleteFunc(p.MissingFields, func(field string) bool { return field == "state" })
	}
	return p, out, valid
}
func lifecycleMatch(e prLifecycle) model.PRMatch {
	return model.PRMatch{ID: "event:" + strconv.FormatInt(e.id, 10), Kind: e.kind, At: e.at, Source: "issue-event", EventID: &e.id}
}
func recordPRMatches(p model.PullRequest) []model.PRMatch {
	out := []model.PRMatch{}
	for _, item := range []struct {
		id, kind string
		at       *time.Time
	}{{"created", "opened", p.CreatedAt}, {"merged", "merged", p.MergedAt}, {"updated", "updated", p.UpdatedAt}} {
		if item.at == nil {
			continue
		}
		id := item.id
		if item.kind == "updated" {
			id += ":" + item.at.Format(time.RFC3339Nano)
		}
		out = append(out, model.PRMatch{ID: id, Kind: item.kind, At: *item.at, Source: "record"})
	}
	return out
}
func selectedPRMatches(matches []model.PRMatch, q model.PRQuery) []model.PRMatch {
	out := []model.PRMatch{}
	seen := map[string]bool{}
	for _, m := range matches {
		if m.At.Before(q.Since) || m.At.After(q.Until) {
			continue
		}
		wanted := q.TimeBasis == model.PRTimeAny && m.Kind != "updated" || q.TimeBasis == model.PRTimeCreated && m.Kind == "opened" || string(q.TimeBasis) == m.Kind
		if wanted && !seen[m.ID] {
			out = append(out, m)
			seen[m.ID] = true
		}
	}
	order := map[string]int{"opened": 0, "closed": 1, "merged": 2, "updated": 3}
	slices.SortFunc(out, func(a, b model.PRMatch) int {
		if n := a.At.Compare(b.At); n != 0 {
			return n
		}
		if order[a.Kind] != order[b.Kind] {
			return order[a.Kind] - order[b.Kind]
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// prBeforeWindow reports records that provably hold no in-window occurrence.
// GitHub bumps updated_at on every open, merge, close, and reopen, so a record
// last updated before the window cannot carry a qualifying action inside it.
// Excluding it costs no evidence: the record itself is the proof, so no
// lifecycle request is spent and no coverage gap is claimed. This does not
// exclude candidates updated after the window, whose history FR-023 requires.
func prBeforeWindow(p model.PullRequest, q model.PRQuery) bool {
	return p.UpdatedAt != nil && p.UpdatedAt.Before(q.Since)
}
func prHistoryNeeded(p model.PullRequest, q model.PRQuery) bool {
	return q.TimeBasis == model.PRTimeAny || q.TimeBasis == model.PRTimeClosed || (q.TimeBasis == model.PRTimeMerged && p.MergedAt == nil) || (q.State != model.PRStateAll && p.State == nil)
}
func prFilter(p model.PullRequest, q model.PRQuery) (matches, known bool) {
	if p.CreatedAt != nil && p.CreatedAt.After(q.Until) {
		return false, true
	}
	if q.Author != "" && p.Author != "" && !strings.EqualFold(q.Author, p.Author) {
		return false, true
	}
	if q.State != model.PRStateAll && p.State != nil && *p.State != q.State {
		return false, true
	}
	if q.Draft != model.PRDraftAll && p.Draft != nil && (*p.Draft != (q.Draft == model.PRDraftOnly)) {
		return false, true
	}
	known = (q.Author == "" || p.Author != "") && (q.State == model.PRStateAll || p.State != nil) && (q.Draft == model.PRDraftAll || p.Draft != nil)
	return known, known
}
func prMissingMatch(s *prSession, p model.PullRequest) {
	s.gap("metadata-unavailable", fmt.Sprintf("PR %d lacks evidence required to evaluate selection.", p.Number), p.Repo, p.Number)
}
