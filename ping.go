// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// taken from http://golang.org/src/pkg/net/ipraw_test.go

package ping

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

const (
	icmpv4EchoRequest = 8
	icmpv4EchoReply   = 0
	icmpv6EchoRequest = 128
	icmpv6EchoReply   = 129
)

var trans chan bool

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

func Ping(address string, timeout int) bool {
	err := Pinger(address, timeout)
	return err == nil
}

func newIcmpPacket(xseq int) (wb []byte, err error) {
	typ := icmpv4EchoRequest
	xid := os.Getpid() & 0xffff
	wb, err = (&icmpMessage{
		Type: typ, Code: 0,
		Body: &icmpEcho{
			ID: xid, Seq: xseq,
			Data: bytes.Repeat([]byte("Go Go Gadget Ping!!!"), 3),
		},
	}).Marshal()
	if err != nil {
		return wb, err
	}
	return wb, err
}

func writeToRemote(c net.Conn) {
	ticker := time.NewTicker(1 * time.Second)
	xseq := 1
	for {
		<-trans
		<-ticker.C
		wb, err := newIcmpPacket(xseq)
		if err != nil {
			return
		}

		if _, err := c.Write(wb); err != nil {
			fmt.Println("write err")
			break
		}
		xseq++
	}
}

func Pinger(address string, timeout int) error {
	trans = make(chan bool, 1)
	c, err := net.Dial("ip4:icmp", address)
	if err != nil {
		fmt.Println("err hello")
		return err
	}
	c.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
	defer c.Close()

	go writeToRemote(c)

	var m *icmpMessage
	wb, _ := newIcmpPacket(0) //  to get send number of byte , for read .
	rb := make([]byte, len(wb))
	for {
		trans <- true
		if _, err = c.Read(rb); err != nil {
			fmt.Println("read err")
			return err
		}
		ipb := ipv4Payload(rb)
		if m, err = parseICMPMessage(ipb); err != nil {
			return err
		}
		if v, ok := m.Body.(*icmpEcho); ok {
			fmt.Printf("seq: %d   rtt: %v\n", v.Seq, time.Since(v.Timestamp))
		}

		switch m.Type {
		case icmpv4EchoRequest, icmpv6EchoRequest:
			continue
		}
		//break
	}
	return nil
}

func ipv4Payload(b []byte) []byte {
	if len(b) < 20 {
		return b
	}
	hdrlen := int(b[0]&0x0f) << 2
	return b[hdrlen:]
}
