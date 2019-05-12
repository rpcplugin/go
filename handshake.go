package rpcplugin

import (
	"context"
	"errors"

	"github.com/apparentlymart/go-ctxenv/ctxenv"
)

// HandshakeConfig contains settings that the client and server must both
// agree on in order for a plugin connection to be established.
type HandshakeConfig struct {
	// CookieKey and CookieValue are used together to return an error if
	// a server program is run directly from the command line, rather than as
	// a child process of a plugin client.
	//
	// CookieKey is used as an environment variable name and CookieValue as
	// its value. The client sets this variable when it launches plugin server
	// child processes, and the server programs check for the variable and
	// corresponding value and will return an error immediately in case of
	// a mismatch.
	//
	// CookieKey will usually be something users can identify as being related
	// to the calling application. CookieValue should be something unlikely to
	// be set manually for some other reason, such as a specific (hard-coded)
	// uuid.
	//
	// This is not a security feature. It is just a heuristic to allow plugin
	// server programs to give good user feedback if a user tries to launch
	// them directly, rather than showing the user the plugin handshake line.
	CookieKey, CookieValue string
}

// NotChildProcessError is the error value returned from Serve if it does not
// detect the "cookie" environment variable that is a heuristic for detecting
// whether or not the server is being launched from the expected parent process.
//
// Use this to detect that error case and potentially show a more
// application-specific error message, e.g. explaining how to install a plugin
// for that application.
var NotChildProcessError error

func init() {
	NotChildProcessError = errors.New("plugin server program launched outside of its expected host")
}

// haveHandshakeCookie is an internal helper to check whether the configured
// handshake cookie environment variable is present for the current process.
func haveHandshakeCookie(ctx context.Context, cfg *HandshakeConfig) bool {
	if cfg.CookieKey == "" {
		panic("no handshake cookie key is configured")
	}
	v := ctxenv.Getenv(ctx, cfg.CookieKey)
	return v == cfg.CookieValue
}
