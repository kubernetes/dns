/*
 * Copyright (c) 2019 by Farsight Security, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dnstap

import (
	"log"
	"net"
	"time"

	"github.com/farsightsec/golang-framestream"
)

// A FrameStreamSockOutput manages a socket connection and sends dnstap
// data over a framestream connection on that socket.
type FrameStreamSockOutput struct {
	outputChannel chan []byte
	address       net.Addr
	wait          chan bool
	dialer        *net.Dialer
	timeout       time.Duration
	retry         time.Duration
	flushTimeout  time.Duration
}

// NewFrameStreamSockOutput creates a FrameStreamSockOutput manaaging a
// connection to the given address.
func NewFrameStreamSockOutput(address net.Addr) (*FrameStreamSockOutput, error) {
	return &FrameStreamSockOutput{
		outputChannel: make(chan []byte, outputChannelSize),
		address:       address,
		wait:          make(chan bool),
		retry:         10 * time.Second,
		flushTimeout:  5 * time.Second,
		dialer: &net.Dialer{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// SetTimeout sets the write timeout for data and control messages and the
// read timeout for handshake responses on the FrameStreamSockOutput's
// connection. The default timeout is zero, for no timeout.
func (o *FrameStreamSockOutput) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// SetFlushTimeout sets the maximum time data will be kept in the output
// buffer.
//
// The default flush timeout is five seconds.
func (o *FrameStreamSockOutput) SetFlushTimeout(timeout time.Duration) {
	o.flushTimeout = timeout
}

// SetRetryInterval specifies how long the FrameStreamSockOutput will wait
// before re-establishing a failed connection. The default retry interval
// is 10 seconds.
func (o *FrameStreamSockOutput) SetRetryInterval(retry time.Duration) {
	o.retry = retry
}

// SetDialer replaces the default net.Dialer for re-establishing the
// the FrameStreamSockOutput connection. This can be used to set the
// timeout for connection establishment and enable keepalives
// new connections.
//
// FrameStreamSockOutput uses a default dialer with a 30 second
// timeout.
func (o *FrameStreamSockOutput) SetDialer(dialer *net.Dialer) {
	o.dialer = dialer
}

// GetOutputChannel returns the channel on which the
// FrameStreamSockOutput accepts data.
//
// GetOutputChannel satisifes the dnstap Output interface.
func (o *FrameStreamSockOutput) GetOutputChannel() chan []byte {
	return o.outputChannel
}

// A timedConn resets an associated timer on each Write to the underlying
// connection, and is used to implement the output's flush timeout.
type timedConn struct {
	net.Conn
	timer   *time.Timer
	timeout time.Duration

	// idle is true if the timer has fired and we have consumed
	// the time from its channel. We use this to prevent deadlocking
	// when resetting or stopping an already fired timer.
	idle bool
}

// SetIdle informs the timedConn that the associated timer is idle, i.e.
// it has fired and has not been reset.
func (t *timedConn) SetIdle() {
	t.idle = true
}

// Stop stops the underlying timer, consuming any time value if the timer
// had fired before Stop was called.
func (t *timedConn) StopTimer() {
	if !t.timer.Stop() && !t.idle {
		<-t.timer.C
	}
	t.idle = true
}

func (t *timedConn) Write(b []byte) (int, error) {
	t.StopTimer()
	t.timer.Reset(t.timeout)
	t.idle = false
	return t.Conn.Write(b)
}

func (t *timedConn) Close() error {
	t.StopTimer()
	return t.Conn.Close()
}

// RunOutputLoop reads data from the output channel and sends it over
// a connections to the FrameStreamSockOutput's address, establishing
// the connection as needed.
//
// RunOutputLoop satisifes the dnstap Output interface.
func (o *FrameStreamSockOutput) RunOutputLoop() {
	var enc *framestream.Encoder
	var err error

	// Start with the connection flush timer in a stopped state.
	// It will be reset by the first Write call on a new connection.
	conn := &timedConn{
		timer:   time.NewTimer(0),
		timeout: o.flushTimeout,
	}
	conn.StopTimer()

	defer func() {
		if enc != nil {
			enc.Flush()
			enc.Close()
		}
		if conn != nil {
			conn.Close()
		}
		close(o.wait)
	}()

	for {
		select {
		case frame, ok := <-o.outputChannel:
			if !ok {
				return
			}

			// the retry loop
			for ;; time.Sleep(o.retry) {
				if enc == nil {
					// connect the socket
					conn.Conn, err = o.dialer.Dial(o.address.Network(), o.address.String())
					if err != nil {
						log.Printf("Dial() failed: %v", err)
						continue // = retry
					}

					// create the encoder
					eopt := framestream.EncoderOptions{
						ContentType:   FSContentType,
						Bidirectional: true,
						Timeout:       o.timeout,
					}
					enc, err = framestream.NewEncoder(conn, &eopt)
					if err != nil {
						log.Printf("framestream.NewEncoder() failed: %v", err)
						conn.Close()
						enc = nil
						continue // = retry
					}
				}

				// try writing
				if _, err = enc.Write(frame); err != nil {
					log.Printf("framestream.Encoder.Write() failed: %v", err)
					enc.Close()
					enc = nil
					conn.Close()
					continue // = retry
				}

				break // success!
			}

		case <-conn.timer.C:
			conn.SetIdle()
			if enc == nil {
				continue
			}
			if err := enc.Flush(); err != nil {
				log.Printf("framestream.Encoder.Flush() failed: %s", err)
				enc.Close()
				enc = nil
				conn.Close()
				time.Sleep(o.retry)
			}
		}
	}
}

// Close shuts down the FrameStreamSockOutput's output channel and returns
// after all pending data has been flushed and the connection has been closed.
//
// Close satisifes the dnstap Output interface
func (o *FrameStreamSockOutput) Close() {
	close(o.outputChannel)
	<-o.wait
}
