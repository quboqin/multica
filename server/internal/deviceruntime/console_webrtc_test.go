package deviceruntime

import (
	"strings"
	"testing"
)

func TestConsoleKeepsRequestedDeviceBindingWhenDeviceDisconnects(t *testing.T) {
	if strings.Contains(consoleWebRTC, "?requestedSerial:(select.value||'')") {
		t.Fatal("requested serial still falls back to the first listed device")
	}
	for _, contract := range []string{
		"target=requestedSerial?online.find(d=>d.serial===requestedSerial):online[0]",
		"Waiting for '+requestedSerial+' to reconnect.",
		"devicePoll=setTimeout(devices,1000)",
	} {
		if !strings.Contains(consoleWebRTC, contract) {
			t.Fatalf("device reconnect contract %q is missing", contract)
		}
	}
}
