// Package rtasales logs in to RTA with built-in or pluggable captcha solving
// and queries paginated sales data for inclusive calendar-date ranges.
//
// A Client owns one RTA account's cookie jar and automatically resolves
// business-facing store IDs through that account's authorized-store list. A
// Client is safe for concurrent use, logs in on the first unauthenticated
// request, and retries once after an expired session. Transient HTTP 429, 408,
// 5xx, and transport timeouts are retried with Retry-After honored. In-flight
// HTTP for one account is serialized so concurrent store queries do not
// stampede. HTTP 401/403 and other permission denials are not retried.
package rtasales
