package ping

import (
	"testing"
)

func TestPing(t *testing.T) {
	if istrue := Ping("127.0.0.1", 10); !istrue {
		t.Error("ping is failed")
	} else {
		t.Log("ping is ok")
	}
}
