// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// taken from http://golang.org/src/pkg/net/ipraw_test.go

package ping

import (
	"bytes"
	"errors"
	"net"
	"os"
	"time"
	lg "github.com/feixiao/log4go"
	"context"
	"fmt"
)

const (
	icmpv4EchoRequest = 8
	icmpv4EchoReply   = 0
	icmpv6EchoRequest = 128
	icmpv6EchoReply   = 129

	icmpMaxPacketSize = 1024
)

type icmpMessage struct {
	Type     int             // type
	Code     int             // code
	Checksum int             // checksum
	Body     icmpMessageBody // body
}

type icmpMessageBody interface {
	Len() int
	Marshal() ([]byte, error)
}

// Marshal returns the binary enconding of the ICMP echo request or
// reply message m.
func (m *icmpMessage) Marshal() ([]byte, error) {
	b := []byte{byte(m.Type), byte(m.Code), 0, 0}
	if m.Body != nil && m.Body.Len() != 0 {
		mb, err := m.Body.Marshal()
		if err != nil {
			return nil, err
		}
		b = append(b, mb...)
	}
	switch m.Type {
	case icmpv6EchoRequest, icmpv6EchoReply:
		return b, nil
	}
	csumcv := len(b) - 1 // checksum coverage
	s := uint32(0)
	for i := 0; i < csumcv; i += 2 {
		s += uint32(b[i+1])<<8 | uint32(b[i])
	}
	if csumcv&1 == 0 {
		s += uint32(b[csumcv])
	}
	s = s>>16 + s&0xffff
	s = s + s>>16
	// Place checksum back in header; using ^= avoids the
	// assumption the checksum bytes are zero.
	b[2] ^= byte(^s & 0xff)
	b[3] ^= byte(^s >> 8)
	return b, nil
}

// parseICMPMessage parses b as an ICMP message.
func parseICMPMessage(b []byte) (*icmpMessage, error) {
	msglen := len(b)
	if msglen < 4 {
		return nil, errors.New("message too short")
	}
	m := &icmpMessage{Type: int(b[0]), Code: int(b[1]), Checksum: int(b[2])<<8 | int(b[3])}
	if msglen > 4 {
		var err error
		switch m.Type {
		case icmpv4EchoRequest, icmpv4EchoReply, icmpv6EchoRequest, icmpv6EchoReply:
			m.Body, err = parseICMPEcho(b[4:])
			if err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}

// imcpEcho represenets an ICMP echo request or reply message body.
type icmpEcho struct {
	ID        int       // identifier
	Seq       int       // sequence number
	Timestamp time.Time // time stamp
	Data      []byte    // data
}

func (p *icmpEcho) Len() int {
	if p == nil {
		return 0
	}
	return 19 + len(p.Data)
}

// Marshal returns the binary enconding of the ICMP echo request or
// reply message body p.
func (p *icmpEcho) Marshal() ([]byte, error) {
	b := make([]byte, 19+len(p.Data))
	b[0], b[1] = byte(p.ID>>8), byte(p.ID&0xff)
	b[2], b[3] = byte(p.Seq>>8), byte(p.Seq&0xff)
	timeencode, _ := time.Now().GobEncode()
	copy(b[4:19], timeencode)
	copy(b[19:], p.Data)
	return b, nil
}

// parseICMPEcho parses b as an ICMP echo request or reply message body.
func parseICMPEcho(b []byte) (*icmpEcho, error) {
	bodylen := len(b)
	p := &icmpEcho{ID: int(b[0])<<8 | int(b[1]), Seq: int(b[2])<<8 | int(b[3])}
	p.Timestamp.GobDecode(b[4:19])
	if bodylen > 19 {
		p.Data = make([]byte, bodylen-19)
		copy(p.Data, b[19:])
	}
	return p, nil
}

func Ping(address string, timeout time.Duration) bool {
	//var p Pinger
	p:=NewPinger(1,200*time.Millisecond,128)
	err,_:= p.ping(address, timeout)
	return err == nil
}

func newIcmpPacket(xseq int,data []byte) (wb []byte, err error) {
	typ := icmpv4EchoRequest
	xid := os.Getpid() & 0xffff
	wb, err = (&icmpMessage{
		Type: typ, Code: 0,
		Body: &icmpEcho{
			ID: xid, Seq: xseq,
			Data: data,
		},
	}).Marshal()
	if err != nil {
		return wb, err
	}
	return wb, err
}

/***********************************************************************************************************************************************************/
type Stat struct {
	Seq 	 int
	Rtt      time.Duration
	DataSize int
}

func (s *Stat) String() string {
	return fmt.Sprintf("size: %d seq: %d   rtt: %v\n",s.DataSize,s.Seq,s.Rtt)
}

type Pinger struct {
	Count		int 		// ping的次数
	Duration 	time.Duration	// 每个ICMP包发送的间隔
	DataSize	int 		// 发送的包大小
	Statistic 	int		// 接受的到的数据包
	Address         string		// 目标地址

	Stats		[]*Stat		// 每个ICMP包的统计信息
	Timeout		time.Duration	// socket读超时

	ctx 		context.Context 	// goroutine上下文管理
	cancel		context.CancelFunc	// 通知相关goroutine退出
	readyChan	chan bool		// 同步发送和接受数据

	LastErr 	error			// 最后错误值
}



func NewPinger(cnt int,duration time.Duration,dataSize int ) *Pinger {
	p := &Pinger{
		Count : cnt,
		Duration  : duration,
		DataSize : dataSize,
		readyChan : make(chan bool,1),
	}
	return p;
}

func (p *Pinger)writeToRemote(c net.Conn,xseq int ) error {

	c.SetWriteDeadline(time.Now().Add(p.Timeout))
	data:=bytes.Repeat([]byte{1},p.DataSize)
	wb, err := newIcmpPacket(xseq,data)
	if err != nil {
		return err
	}
	lg.Info("wbSize : %d",len(wb))
	if _, err := c.Write(wb); err != nil {
		return err
	}
	return nil
}

func (p *Pinger) recvFromRemote(c net.Conn) {
	var err error
	var m *icmpMessage
	rb := make([]byte, icmpMaxPacketSize)
	var n int

	p.readyChan <- true
	for {
		select {
		case <- p.ctx.Done():
			lg.Info("recvFromRemote is canceled！");
			return
		default:

		}

		c.SetReadDeadline(time.Now().Add(p.Timeout))
		if n, err = c.Read(rb); err != nil {
			p.LastErr = err
			return
		}
		ipb := ipv4Payload(rb)
		if m, err = parseICMPMessage(ipb); err != nil {
			p.LastErr = err
			lg.Error(err)
			return
		}
		if v, ok := m.Body.(*icmpEcho); ok {
			lg.Info("bsize: %d seq: %d   rtt: %v\n",n,v.Seq, time.Since(v.Timestamp))
			s := &Stat{
				DataSize:n,
				Seq:v.Seq,
				Rtt:time.Since(v.Timestamp),
			}
			p.Stats = append(p.Stats,s)
			p.Statistic ++
		}

		switch m.Type {
		case icmpv4EchoRequest, icmpv6EchoRequest:
			continue
		}
	}
}



func (p *Pinger)ping(address string, timeout time.Duration)(error,[]*Stat){
	c, err := net.Dial("ip4:icmp", address)
	if err != nil {
		lg.Error(err)
		return err,nil
	}
	p.Address = address
	p.Timeout = timeout
	defer c.Close()

	p.ctx, p.cancel = context.WithCancel(context.Background())

	//go p.writeToRemote(c)
	go p.recvFromRemote(c)
	<- p.readyChan

	for i:=0;i< p.Count; i++ {
		lg.Info(i)
		p.writeToRemote(c,i)
		time.Sleep(p.Duration)
	}

	p.cancel()
	return err,p.Stats
}
// 全部发送的包个数，丢包个数，丢包率
func  (p *Pinger)PacketLoss()(int, int,float32){
	lost := p.Count- p.Statistic
	return p.Count, lost,float32(lost) / float32(p.Count)
}

func ipv4Payload(b []byte) []byte {
	if len(b) < 20 {
		return b
	}
	hdrlen := int(b[0]&0x0f) << 2
	return b[hdrlen:]
}
