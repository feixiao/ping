package ping

import (
	"testing"
)

func TestPing(t *testing.T) {
	if istrue := Ping("192.168.1.116", 10); !istrue {
		t.Error("ping is failed")
	} else {
		t.Log("ping is ok")
	}
}
