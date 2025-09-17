package db

// Common session errors
var ErrSessionClosed = &SessionError{"session has been closed"}
var ErrSessionNotFound = &SessionError{"session not found"}

type SessionError struct{ msg string }

func (e *SessionError) Error() string { return e.msg }
