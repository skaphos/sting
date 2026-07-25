// SPDX-License-Identifier: MIT

package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serverJSON is the checked-in MCP registry entry. These tests guard against it
// drifting from what sting actually serves: the registry is a published
// description of this server, and a description that no longer matches is worse
// than none, because clients act on it.
type serverJSON struct {
	Schema      string `json:"$schema"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Repository  struct {
		URL    string `json:"url"`
		Source string `json:"source"`
	} `json:"repository"`
	Packages []struct {
		RegistryType string `json:"registryType"`
		Identifier   string `json:"identifier"`
		Version      string `json:"version"`
		Transport    struct {
			Type string `json:"type"`
		} `json:"transport"`
		EnvironmentVariables []struct {
			Name       string `json:"name"`
			IsRequired bool   `json:"isRequired"`
			IsSecret   bool   `json:"isSecret"`
		} `json:"environmentVariables"`
	} `json:"packages"`
}

func loadServerJSON(t *testing.T) serverJSON {
	t.Helper()
	path := filepath.Join("..", "..", "server.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var s serverJSON
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return s
}

// TestServerJSONIdentity pins the registry identity. The namespace is not
// cosmetic: it is derived from the domain whose DNS TXT record proves ownership,
// so changing it silently would make publishing fail or, worse, publish under a
// name nobody controls.
func TestServerJSONIdentity(t *testing.T) {
	s := loadServerJSON(t)

	if s.Name != "io.skaphos/sting" {
		t.Errorf("name = %q, want io.skaphos/sting (the reverse-DNS form of skaphos.io)", s.Name)
	}
	if !strings.HasPrefix(s.Name, "io.skaphos/") {
		t.Error("name is outside the io.skaphos namespace, which is the only one the DNS proof covers")
	}
	if s.Repository.URL != "https://github.com/skaphos/sting" {
		t.Errorf("repository.url = %q", s.Repository.URL)
	}
}

// TestServerJSONDescribesReadOnly: every tool sting exposes is read-only, and
// the published description is where a prospective user learns that.
func TestServerJSONDescribesReadOnly(t *testing.T) {
	s := loadServerJSON(t)

	if !strings.Contains(strings.ToLower(s.Description), "read-only") {
		t.Errorf("description does not state the read-only scope: %q", s.Description)
	}
	if len(s.Description) > 100 {
		t.Errorf("description is %d characters; the registry schema caps it at 100", len(s.Description))
	}
}

// TestServerJSONTransportMatchesServer: sting serves MCP over stdio, and the
// container entrypoint starts exactly that. A mismatch here means a client
// configured from the registry cannot connect.
func TestServerJSONTransportMatchesServer(t *testing.T) {
	s := loadServerJSON(t)

	if len(s.Packages) != 1 {
		t.Fatalf("expected exactly one package entry, got %d", len(s.Packages))
	}
	pkg := s.Packages[0]

	if pkg.Transport.Type != "stdio" {
		t.Errorf("transport = %q, want stdio", pkg.Transport.Type)
	}
	if pkg.RegistryType != "oci" {
		t.Errorf("registryType = %q, want oci", pkg.RegistryType)
	}
	if pkg.Identifier != "ghcr.io/skaphos/sting" {
		t.Errorf("identifier = %q, want the published image", pkg.Identifier)
	}
}

// TestServerJSONAdvertisesOwnCredentials guards a constitutional boundary:
// sting authenticates with its own STING_-prefixed tokens and never reads
// ambient provider tokens. Advertising GITHUB_TOKEN here would misrepresent the
// credential model to every client that reads the registry.
func TestServerJSONAdvertisesOwnCredentials(t *testing.T) {
	s := loadServerJSON(t)
	pkg := s.Packages[0]

	got := make(map[string]bool)
	for _, env := range pkg.EnvironmentVariables {
		got[env.Name] = true

		if !strings.HasPrefix(env.Name, "STING_") {
			t.Errorf("environment variable %q is not one of sting's own keys", env.Name)
		}
		if !env.IsSecret {
			t.Errorf("%s carries a credential but is not marked secret", env.Name)
		}
	}

	for _, want := range []string{"STING_TOKEN", "STING_GITLAB_TOKEN"} {
		if !got[want] {
			t.Errorf("registry entry does not advertise %s", want)
		}
	}
	for _, forbidden := range []string{"GITHUB_TOKEN", "GH_TOKEN", "GITLAB_TOKEN"} {
		if got[forbidden] {
			t.Errorf("registry entry advertises the ambient token %s, which sting does not read", forbidden)
		}
	}
}

// TestServerJSONVersionsAreConsistent: the release workflow stamps both version
// fields from the tag, so they must be kept in step with each other. Divergence
// checked in would publish an entry naming an image tag that does not exist.
func TestServerJSONVersionsAreConsistent(t *testing.T) {
	s := loadServerJSON(t)

	if s.Version != s.Packages[0].Version {
		t.Errorf("server version %q and package version %q disagree; the release stamps both",
			s.Version, s.Packages[0].Version)
	}
}

// TestServerJSONCoversEveryAdvertisedTool is the drift guard proper: the
// registry entry must not describe a surface sting does not have, nor omit one
// it does. Every tool the server registers is read-only, which is what the
// description claims.
func TestServerJSONCoversEveryAdvertisedTool(t *testing.T) {
	s := loadServerJSON(t)

	tools := ReadOnlyTools()
	if len(tools) == 0 {
		t.Fatal("the server advertises no read-only tools; the registry description is now wrong")
	}

	// Every registered tool must be read-only for the published description
	// to be accurate.
	if len(tools) != len(toolDefinitions()) {
		t.Errorf("the server registers %d tools but only %d are read-only; the registry entry "+
			"claims read-only scope and would be misleading", len(toolDefinitions()), len(tools))
	}

	if !strings.Contains(strings.ToLower(s.Description), "commit") {
		t.Errorf("description does not mention commits, which every advertised tool returns: %q",
			s.Description)
	}
}
