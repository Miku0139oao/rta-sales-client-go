// Package rtasales logs in to RTA with built-in or pluggable captcha solving
// and queries paginated sales data for inclusive calendar-date ranges.
//
// A Client binds one caller-supplied business store ID to one RTA account and
// owns that account's cookie jar. RTA applies the actual store scope through
// the authenticated account. A Client is safe for concurrent use, logs in on
// the first unauthenticated request, and retries once after an expired session.
package rtasales
