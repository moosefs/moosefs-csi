package driver

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"syscall"

	log "github.com/sirupsen/logrus"
)

// values from MFSCommunication.h

const (
	PROTO_BASE               = 0
	ANTOAN_NOP               = 0
	CLTOMA_FUSE_REGISTER     = PROTO_BASE + 400
	MATOCL_FUSE_REGISTER     = PROTO_BASE + 401
	CLTOMA_FUSE_QUOTACONTROL = PROTO_BASE + 476
	MATOCL_FUSE_QUOTACONTROL = PROTO_BASE + 477

	QUOTA_FLAG_HLENGTH = 0x20
	QUOTA_FLAG_HSIZE   = 0x40

	FUSE_REGISTER_BLOB_ACL = "DjI1GAQDULI5d2YjA26ypc3ovkhjvhciTQVx3CS4nYgtBoUcsljiVpsErJENHaw0"
	REGISTER_TOOLS         = 4
)

type MfsPacket struct {
	data []byte
	pos  int
}

func newPacketForWrite(packetType uint32) *MfsPacket {
	res := MfsPacket{
		data: make([]byte, 8, 1024+8),
	}
	res.pos = 8
	binary.BigEndian.PutUint32(res.data, packetType)
	return &res
}

func newPacketForRead(packetType, packetLen uint32) *MfsPacket {
	res := MfsPacket{
		data: make([]byte, 8+packetLen),
	}
	res.pos = 8
	binary.BigEndian.PutUint32(res.data, packetType)
	binary.BigEndian.PutUint32(res.data[4:], packetLen)
	return &res
}

func (p *MfsPacket) BufferForRead() []byte {
	return p.data[8:]
}

func (p *MfsPacket) Slide(by int) {
	p.pos += by
}

// gets
func (p *MfsPacket) Get8() uint8 {
	defer p.Slide(1)
	return p.data[p.pos]
}

func (p *MfsPacket) Get16() uint16 {
	defer p.Slide(2)
	return binary.BigEndian.Uint16(p.data[p.pos:])
}

func (p *MfsPacket) Get32() uint32 {
	defer p.Slide(4)
	return binary.BigEndian.Uint32(p.data[p.pos:])
}

func (p *MfsPacket) Get64() uint64 {
	defer p.Slide(8)
	return binary.BigEndian.Uint64(p.data[p.pos:])
}

// puts
func (p *MfsPacket) Put8(val uint8) {
	p.data = append(p.data, val)
}

func (p *MfsPacket) Put16(val uint16) {
	buff := make([]byte, 2)
	binary.BigEndian.PutUint16(buff, val)
	p.data = append(p.data, buff...)
}

func (p *MfsPacket) Put32(val uint32) {
	buff := make([]byte, 4)
	binary.BigEndian.PutUint32(buff, val)
	p.data = append(p.data, buff...)
}

func (p *MfsPacket) Put64(val uint64) {
	buff := make([]byte, 8)
	binary.BigEndian.PutUint64(buff, val)
	p.data = append(p.data, buff...)
}

func (p *MfsPacket) PutBytesNoLen(val []byte) {
	p.data = append(p.data, val...)
}

//

func (p *MfsPacket) GetTypeLen() (uint32, uint32) {
	return binary.BigEndian.Uint32(p.data), binary.BigEndian.Uint32(p.data[4:])
}

func (p *MfsPacket) GetType() uint32 {
	return binary.BigEndian.Uint32(p.data)
}

func (p *MfsPacket) GetLen() uint32 {
	return binary.BigEndian.Uint32(p.data[4:])
}

//

func ReadPacket(conn net.Conn) (*MfsPacket, error) {
	headers := make([]byte, 8)
	if leng, err := io.ReadFull(conn, headers); err != nil {
		return nil, fmt.Errorf("ReadPacket couldn't read headers (read only %d of 8 bytes)", leng)
	}
	packetType := binary.BigEndian.Uint32(headers)
	packetLen := binary.BigEndian.Uint32(headers[4:])

	log.Debugf("ReadPacket: got packet %d with length %d", packetType, packetLen)
	res := newPacketForRead(packetType, packetLen)

	if packetLen > 0 {
		if leng, err := io.ReadFull(conn, res.data[8:]); err != nil {
			return nil, fmt.Errorf("ReadPacket couldn't read packet (read only %d of %d bytes)", leng, len(res.data[8:]))
		}
	}
	return res, nil
}

func (p *MfsPacket) PrepareForSend() {
	binary.BigEndian.PutUint32(p.data[4:], uint32(len(p.data)-8))
}

func (p *MfsPacket) SendPacket(conn net.Conn) error {
	p.PrepareForSend()
	wrote, err := conn.Write(p.data)
	if err != nil {
		return err
	}
	if wrote != len(p.data) {
		return fmt.Errorf("SendPacket couldn't send packet (send only %d of %d bytes)", wrote, len(p.data))
	}
	return nil
}

func (p *MfsPacket) SendAndReceive(conn net.Conn, expectedType uint32) (*MfsPacket, error) {
	if err := p.SendPacket(conn); err != nil {
		return nil, err
	}

	for {
		pac, err := ReadPacket(conn)
		if err != nil {
			return nil, err
		}
		packetType, _ := pac.GetTypeLen()
		if packetType != ANTOAN_NOP {
			if packetType != expectedType {
				log.Errorf("MfsPacket::SendAndReceive -- expected packet: %d, got: %d", expectedType, packetType)
				return nil, fmt.Errorf("SendAndReceive got wrong packet (expected %d, got %d)", expectedType, packetType)
			}
			return pac, nil
		}
	}
}

func MasterRegister(conn net.Conn, sessionId uint32) error {
	p := newPacketForWrite(CLTOMA_FUSE_REGISTER)
	p.PutBytesNoLen([]byte(FUSE_REGISTER_BLOB_ACL))
	p.Put8(REGISTER_TOOLS)
	p.Put32(sessionId)
	p.Put16(3)  // versmaj
	p.Put8(0)   // versmid
	p.Put8(113) // versmin
	r, err := p.SendAndReceive(conn, MATOCL_FUSE_REGISTER)
	if err != nil {
		return err
	}
	if _, leng := r.GetTypeLen(); leng != 1 {
		return fmt.Errorf("MasterRegister expected packet len 1, got %d", leng)
	}
	status := r.Get8()
	if status != 0 {
		return fmt.Errorf("MasterRegister got status: %d", status)
	}
	return nil
}

func GetRegisterInfo(mountpoint string) (uint32, string, error) {
	masterinfoPath := mountpoint + "/.masterinfo"
	// check file length before reading it
	f, err := os.Stat(masterinfoPath)
	if err != nil {
		return 0, "", errors.New("GetRegisterInfo couldn't stat masterinfo")
	}
	if f.Size() != 10 && f.Size() != 14 {
		return 0, "", errors.New("GetRegisterInfo masterinfo size isn't equal to 10 or 14")
	}
	masterinfo, err := ioutil.ReadFile(masterinfoPath)
	masterip := binary.BigEndian.Uint32(masterinfo[0:])
	masternetip := make(net.IP, 4)
	binary.BigEndian.PutUint32(masternetip, masterip)
	masterport := binary.BigEndian.Uint16(masterinfo[4:])
	sessionid := binary.BigEndian.Uint32(masterinfo[6:])
	var masterversion uint32 = 0
	if f.Size() == 14 {
		masterversion = binary.BigEndian.Uint32(masterinfo[10:])
	}
	_ = masterversion

	return sessionid, fmt.Sprintf("%s:%d", masternetip.String(), masterport), nil
}

func MasterConnect(mountedPath string) (net.Conn, error) {
	sessionId, address, err := GetRegisterInfo(mountedPath)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	if err := MasterRegister(conn, sessionId); err != nil {
		return nil, err
	}
	return conn, nil
}

func GetInode(path string) (uint32, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("GetInode stat conversion err")
	}
	return uint32(stat.Ino), nil
}

/*
func SetQuota(path string, size uint64) (uint64, error) {
	log.Infof("SetQuota (path %s, size %d)", path, size)
	conn, err := MasterConnect(path)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Errorf("SetQuota -- conn.Close() error: %s", err.Error())
		}
	}()
	inode, err := GetInode(path)
	if err != nil {
		return 0, err
	}
	packet := newPacketForWrite(CLTOMA_FUSE_QUOTACONTROL)
	packet.Put32(0) // msgid
	packet.Put32(inode)
	packet.Put8(QUOTA_FLAG_HSIZE)
	packet.Put32(0)    // graceperiod
	packet.Put32(0)    // sinodes
	packet.Put64(0)    // slength
	packet.Put64(0)    // ssize
	packet.Put64(0)    // srealsize
	packet.Put32(0)    // hinodes
	packet.Put64(0)    // hlength
	packet.Put64(size) // hsize
	packet.Put64(0)    // hrealsize

	resp, err := packet.SendAndReceive(conn, MATOCL_FUSE_QUOTACONTROL)
	if err != nil {
		return 0, err
	}
	leng := resp.GetLen()
	if leng == 1 {
		return 0, fmt.Errorf("SetQuota got status: %d", resp.Get8())
	} else if leng != 93 && leng != 89 {
		return 0, fmt.Errorf("SetQuota response wrong leng (%d)", leng)
	} else if resp.Get32() != 0 { // msgid
		return 0, errors.New("SetQuota msgid response is not 0")
	}
	if leng == 89 {
		resp.Slide(41)
	} else {
		resp.Slide(45)
	}
	hsize := resp.Get64()
	// 36 left
	if hsize != size {
		log.Warningf("SetQuota -- requested %d hard quota size, got %d", size, hsize)
	}
	return hsize, nil
}
*/

func SetQuota(path string, size uint64) (uint64, error) {
	if size <= 0 {
		return 0, errors.New("SetQuota quota size must be positive")
	}
	return QuotaControl(path, size)
}

func GetQuota(path string) (uint64, error) {
	return QuotaControl(path, 0)
}

func QuotaControl(path string, size uint64) (uint64, error) {
	log.Infof("QuotaControl (path %s, size %d)", path, size)
	conn, err := MasterConnect(path)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Errorf("QuotaControl -- conn.Close() error: %s", err.Error())
		}
	}()
	inode, err := GetInode(path)
	if err != nil {
		return 0, err
	}
	packet := newPacketForWrite(CLTOMA_FUSE_QUOTACONTROL)
	packet.Put32(0) // msgid
	packet.Put32(inode)
	if size == 0 {
		packet.Put8(0)
	} else {
		packet.Put8(QUOTA_FLAG_HSIZE)
		packet.Put32(0)    // graceperiod
		packet.Put32(0)    // sinodes
		packet.Put64(0)    // slength
		packet.Put64(0)    // ssize
		packet.Put64(0)    // srealsize
		packet.Put32(0)    // hinodes
		packet.Put64(0)    // hlength
		packet.Put64(size) // hsize
		packet.Put64(0)    // hrealsize
	}
	resp, err := packet.SendAndReceive(conn, MATOCL_FUSE_QUOTACONTROL)
	if err != nil {
		return 0, err
	}
	leng := resp.GetLen()
	if leng == 1 {
		return 0, fmt.Errorf("QuotaControl got status: %d", resp.Get8())
	} else if leng != 93 && leng != 89 {
		return 0, fmt.Errorf("QuotaControl response wrong leng (%d)", leng)
	} else if resp.Get32() != 0 { // msgid
		return 0, errors.New("QuotaControl msgid response is not 0")
	}
	if leng == 89 {
		resp.Slide(41)
	} else {
		resp.Slide(45)
	}
	hsize := resp.Get64()
	// 36 left
	if size > 0 && hsize != size {
		log.Warningf("QuotaControl -- requested %d hard quota size, got %d", size, hsize)
	}
	return hsize, nil
}
