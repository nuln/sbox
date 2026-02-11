package sbox

import (
	"errors"
	"os"
)

// Common storage errors. Where possible, these alias os package errors
// for compatibility with os.IsNotExist, os.IsPermission, etc.
var (
	ErrNotFound     = os.ErrNotExist
	ErrExist        = os.ErrExist
	ErrPermission   = os.ErrPermission
	ErrInvalid      = os.ErrInvalid
	ErrIsDir        = errors.New("sbox: is a directory")
	ErrNotDir       = errors.New("sbox: not a directory")
	ErrClosed       = errors.New("sbox: already closed")
	ErrNotSupported = errors.New("sbox: feature not supported by this backend")
)
