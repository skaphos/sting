// SPDX-License-Identifier: MIT
package ghclient

// Option configures a Client at construction time. New takes options
// variadically, so every existing three-argument call site stays
// source-compatible.
type Option func(*Client)

// WithRequestBudget caps the provider requests the client may consume and
// enables cost accounting.
//
// A ceiling of 0 disables the cap but still accounts: accounting is never
// optional, only enforcement is, so an intentional uncapped run can still
// report what it spent. Negative ceilings are treated as 0.
//
// Enforcement happens in the transport, which means it covers every request the
// client makes — pagination and retries included — without any call site
// needing to know about it.
func WithRequestBudget(ceiling int) Option {
	return func(c *Client) {
		if ceiling < 0 {
			ceiling = 0
		}
		c.budgetCeiling = ceiling
		c.budgetEnabled = true
	}
}
