
package wpn

import (
	"bytes"
	"sync"

	"github.com/kmcsr/wpn/internal/pool"
)

var packetFormatBufPool = sync.Pool{
	New: func()(any){
		return bytes.NewBuffer(make([]byte, 0, pool.IPPacketMaxSize + 1024))
	},
}
