package storage

import "errors"

var (
	// ErrNotFound is returned by Get when the key doesn't exist.
	ErrNotFound = errors.New("storage: key not found")

	// ErrUnsupported is returned when an implementation doesn't support an
	// optional operation (e.g. PresignedGetURL on the filesystem store).
	ErrUnsupported = errors.New("storage: operation not supported by this backend")
)
