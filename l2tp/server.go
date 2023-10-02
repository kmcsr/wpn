
// See <https://www.rfc-editor.org/rfc/rfc2661.html>
package l2tp

import (
	"context"
	"net"

	"github.com/kmcsr/go-logger"
	"github.com/kmcsr/wpn/internal/pool"
)

const L2TPVersion byte = 0x02
const DefaultL2TPPort = ":1701"

type Server struct {
	Addr string
	Logger logger.Logger
}

func (s *Server)recordErr(addr net.Addr, err error){
	if s.Logger != nil {
		s.Logger.Debugf("Error when handling %v: %v", addr, err)
	}
}

// serve a UDP connection
func (s *Server)Serve(conn net.PacketConn)(err error){
	defer conn.Close()
	var (
		buf []byte
		free func()
		n int
		addr net.Addr
	)
	for {
		buf, free = pool.GetIPPacketBuf()
		if n, addr, err = conn.ReadFrom(buf); err != nil {
			free()
			return
		}
		go func(buf []byte, free func(), serving net.PacketConn, addr net.Addr){
			defer free()
			s.handle(buf, serving, addr)
		}(buf[:n], free, conn, addr)
	}
}

func (s *Server)Shutdown(ctx context.Context)(err error){
	return
}

func (s *Server)ListenAndServe()(err error){
	addr := s.Addr
	if addr == "" {
		addr = DefaultL2TPPort
	}
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return
	}
	return s.Serve(conn)
}

func (s *Server)handle(buf []byte, serving net.PacketConn, addr net.Addr){
	var err error
	defer func(){
		if rer := recoverAsError(); rer != nil {
			s.recordErr(addr, rer)
		}else if err != nil {
			s.recordErr(addr, err)
		}
	}()
	var head *header
	if head, buf, err = parseHeader(buf); err != nil {
		return
	}
	if head.isControl {
		avps := make([]*avpPayload, 0, 3)
		for len(buf) > 0 {
			var avp *avpPayload
			if avp, buf, err = parseAVP(buf); err != nil {
				return
			}
			avps = append(avps, avp)
		}
		_ = avps[0]
		response := &header{
			isControl: true,
			sequence: true,
			tunnelID: head.tunnelID,
			sessionID: head.sessionID,
			ns: head.ns + 1,
			nr: head.nr + 1,
		}
		if buf, err = response.encode(buf[:0]); err != nil {
			return
		}
		if _, err = serving.WriteTo(buf, addr); err != nil {
			return
		}
	}else{
		println("recv non control msg from:", addr.String())
	}
}

// This header is formatted:
//
//  0                   1                   2                   3
//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |T|L|x|x|S|x|O|P|x|x|x|x|  Ver  |          Length (opt)         |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |           Tunnel ID           |           Session ID          |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |             Ns (opt)          |             Nr (opt)          |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |      Offset Size (opt)        |    Offset pad... (opt)
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
type header struct {
	isControl bool
	priority bool
	sequence bool
	tunnelID uint16
	sessionID uint16
	/* Ns indicates the sequence number for this data or control message */
	ns uint16
	/* Nr indicates the sequence number expected in the next control message to be received. */
	nr uint16
	length int
	offset int
}

const (
	flagType     = 1 << (15 - 0) // The Type (T) bit indicates the type of message. It is set to 0 for a data message and 1 for a control message.
	flagLength   = 1 << (15 - 1) // If the Length (L) bit is 1, the Length field is present. This bit MUST be set to 1 for control messages.
	flagSequence = 1 << (15 - 4) // If the Sequence (S) bit is set to 1 the Ns and Nr fields are present. The S bit MUST be set to 1 for control messages.
	flagOffset   = 1 << (15 - 6) // If the Offset (O) bit is 1, the Offset Size field is present. The O bit MUST be set to 0 (zero) for control messages.
	flagPriority = 1 << (15 - 7) // If the Priority (P) bit is 1, this data message should receive preferential treatment in its local queuing and transmission.
)

func parseHeader(buf []byte)(head *header, payload []byte, err error){
	if len(buf) < 6 {
		return nil, nil, WrongLengthErr
	}
	flags := ((uint16)(buf[0]) << 8) | (uint16)(buf[1])
	buf = buf[2:]
	version := (byte)(flags & 0xf)
	if version != L2TPVersion {
		err = &VersionError{version}
		return
	}
	var (
		isControl bool = flags & flagType != 0
		priority bool = flags & flagPriority != 0
		sequence bool = flags & flagSequence != 0
		leng int
		tunnelID uint16
		sessionID uint16
		ns, nr uint16
		offset uint16
	)
	if flags & flagLength != 0 {
		leng = ((int)(buf[0]) << 8) | (int)(buf[1])
		if len(buf) + 2 < leng {
			return nil, nil, WrongLengthErr
		}
		buf = buf[2:leng - 2]
	}
	tunnelID = ((uint16)(buf[0]) << 8) | (uint16)(buf[1])
	sessionID = ((uint16)(buf[2]) << 8) | (uint16)(buf[3])
	buf = buf[4:]
	if sequence {
		ns = ((uint16)(buf[0]) << 8) | (uint16)(buf[1])
		nr = ((uint16)(buf[2]) << 8) | (uint16)(buf[3])
		buf = buf[4:]
	}else if isControl {
		err = &UnexpectFlagValue{"Sequence", true}
		return
	}
	if flags & flagOffset != 0 {
		if isControl {
			err = &UnexpectFlagValue{"Offset", false}
			return
		}
		offset = ((uint16)(buf[0]) << 8) | (uint16)(buf[1])
		buf = buf[2 + offset:]
	}
	head = &header{
		isControl: isControl,
		priority: priority,
		sequence: sequence,
		tunnelID: tunnelID,
		sessionID: sessionID,
		ns: ns,
		nr: nr,
		length: leng,
		offset: (int)(offset),
	}
	payload = buf
	return
}

func (head *header)encode(buf []byte)(_ []byte, err error){
	start := len(buf)
	var flags uint16 = (uint16)(L2TPVersion)
	buf = append(buf, 0, 0) // reserve for flags
	if head.isControl {
		flags |= flagType
	}
	if head.priority {
		flags |= flagPriority
	}
	if head.length != 0 {
		flags |= flagLength
		buf = append(buf, (byte)(head.length >> 8), (byte)(head.length & 0xff))
	}
	buf = append(buf, (byte)(head.tunnelID >> 8), (byte)(head.tunnelID & 0xff))
	buf = append(buf, (byte)(head.sessionID >> 8), (byte)(head.sessionID & 0xff))
	if head.sequence {
		flags |= flagSequence
		buf = append(buf, (byte)(head.ns >> 8), (byte)(head.ns & 0xff))
		buf = append(buf, (byte)(head.nr >> 8), (byte)(head.nr & 0xff))
	}
	if head.offset > 0 {
		flags |= flagOffset
		buf = append(buf, (byte)(head.offset >> 8), (byte)(head.offset & 0xff))
		for i := head.offset; i < head.offset; i++ {
			buf = append(buf, 0xfa)
		}
	}
	buf[start] = (byte)(flags >> 8)
	buf[start + 1] = (byte)(flags & 0xff)
	return buf, nil
}


// Each AVP (Attribute-Value Pair) is encoded as:
//
//  0                   1                   2                   3
//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |M|H| rsvd  |      Length       |           Vendor ID           |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |         Attribute Type        |        Attribute Value...
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//                     [until Length is reached]...                |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
type avpPayload struct {
	mandatory bool
	hidden bool
	length int
	vendorID uint16
	attrType uint16
	value []byte
}

const (
	flagMandatory = 1 << (15 - 0) // Mandatory (M) bit: Controls the behavior required of an implementation which receives an AVP which it does not recognize.
	flagHidden    = 1 << (15 - 1) // Hidden (H) bit: Identifies the hiding of data in the Attribute Value field of an AVP.
)

func parseAVP(buf []byte)(msg *avpPayload, remain []byte, err error){
	if len(buf) < 6 {
		return nil, nil, WrongLengthErr
	}
	flags := ((uint16)(buf[0]) << 8) | (uint16)(buf[1])
	msg = &avpPayload{
		mandatory: flags & flagMandatory != 0,
		hidden: flags & flagHidden != 0,
	}
	msg.length = (int)(flags & 0x3ff)
	if msg.length < 6 || len(buf) < msg.length {
		return nil, nil, WrongLengthErr
	}
	msg.vendorID = ((uint16)(buf[2]) << 8) | (uint16)(buf[3])
	msg.attrType = ((uint16)(buf[4]) << 8) | (uint16)(buf[5])
	msg.value = buf[6:msg.length]
	remain = buf[msg.length:]
	return
}

func (msg *avpPayload)encode(buf []byte)(_ []byte, err error){
	var flags uint16
	if msg.mandatory {
		flags |= flagMandatory
	}
	if msg.hidden {
		flags |= flagHidden
	}
	flags |= (uint16)(6 + len(msg.value)) & 0x3ff
	buf = append(buf, (byte)(flags >> 8), (byte)(flags & 0xff))
	buf = append(buf, (byte)(msg.vendorID >> 8), (byte)(msg.vendorID & 0xff))
	buf = append(buf, (byte)(msg.attrType >> 8), (byte)(msg.attrType & 0xff))
	buf = append(buf, msg.value...)
	return buf, nil
}
