package quicktunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"

	capnp "zombiezen.com/go/capnproto2"
	"zombiezen.com/go/capnproto2/rpc"
)

// The edge's RegistrationServer interface, from cloudflared's
// tunnelrpc/proto/tunnelrpc.capnp. Only the two calls a connection needs are
// encoded here, by hand, instead of vendoring the 4,800 lines capnpc-go
// generates for the whole schema. The struct layouts below are the ones that
// schema compiles to; registration_test.go pins them.
const (
	registrationServerID = 0xf71695ec7fe85497
	methodRegister       = 0
	methodUnregister     = 1
)

// registration is one registered connection's RPC session. Closing it, or the
// stream under it, is how the edge learns the connection is gone.
type registration struct {
	conn *rpc.Conn
	boot capnp.Client
}

// registerParams is what registerConnection is called with.
type registerParams struct {
	creds    Credentials
	tunnelID [16]byte
	clientID [16]byte
	version  string
	attempts uint8 // previous failed attempts, which the edge uses for backoff
}

// register calls registerConnection over stream and returns where the
// connection landed, as an airport code. A refusal the edge says not to
// retry comes back wrapped in errRejected.
func register(ctx context.Context, stream io.ReadWriteCloser, p registerParams) (*registration, string, error) {
	conn := rpc.NewConn(rpc.StreamTransport(stream), rpc.ConnLog(quietLog{}))
	reg := &registration{conn: conn, boot: conn.Bootstrap(ctx)}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	res, err := reg.boot.Call(&capnp.Call{
		Ctx:        ctx,
		Method:     capnp.Method{InterfaceID: registrationServerID, MethodID: methodRegister, InterfaceName: "RegistrationServer", MethodName: "registerConnection"},
		ParamsSize: capnp.ObjectSize{DataSize: 8, PointerCount: 3},
		ParamsFunc: p.encode,
	}).Struct()
	if err != nil {
		reg.close()
		return nil, "", fmt.Errorf("register connection: %w", err)
	}
	location, err := decodeConnectionResponse(res)
	if err != nil {
		reg.close()
		return nil, "", err
	}
	return reg, location, nil
}

// encode fills registerConnection's parameters:
//
//	(auth :TunnelAuth, tunnelId :Data, connIndex :UInt8, options :ConnectionOptions)
func (p registerParams) encode(s capnp.Struct) error {
	seg := s.Segment()

	auth, err := capnp.NewStruct(seg, capnp.ObjectSize{PointerCount: 2})
	if err != nil {
		return err
	}
	if err := auth.SetText(0, p.creds.AccountTag); err != nil {
		return err
	}
	if err := auth.SetData(1, p.creds.Secret); err != nil {
		return err
	}

	client, err := capnp.NewStruct(seg, capnp.ObjectSize{PointerCount: 4})
	if err != nil {
		return err
	}
	if err := client.SetData(0, p.clientID[:]); err != nil {
		return err
	}
	// serialized_headers: response headers travel in one header of their own,
	// so HTTP/2's header rules are not applied to an HTTP/1 origin's headers.
	features, err := capnp.NewTextList(seg, 1)
	if err != nil {
		return err
	}
	if err := features.Set(0, "serialized_headers"); err != nil {
		return err
	}
	if err := client.SetPtr(1, features.ToPtr()); err != nil {
		return err
	}
	if err := client.SetText(2, p.version); err != nil {
		return err
	}
	if err := client.SetText(3, runtime.GOOS+"_"+runtime.GOARCH); err != nil {
		return err
	}

	opts, err := capnp.NewStruct(seg, capnp.ObjectSize{DataSize: 8, PointerCount: 2})
	if err != nil {
		return err
	}
	if err := opts.SetPtr(0, client.ToPtr()); err != nil {
		return err
	}
	opts.SetUint8(2, p.attempts) // numPreviousAttempts

	if err := s.SetPtr(0, auth.ToPtr()); err != nil {
		return err
	}
	if err := s.SetData(1, p.tunnelID[:]); err != nil {
		return err
	}
	s.SetUint8(0, 0) // connIndex: a quick tunnel has the one connection
	return s.SetPtr(2, opts.ToPtr())
}

// decodeConnectionResponse reads registerConnection's result:
//
//	(result :ConnectionResponse)
//	ConnectionResponse.result :union { error @0 :ConnectionError; connectionDetails @1 :ConnectionDetails }
//	ConnectionError (cause :Text, retryAfter :Int64, shouldRetry :Bool)
//	ConnectionDetails (uuid :Data, locationName :Text, tunnelIsRemotelyManaged :Bool)
func decodeConnectionResponse(results capnp.Struct) (string, error) {
	p, err := results.Ptr(0)
	if err != nil {
		return "", err
	}
	resp := p.Struct()
	inner, err := resp.Ptr(0)
	if err != nil {
		return "", err
	}
	s := inner.Struct()
	switch resp.Uint16(0) {
	case 0:
		cause, _ := s.Ptr(0)
		err := fmt.Errorf("edge refused the connection: %s", cause.Text())
		if !s.Bit(64) { // shouldRetry
			err = fmt.Errorf("%w: %s", errRejected, cause.Text())
		}
		return "", err
	case 1:
		loc, _ := s.Ptr(1)
		return loc.Text(), nil
	}
	return "", errors.New("edge answered registration with an unknown result")
}

// unregister tells the edge this connection is going away on purpose, so it
// stops sending it requests before the socket closes.
func (r *registration) unregister(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	r.boot.Call(&capnp.Call{ //nolint:errcheck // best effort on the way out
		Ctx:        ctx,
		Method:     capnp.Method{InterfaceID: registrationServerID, MethodID: methodUnregister, InterfaceName: "RegistrationServer", MethodName: "unregisterConnection"},
		ParamsFunc: func(capnp.Struct) error { return nil },
	}).Struct()
}

func (r *registration) close() {
	r.boot.Close()
	r.conn.Close()
}

// quietLog drops the RPC library's own logging, which otherwise goes to
// stdout and says nothing a user could act on.
type quietLog struct{}

func (quietLog) Infof(context.Context, string, ...interface{})  {}
func (quietLog) Errorf(context.Context, string, ...interface{}) {}
