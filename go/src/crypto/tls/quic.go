// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tls

import (
	"context"
	"errors"
	"fmt"
)

// QUICEncryptionLevel represents a QUIC encryption level used to transmit
// handshake messages.
type QUICEncryptionLevel int

const (
	QUICEncryptionLevelInitial = QUICEncryptionLevel(iota)
	QUICEncryptionLevelHandshake
	QUICEncryptionLevelApplication
)

func (l QUICEncryptionLevel) String() string {
	switch l {
	case QUICEncryptionLevelInitial:
		return "Initial"
	case QUICEncryptionLevelHandshake:
		return "Handshake"
	case QUICEncryptionLevelApplication:
		return "Application"
	default:
		return fmt.Sprintf("QUICEncryptionLevel(%v)", int(l))
	}
}

// A QUICConn represents a connection which uses a QUIC implementation as the underlying
// transport as described in RFC 9001.
//
// Methods of QUICConn are not safe for concurrent use.
type QUICConn struct {
	conn *Conn
}

// A QUICConfig configures a QUICConn.
type QUICConfig struct {
	TLSConfig *Config
}

// A QUICEventKind is a type of operation on a QUIC connection.
type QUICEventKind int

const (
	// QUICNoEvent indicates that there are no events available.
	QUICNoEvent QUICEventKind = iota

	// QUICSetReadSecret and QUICSetWriteSecret provide the read and write
	// secrets for a given encryption level.
	// QUICEvent.Level, QUICEvent.Data, and QUICEvent.Suite are set.
	//
	// Secrets for the Initial encryption level are derived from the initial
	// destination connection ID, and are not provided by the QUICConn.
	QUICSetReadSecret
	QUICSetWriteSecret

	// QUICWriteData provides data to send to the peer in CRYPTO frames.
	// QUICEvent.Data is set.
	QUICWriteData

	// QUICTransportParameters provides the peer's QUIC transport parameters.
	// QUICEvent.Data is set.
	QUICTransportParameters

	// QUICTransportParametersRequired indicates that the caller must provide
	// QUIC transport parameters to send to the peer. The caller should set
	// the transport parameters with QUICConn.SetTransportParameters and call
	// QUICConn.NextEvent again.
	//
	// If transport parameters are set before calling QUICConn.Start, the
	// connection will never generate a QUICTransportParametersRequired event.
	QUICTransportParametersRequired

	// QUICHandshakeDone indicates that the TLS handshake has completed.
	QUICHandshakeDone
)

// A QUICEvent is an event occurring on a QUIC connection.
//
// The type of event is specified by the Kind field.
// The contents of the other fields are kind-specific.
type QUICEvent struct {
	Kind QUICEventKind

	// Set for QUICSetReadSecret, QUICSetWriteSecret, and QUICWriteData.
	Level QUICEncryptionLevel

	// Set for QUICTransportParameters, QUICSetReadSecret, QUICSetWriteSecret, and QUICWriteData.
	// The contents are owned by crypto/tls, and are valid until the next NextEvent call.
	Data []byte

	// Set for QUICSetReadSecret and QUICSetWriteSecret.
	Suite uint16
}

type quicState struct {
	events    []QUICEvent
	nextEvent int

	// eventArr is a statically allocated event array, large enough to handle
	// the usual maximum number of events resulting from a single call:
	// transport parameters, Initial data, Handshake write and read secrets,
	// Handshake data, Application write secret, Application data.
	eventArr [7]QUICEvent

	started  bool
	signalc  chan struct{}   // handshake data is available to be read
	blockedc chan struct{}   // handshake is waiting for data, closed when done
	cancelc  <-chan struct{} // handshake has been canceled
	cancel   context.CancelFunc

	// readbuf is shared between HandleData and the handshake goroutine.
	// HandshakeCryptoData passes ownership to the handshake goroutine by
	// reading from signalc, and reclaims ownership by reading from blockedc.
	readbuf []byte

	transportParams []byte // to send to the peer
}

// QUICClient returns a new TLS client side connection using QUICTransport as the
// underlying transport. The config cannot be nil.
//
// The config's MinVersion must be at least TLS 1.3.
func QUICClient(config *QUICConfig) *QUICConn {
	return newQUICConn(Client(nil, config.TLSConfig))
}

// QUICServer returns a new TLS server side connection using QUICTransport as the
// underlying transport. The config cannot be nil.
//
// The config's MinVersion must be at least TLS 1.3.
func QUICServer(config *QUICConfig) *QUICConn {
	return newQUICConn(Server(nil, config.TLSConfig))
}

func newQUICConn(conn *Conn) *QUICConn {
	conn.quic = &quicState{
		signalc:  make(chan struct{}),
		blockedc: make(chan struct{}),
	}
	conn.quic.events = conn.quic.eventArr[:0]
	return &QUICConn{
		conn: conn,
	}
}

// Start starts the client or server handshake protocol.
// It may produce connection events, which may be read with NextEvent.
//
// Start must be called at most once.
func (q *QUICConn) Start(ctx context.Context) error {
	if q.conn.quic.started {
		return quicError(errors.New("tls: Start called more than once"))
	}
	q.conn.quic.started = true
	if q.conn.config.MinVersion < VersionTLS13 {
		return quicError(errors.New("tls: Config MinVersion must be at least TLS 1.13"))
	}
	go q.conn.HandshakeContext(ctx)
	if _, ok := <-q.conn.quic.blockedc; !ok {
		return q.conn.handshakeErr
	}
	return nil
}

// NextEvent returns the next event occurring on the connection.
// It returns an event with a Kind of QUICNoEvent when no events are available.
func (q *QUICConn) NextEvent() QUICEvent {
	qs := q.conn.quic
	if last := qs.nextEvent - 1; last >= 0 && len(qs.events[last].Data) > 0 {
		// Write over some of the previous event's data,
		// to catch callers erroniously retaining it.
		qs.events[last].Data[0] = 0
	}
	if qs.nextEvent >= len(qs.events) {
		qs.events = qs.events[:0]
		qs.nextEvent = 0
		return QUICEvent{Kind: QUICNoEvent}
	}
	e := qs.events[qs.nextEvent]
	qs.events[qs.nextEvent] = QUICEvent{} // zero out references to data
	qs.nextEvent++
	return e
}

// Close closes the connection and stops any in-progress handshake.
func (q *QUICConn) Close() error {
	if q.conn.quic.cancel == nil {
		return nil // never started
	}
	q.conn.quic.cancel()
	for range q.conn.quic.blockedc {
		// Wait for the handshake goroutine to return.
	}
	return q.conn.handshakeErr
}

// HandleData handles handshake bytes received from the peer.
// It may produce connection events, which may be read with NextEvent.
func (q *QUICConn) HandleData(level QUICEncryptionLevel, data []byte) error {
	c := q.conn
	if c.in.level != level {
		return quicError(c.in.setErrorLocked(errors.New("tls: handshake data received at wrong level")))
	}
	c.quic.readbuf = data
	<-c.quic.signalc
	_, ok := <-c.quic.blockedc
	if ok {
		// The handshake goroutine is waiting for more data.
		return nil
	}
	// The handshake goroutine has exited.
	c.hand.Write(c.quic.readbuf)
	c.quic.readbuf = nil
	for q.conn.hand.Len() >= 4 && q.conn.handshakeErr == nil {
		b := q.conn.hand.Bytes()
		n := int(b[1])<<16 | int(b[2])<<8 | int(b[3])
		if 4+n < len(b) {
			return nil
		}
		if err := q.conn.handlePostHandshakeMessage(); err != nil {
			return quicError(err)
		}
	}
	if q.conn.handshakeErr != nil {
		return quicError(q.conn.handshakeErr)
	}
	return nil
}

// ConnectionState returns basic TLS details about the connection.
func (q *QUICConn) ConnectionState() ConnectionState {
	return q.conn.ConnectionState()
}

// SetTransportParameters sets the transport parameters to send to the peer.
//
// Server connections may delay setting the transport parameters until after
// receiving the client's transport parameters. See QUICTransportParametersRequired.
func (q *QUICConn) SetTransportParameters(params []byte) {
	if params == nil {
		params = []byte{}
	}
	q.conn.quic.transportParams = params
	if q.conn.quic.started {
		<-q.conn.quic.signalc
		<-q.conn.quic.blockedc
	}
}

// quicError ensures err is an AlertError.
// If err is not already, quicError wraps it with alertInternalError.
func quicError(err error) error {
	if err == nil {
		return nil
	}
	var ae AlertError
	if errors.As(err, &ae) {
		return err
	}
	var a alert
	if !errors.As(err, &a) {
		a = alertInternalError
	}
	// Return an error wrapping the original error and an AlertError.
	// Truncate the text of the alert to 0 characters.
	return fmt.Errorf("%w%.0w", err, AlertError(a))
}

func (c *Conn) quicReadHandshakeBytes(n int) error {
	for c.hand.Len() < n {
		if err := c.quicWaitForSignal(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Conn) quicSetReadSecret(level QUICEncryptionLevel, suite uint16, secret []byte) {
	c.quic.events = append(c.quic.events, QUICEvent{
		Kind:  QUICSetReadSecret,
		Level: level,
		Suite: suite,
		Data:  secret,
	})
}

func (c *Conn) quicSetWriteSecret(level QUICEncryptionLevel, suite uint16, secret []byte) {
	c.quic.events = append(c.quic.events, QUICEvent{
		Kind:  QUICSetWriteSecret,
		Level: level,
		Suite: suite,
		Data:  secret,
	})
}

func (c *Conn) quicWriteCryptoData(level QUICEncryptionLevel, data []byte) {
	var last *QUICEvent
	if len(c.quic.events) > 0 {
		last = &c.quic.events[len(c.quic.events)-1]
	}
	if last == nil || last.Kind != QUICWriteData || last.Level != level {
		c.quic.events = append(c.quic.events, QUICEvent{
			Kind:  QUICWriteData,
			Level: level,
		})
		last = &c.quic.events[len(c.quic.events)-1]
	}
	last.Data = append(last.Data, data...)
}

func (c *Conn) quicSetTransportParameters(params []byte) {
	c.quic.events = append(c.quic.events, QUICEvent{
		Kind: QUICTransportParameters,
		Data: params,
	})
}

func (c *Conn) quicGetTransportParameters() ([]byte, error) {
	if c.quic.transportParams == nil {
		c.quic.events = append(c.quic.events, QUICEvent{
			Kind: QUICTransportParametersRequired,
		})
	}
	for c.quic.transportParams == nil {
		if err := c.quicWaitForSignal(); err != nil {
			return nil, err
		}
	}
	return c.quic.transportParams, nil
}

func (c *Conn) quicHandshakeComplete() {
	c.quic.events = append(c.quic.events, QUICEvent{
		Kind: QUICHandshakeDone,
	})
}

// quicWaitForSignal notifies the QUICConn that handshake progress is blocked,
// and waits for a signal that the handshake should proceed.
//
// The handshake may become blocked waiting for handshake bytes
// or for the user to provide transport parameters.
func (c *Conn) quicWaitForSignal() error {
	// Drop the handshake mutex while blocked to allow the user
	// to call ConnectionState before the handshake completes.
	c.handshakeMutex.Unlock()
	defer c.handshakeMutex.Lock()
	// Send on blockedc to notify the QUICConn that the handshake is blocked.
	// Exported methods of QUICConn wait for the handshake to become blocked
	// before returning to the user.
	select {
	case c.quic.blockedc <- struct{}{}:
	case <-c.quic.cancelc:
		return c.sendAlertLocked(alertCloseNotify)
	}
	// The QUICConn reads from signalc to notify us that the handshake may
	// be able to proceed. (The QUICConn reads, because we close signalc to
	// indicate that the handshake has completed.)
	select {
	case c.quic.signalc <- struct{}{}:
		c.hand.Write(c.quic.readbuf)
		c.quic.readbuf = nil
	case <-c.quic.cancelc:
		return c.sendAlertLocked(alertCloseNotify)
	}
	return nil
}
