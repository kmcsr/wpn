
package socks5

import (
	"errors"
	"fmt"
)

var (
	ErrNoAcceptableAuth = errors.New("No acceptable auth were offered")
	ErrUnsupportVersion = errors.New("Unsupport packet format verion")
	ErrUnauthorized = errors.New("Unauthorized")
	ErrVersionChanged = errors.New("Version changed after auth")
	ErrTargetBlocked = errors.New("Target address is blocked")
	ErrUnsupportCommand = errors.New("Unsupported command")
)

type UnsupportVersionError struct {
	Version byte
}

func (e *UnsupportVersionError)Error()(string){
	return fmt.Sprintf("Unsupport socks version 0x%x", e.Version)
}
