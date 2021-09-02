/*
 * Copyright (c) 2013-2014 by Farsight Security, Inc.
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
	"os"
	"time"
)

// A FrameStreamSockInput collects dnstap data from one or more clients of
// a listening socket.
type FrameStreamSockInput struct {
	wait     chan bool
	listener net.Listener
	timeout  time.Duration
}

// NewFrameStreamSockInput creates a FrameStreamSockInput collecting dnstap
// data from clients which connect to the given listener.
func NewFrameStreamSockInput(listener net.Listener) (input *FrameStreamSockInput) {
	input = new(FrameStreamSockInput)
	input.listener = listener
	return
}

// SetTimeout sets the timeout for reading the initial handshake and writing
// response control messages to clients of the FrameStreamSockInput's listener.
//
// The timeout is effective only for connections accepted after the call to
// FrameStreamSockInput.
func (input *FrameStreamSockInput) SetTimeout(timeout time.Duration) {
	input.timeout = timeout
}

// NewFrameStreamSockInputFromPath creates a unix domain socket at the
// given socketPath and returns a FrameStreamSockInput collecting dnstap
// data from clients connecting to this socket.
//
// If a socket or other file already exists at socketPath,
// NewFrameStreamSockInputFromPath removes it before creating the socket.
func NewFrameStreamSockInputFromPath(socketPath string) (input *FrameStreamSockInput, err error) {
	os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return
	}
	return NewFrameStreamSockInput(listener), nil
}

// ReadInto accepts connections to the FrameStreamSockInput's listening
// socket and sends all dnstap data read from these connections to the
// output channel.
//
// ReadInto satisfies the dnstap Input interface.
func (input *FrameStreamSockInput) ReadInto(output chan []byte) {
	for {
		conn, err := input.listener.Accept()
		if err != nil {
			log.Printf("net.Listener.Accept() failed: %s\n", err)
			continue
		}
		i, err := NewFrameStreamInputTimeout(conn, true, input.timeout)
		if err != nil {
			log.Printf("dnstap.NewFrameStreamInput() failed: %s\n", err)
			continue
		}
		log.Printf("dnstap.FrameStreamSockInput: accepted a socket connection\n")
		go i.ReadInto(output)
	}
}

// Wait satisfies the dnstap Input interface.
//
// The FrameSTreamSocketInput Wait method never returns, because the
// corresponding Readinto method also never returns.
func (input *FrameStreamSockInput) Wait() {
	select {}
}
