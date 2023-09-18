
package wpn

import (
	"bytes"
	"sync"
)

const ipPacketMaxSize = 65536
var ipBufPool = sync.Pool{
	New: func()(any){
		buf := make([]byte, ipPacketMaxSize)
		return &buf
	},
}

func getIPPktBuf()(buf []byte, free func()){
	p := ipBufPool.Get().(*[]byte)
	return *p, func(){
		ipBufPool.Put(p)
	}
}

var packetFormatBufPool = sync.Pool{
	New: func()(any){
		return bytes.NewBuffer(make([]byte, 0, ipPacketMaxSize + 1024))
	},
}
