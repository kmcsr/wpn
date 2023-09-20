
package pool

import (
	"sync"
)

const IPPacketMaxSize = 65536
var ipBufPool = sync.Pool{
	New: func()(any){
		buf := make([]byte, IPPacketMaxSize)
		return &buf
	},
}

func GetIPPacketBuf()(buf []byte, free func()){
	p := ipBufPool.Get().(*[]byte)
	return *p, func(){
		ipBufPool.Put(p)
	}
}
