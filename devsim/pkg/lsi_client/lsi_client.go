package lsiclient

import (
	"encoding/binary"
	"log"
	"net"
	"time"
)

type Instance struct {
	q                *PosQueue
	locSrvAddr       string
	IsConnected      bool
	AcksMissed       int
	networkAvailable bool
}

func NewInstance(locSrvAddr string) *Instance {
	i := &Instance{
		q:                NewPosQueue(10, 1000),
		locSrvAddr:       locSrvAddr,
		IsConnected:      false,
		AcksMissed:       0,
		networkAvailable: true,
	}
	go i.clientLoop()
	return i
}

func (i *Instance) PushMessage(msg []byte) {
	i.q.PushOverwrite(msg, true)
}

func (i *Instance) SetNetworkAvailable(available bool) {
	i.networkAvailable = available
}

func openConnection(srvAddr string) (net.Conn, error) {
	raddr, err := net.ResolveUDPAddr("udp", srvAddr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func waitForAck(conn net.Conn, seq uint32) error {
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	defer conn.SetReadDeadline(time.Time{})
	for {
		// wait for ack but stop on timeout
		ack := make([]byte, 4)
		n, err := conn.Read(ack)
		if err != nil {
			// delay if error is not a timeout
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return err
			}
			time.Sleep(200 * time.Millisecond)
			log.Printf("Failed to read ack: %v", err)
			return err
		}
		if n < 4 {
			log.Printf("Ack message too short: %d bytes", n)
			return err
		}

		if binary.LittleEndian.Uint32(ack) == seq {
			break
		} else {
			log.Printf("Ignoring old ack #%d, expected %d", binary.LittleEndian.Uint32(ack), seq)
		}
	}
	return nil
}

func (i *Instance) deliverMessage(conn net.Conn, seq uint32, msg []byte) {
	if !i.networkAvailable {
		i.IsConnected = false
		return
	}

	// prepend sequence number to message
	buf := make([]byte, 4+len(msg))
	binary.LittleEndian.PutUint32(buf[0:4], seq)
	copy(buf[4:], msg)

	for {
		_, err := conn.Write(buf)
		if err == nil {
			if !i.networkAvailable {
				i.IsConnected = false
				break
			}

			if waitForAck(conn, seq) == nil {
				i.IsConnected = true
				log.Printf("Delivered message seq %d", seq)
				break
			} else {
				i.AcksMissed++
				log.Printf("Ack wait timeout for seq %d (%d missed)", seq, i.AcksMissed)
			}
		} else {
			log.Printf("Failed to send message: %v", err)
			i.IsConnected = false
			time.Sleep(500 * time.Millisecond)
		}
		if i.q.IsFull() {
			log.Printf("pos_queue full, don't wait for ACK for seq %d", seq)
			i.IsConnected = false
			break
		}
	}
}

func (i *Instance) clientLoop() {
	seq := uint32(0)
	var conn net.Conn
	var err error
	for {
		if conn == nil {
			conn, err = openConnection(i.locSrvAddr)
			if err != nil {
				log.Printf("Failed to open connection: %v", err)
				time.Sleep(time.Second)
				continue
			}
		}
		qItem, _, err := i.q.Pop(5 * time.Second)
		if err != nil {
			log.Printf("Failed to pop item from queue: %v", err)
			time.Sleep(time.Second)
			continue
		}
		seq++
		i.deliverMessage(conn, seq, qItem)
		if !i.IsConnected {
			conn.Close()
			conn = nil
			time.Sleep(time.Second)
		}
	}
}
