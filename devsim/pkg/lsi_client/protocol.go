package lsiclient

import (
	"encoding/binary"
	"errors"
	"time"
	"github.com/ci4rail/io4edge-client-go/v2/pkg/transport"

	log "github.com/sirupsen/logrus"
)

// FrameHandshake represents a stream with message semantics
type FrameHandshake struct {
	trans   transport.Transport
	recvSeq recvSeqNum
	sendSeq uint32
}

type recvSeqNum struct {
	lastSeq      uint32
	lastSeqValid bool
}

// NewFrameHandshakeFromTransport creates a message stream from transport t
func NewFrameHandshakeFromTransport(t transport.Transport) *FrameHandshake {
	return &FrameHandshake{
		trans: t,
		recvSeq: recvSeqNum{
			lastSeq:      0,
			lastSeqValid: false,
		},
		sendSeq: 0,
	}
}

func (fh *FrameHandshake) receiveAck() error {
	fh.trans.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	defer fh.trans.SetReadDeadline(time.Time{})
	for {
		// wait for ack but stop on timeout
		ack := make([]byte, 4)
		n, err := fh.trans.Read(ack)
		if err != nil {
			return err
		}
		if n < 4 {
			return errors.New("FrameHandshake receiveAck: Ack message too short")
		}

		if binary.LittleEndian.Uint32(ack) == fh.sendSeq {
			break
		} else {
			log.Debugf("Ignoring old ack #%d, expected %d", binary.LittleEndian.Uint32(ack), fh.sendSeq)
		}
	}

	return nil
}

// WriteMsg writes io4edge standard message to the transport stream
func (fh *FrameHandshake) WriteMsg(payload []byte) error {
	// send message via transport
	fh.sendSeq++
	msg := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(msg, fh.sendSeq)
	copy(msg[4:], payload)
	_, err := fh.trans.Write(msg)
	if err != nil {
		return err
	}

	// wait for ack but stop on timeout
	err = fh.receiveAck()

	return err
}
