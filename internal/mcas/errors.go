// Package mcas implements a client for the MyChildAtSchool (Bromcom MCAS)
// parent portal, ported from the ha-mychildatschool-mcas Python integration.
package mcas

// AuthError means the credentials were rejected, or a session could not be
// re-established after it lapsed.
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string { return e.Message }

// APIError is any other failure talking to MCAS (unexpected status code,
// unparseable response, etc).
type APIError struct {
	Message string
}

func (e *APIError) Error() string { return e.Message }
