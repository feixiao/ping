package ping

import (
	"testing"
	"time"
)

func TestBasicPing(t *testing.T) {
	if istrue := Ping("www.baidu.com", 1*time.Second); !istrue {
		t.Error("ping is failed")
	} else {
		t.Log("ping is ok")
	}
}

func TestPingInfo(t *testing.T){
	p := NewPinger(4,50*time.Millisecond,100)

	err,stats := p.ping("www.baidu.com", 1*time.Second)
	if err !=nil {
		t.Error(err)
	}

	t.Log(stats)

	t.Log(p.PacketLoss())

}
