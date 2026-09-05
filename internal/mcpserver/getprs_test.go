// SPDX-License-Identifier: MIT
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func TestPRToolValidation(t *testing.T) {
	h := &handler{cfg: config.Default()}
	for _, in := range []GetPRsInput{{}, {Provider: "gitlab", Author: "octocat"}, {Author: "octocat", Role: "reviewer"}} {
		_, out, err := h.getPRs(context.Background(), nil, in)
		if err == nil || out.SchemaVersion != "" {
			t.Fatal("validation fabricated report")
		}
	}
}

func TestPRToolPartialAndPanic(t *testing.T) {
	original := collectPRs
	t.Cleanup(func() { collectPRs = original })
	h := &handler{cfg: config.Default()}
	collectPRs = func(_ context.Context, _ config.Config, q model.PRQuery) (model.PRResult, error) {
		return model.PRResult{SchemaVersion: model.PRSchemaVersion, Query: q, PRs: []model.PRActivityEntry{}, Truncated: true}, errors.New("fixture failure")
	}
	result, out, err := h.getPRs(context.Background(), nil, GetPRsInput{Author: "octocat"})
	if err != nil || !result.IsError || out.SchemaVersion == "" {
		t.Fatalf("partial lost: %v %+v", err, out)
	}
	collectPRs = func(context.Context, config.Config, model.PRQuery) (model.PRResult, error) { panic("secret-token") }
	result, _, err = h.getPRs(context.Background(), nil, GetPRsInput{Author: "octocat"})
	if err != nil || !result.IsError {
		t.Fatal("panic not recovered")
	}
	b, e := json.Marshal(result)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), "secret-token") {
		t.Fatal("panic leaked secret")
	}
}
