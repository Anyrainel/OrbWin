// Package gateway provides the private relay-to-Windows transport. It has only
// standard-library dependencies and never reads OS roots, keys or trust stores.
package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"net"
	"strings"
	"time"
)

const Protocol = "orbwin-gateway/1"
const RootBudget = 16384

type Mode byte

const (
	HTTP  Mode = 1
	HTTPS Mode = 2
)

type Identity struct {
	root    []byte
	server  *tls.Config
	session string
	expires time.Time
}

func ValidSession(session string) bool {
	raw, err := hex.DecodeString(session)
	return err == nil && len(raw) == 16 && session == strings.ToLower(session)
}
func hostname(session string) string { return "gateway-" + session + ".orbwin.invalid" }
func number() (*big.Int, error) {
	value, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	return value.Add(value, big.NewInt(1)), nil
}

// Keys remain in Windows memory. Only the public root is passed to the relay.
// The root signer is not retained; each process/rotation creates a new identity.
func NewIdentity(session string, now time.Time) (*Identity, error) {
	if !ValidSession(session) {
		return nil, errors.New("exact gateway session required")
	}
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := number()
	if err != nil {
		return nil, err
	}
	expires := now.AddDate(1, 0, 0)
	rootTemplate := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "OrbWin private gateway " + session}, NotBefore: now.Add(-5 * time.Minute), NotAfter: expires, IsCA: true, BasicConstraintsValid: true, MaxPathLenZero: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err = number()
	if err != nil {
		return nil, err
	}
	leafTemplate := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: hostname(session)}, DNSNames: []string{hostname(session)}, NotBefore: root.NotBefore, NotAfter: expires, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, root, &key.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return nil, err
	}
	configuration := &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{Protocol}, Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER}, PrivateKey: key, Leaf: leaf}}}
	configuration.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		if hello.ServerName != hostname(session) {
			return nil, errors.New("gateway instance name differs")
		}
		return nil, nil
	}
	return &Identity{root: rootDER, server: configuration, session: session, expires: leaf.NotAfter}, nil
}
func (identity *Identity) PublicRoot() []byte        { return append([]byte(nil), identity.root...) }
func (identity *Identity) ServerConfig() *tls.Config { return identity.server.Clone() }
func (identity *Identity) ExpiresAt() time.Time      { return identity.expires }

func ClientConfig(session string, rootDER []byte) (*tls.Config, error) {
	if !ValidSession(session) || len(rootDER) == 0 || len(rootDER) > RootBudget {
		return nil, errors.New("bounded gateway root and exact session required")
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return nil, errors.New("gateway public root is invalid")
	}
	key, ok := root.PublicKey.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() || !root.IsCA || !root.BasicConstraintsValid || !root.MaxPathLenZero || root.MaxPathLen != 0 || root.KeyUsage != x509.KeyUsageCertSign || root.Subject.CommonName != "OrbWin private gateway "+session || root.CheckSignatureFrom(root) != nil || len(root.UnhandledCriticalExtensions) != 0 {
		return nil, errors.New("gateway root does not match its private-purpose profile/session")
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	return &tls.Config{MinVersion: tls.VersionTLS12, ServerName: hostname(session), RootCAs: pool, NextProtos: []string{Protocol}}, nil
}
func modeFrame(mode Mode) ([8]byte, error) {
	frame := [8]byte{'O', 'R', 'B', 'G', 1, 0, 0, byte(mode)}
	if mode != HTTP && mode != HTTPS {
		return frame, errors.New("unsupported gateway mode")
	}
	return frame, nil
}
func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		count, err := writer.Write(data)
		if err != nil {
			return err
		}
		if count <= 0 || count > len(data) {
			return io.ErrShortWrite
		}
		data = data[count:]
	}
	return nil
}

// No application bytes are written until standard certificate verification,
// exact instance SNI/ALPN and the gateway's mode acknowledgement all succeed.
// Context/deadline govern setup; CopyDuplex owns the established stream lifetime.
func Dial(ctx context.Context, address, session string, root []byte, mode Mode) (*tls.Conn, error) {
	frame, err := modeFrame(mode)
	if err != nil {
		return nil, err
	}
	configuration, err := ClientConfig(session, root)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	connection, err := (&tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}, Config: configuration}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	secure := connection.(*tls.Conn)
	accepted := false
	defer func() {
		if !accepted {
			secure.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { secure.Close() })
	defer stop()
	if secure.ConnectionState().NegotiatedProtocol != Protocol {
		return nil, errors.New("gateway did not negotiate its private protocol")
	}
	deadline, _ := ctx.Deadline()
	if err := secure.SetDeadline(deadline); err != nil {
		return nil, err
	}
	if err := writeAll(secure, frame[:]); err != nil {
		return nil, err
	}
	var acknowledgement [2]byte
	if _, err := io.ReadFull(secure, acknowledgement[:]); err != nil {
		return nil, err
	}
	if acknowledgement != [2]byte{1, byte(mode)} {
		return nil, errors.New("gateway mode was not acknowledged")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := secure.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	accepted = true
	return secure, nil
}

// AcceptMode must run on a connection configured with this process identity.
// Prefix parsing consumes exactly eight bytes, leaving coalesced payload intact.
func AcceptMode(ctx context.Context, connection *tls.Conn) (Mode, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { connection.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		return 0, err
	}
	if err := connection.HandshakeContext(ctx); err != nil {
		return 0, err
	}
	if connection.ConnectionState().NegotiatedProtocol != Protocol {
		return 0, errors.New("gateway protocol negotiation required")
	}
	var frame [8]byte
	if _, err := io.ReadFull(connection, frame[:]); err != nil {
		return 0, err
	}
	mode := Mode(frame[7])
	expected, err := modeFrame(mode)
	if err != nil || frame != expected {
		return 0, errors.New("invalid gateway version/mode frame")
	}
	if err := writeAll(connection, []byte{1, byte(mode)}); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		return 0, err
	}
	return mode, nil
}
