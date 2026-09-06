// SPDX-License-Identifier: MIT
package mcpinstall

import "fmt"

// credentialEnvNames returns names only. Installation must never capture the
// installer's credential values or require a PAT from an OAuth-only user.
func credentialEnvNames() []string {
	return []string{"STING_TOKEN", "STING_GITLAB_TOKEN"}
}

func credentialReferences(format string) map[string]string {
	refs := make(map[string]string)
	for _, name := range credentialEnvNames() {
		refs[name] = fmt.Sprintf(format, name)
	}
	return refs
}

// credentialEnvConfigured lets reinstall upgrade an old command-only entry
// without rewriting already configured entries or inspecting credential values.
func credentialEnvConfigured(env map[string]string, forwarded []any) bool {
	present := make(map[string]bool)
	for name := range env {
		present[name] = true
	}
	for _, value := range forwarded {
		switch value := value.(type) {
		case string:
			present[value] = true
		case map[string]any:
			name, _ := value["name"].(string)
			present[name] = true
		}
	}
	for _, name := range credentialEnvNames() {
		if !present[name] {
			return false
		}
	}
	return true
}

// addCredentialReferences fills missing keys only, preserving explicit tokens,
// custom references, empty overrides, and unrelated environment settings.
func addCredentialReferences(entry map[string]any, field, format string) error {
	env, err := jsonObjectAt(entry, field, "sting")
	if err != nil {
		return err
	}
	if env == nil {
		env = make(map[string]any)
	}
	for name, reference := range credentialReferences(format) {
		if _, exists := env[name]; !exists {
			env[name] = reference
		}
	}
	entry[field] = env
	return nil
}

func addGrokCredentialReferences(entry map[string]any) error {
	return addCredentialReferences(entry, "env", "${%s:-}")
}

// addCodexCredentialEnv preserves forwarding order and source objects. Explicit
// env entries also count as configured, so a default cannot shadow a user value.
func addCodexCredentialEnv(entry map[string]any) error {
	explicit, err := jsonObjectAt(entry, "env", "sting")
	if err != nil {
		return err
	}
	var forwarded []any
	if raw, exists := entry["env_vars"]; exists {
		var ok bool
		forwarded, ok = raw.([]any)
		if !ok {
			return fmt.Errorf("sting.env_vars must be an array")
		}
	}
	present := make(map[string]bool)
	for name := range explicit {
		present[name] = true
	}
	for _, value := range forwarded {
		var name string
		switch value := value.(type) {
		case string:
			name = value
		case map[string]any:
			name, _ = value["name"].(string)
		}
		if name == "" {
			return fmt.Errorf("sting.env_vars entries must be names or objects with a nonempty name")
		}
		present[name] = true
	}
	for _, name := range credentialEnvNames() {
		if !present[name] {
			forwarded = append(forwarded, name)
		}
	}
	if len(forwarded) > 0 {
		entry["env_vars"] = forwarded
	}
	return nil
}
