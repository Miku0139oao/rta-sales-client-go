// Package rtasales logs in to RTA with built-in or pluggable captcha solving,
// discovers the stores available to the authenticated account, and queries
// paginated sales data for inclusive calendar-date ranges.
//
// A Client owns one account's cookie jar and authorized-store cache and is safe for
// concurrent use. It automatically logs in on the first unauthenticated
// request and retries once after an expired session.
package rtasales
