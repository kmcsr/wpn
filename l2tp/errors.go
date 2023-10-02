
package l2tp

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
)

var (
	IncorrectFormatErr = errors.New("Packet format incorrect")
	WrongLengthErr = errors.New("Wrong length of the packet")
)

type VersionError struct {
	Version byte
}

func (e *VersionError)Error()(string){
	return fmt.Sprintf("Unsupport version %d, support %d", e.Version, L2TPVersion)
}

type UnexpectFlagValue struct {
	Flag string
	Expect bool
}

func (e *UnexpectFlagValue)Error()(string){
	if e.Expect {
		return fmt.Sprintf("Flag %s is expected to set", e.Flag)
	}
	return fmt.Sprintf("Flag %s is unexpected to set", e.Flag)
}

type PanicError struct {
	err any
	stacktrace string
}

var stackBufPool = sync.Pool{
	New: func()(any){
		buf := make([]byte, 1024)
		return &buf
	},
}

func getStack()(string){
	buf := *(stackBufPool.Get().(*[]byte))
	for {
		n := runtime.Stack(buf, false)
		if n < len(buf) {
			s := (string)(buf[:n])
			stackBufPool.Put(&buf)
			return s
		}
		stackBufPool.Put(&buf)
		buf = make([]byte, len(buf) + 1024)
	}
}

func recoverAsError()(*PanicError){
	err := recover()
	if err == nil {
		return nil
	}
	return &PanicError{
		err: err,
		stacktrace: getStack(),
	}
}

func (e *PanicError)Error()(string){
	return fmt.Sprintf("panic: %v\n", e.err) + e.stacktrace
}
