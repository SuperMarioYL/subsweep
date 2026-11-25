package server_test

import (
	"testing"

	"github.com/SuperMarioYL/subsweep/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The UI promises to stay on this machine, so anything that is not a loopback
// address must be refused before a socket opens — including the bare ":8787"
// form, which net.Listen reads as every interface.
func TestValidateAddrRejectsNonLoopback(t *testing.T) {
	require.NoError(t, server.ValidateAddr(server.DefaultAddr))
	for _, ok := range []string{"127.0.0.1:8787", "127.0.0.2:8787", "localhost:8787", "[::1]:8787"} {
		assert.NoError(t, server.ValidateAddr(ok), "loopback address %q must be accepted", ok)
	}
	for _, bad := range []string{"0.0.0.0:8787", ":8787", "192.168.1.10:8787", "example.com:8787", "127.0.0.1"} {
		assert.Error(t, server.ValidateAddr(bad), "non-loopback address %q must be refused", bad)
	}
}
