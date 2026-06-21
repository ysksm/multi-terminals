// Package apperr provides typed application-layer errors for HTTP status classification.
package apperr

// ValidationError wraps an error to signal that the failure is due to invalid
// client input (Value-Object construction or domain-invariant violations).
// The HTTP layer maps this to 400 Bad Request.
type ValidationError struct {
	Err error
}

// Error implements the error interface.
func (e *ValidationError) Error() string { return e.Err.Error() }

// Unwrap returns the underlying error, preserving the errors.Is/As chain.
func (e *ValidationError) Unwrap() error { return e.Err }

// Validation wraps err in a *ValidationError. If err is nil, it returns nil.
func Validation(err error) error {
	if err == nil {
		return nil
	}
	return &ValidationError{Err: err}
}
