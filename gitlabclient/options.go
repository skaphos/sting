// SPDX-License-Identifier: MIT
package gitlabclient

// Option configures a Client at construction time. New takes options
// variadically so existing three-argument callers remain source-compatible.
type Option func(*Client)

// WithRequestBudget caps provider requests and enables cost accounting.
// A ceiling of 0 is intentionally uncapped but still accounted.
func WithRequestBudget(ceiling int) Option {
	return func(c *Client) {
		if ceiling < 0 {
			ceiling = 0
		}
		c.budgetCeiling = ceiling
		c.budgetEnabled = true
	}
}
