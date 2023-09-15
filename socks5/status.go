
package socks5

type StatusCode byte

const (
	StatusGranted             StatusCode = 0x00 // request granted
	StatusFailed              StatusCode = 0x01 // general failure
	StatusBlocked             StatusCode = 0x02 // connection not allowed by ruleset
	StatusNetworkUnreachable  StatusCode = 0x03 // network unreachable
	StatusHostUnreachable     StatusCode = 0x04 // host unreachable
	StatusRefused             StatusCode = 0x05 // connection refused by destination host
	StatusTLLExpired          StatusCode = 0x06 // TTL expired
	StatusCommandUnsupported  StatusCode = 0x07 // command not supported / protocol error
	StatusAddrTypeUnsupported StatusCode = 0x08 // address type not supported
)
