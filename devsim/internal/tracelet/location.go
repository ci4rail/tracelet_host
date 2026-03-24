/*
Copyright © 2024 Ci4Rail GmbH
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tracelet

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"time"

	pb "github.com/ci4rail/io4edge_api/tracelet/go/tracelet"
	lsiclient "github.com/ci4rail/tracelet_host/devsim/pkg/lsi_client"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type location struct {
	uwbValid  bool
	uwbX      float64
	uwbY      float64
	uwbZ      float64
	gnssValid bool
	gnssLat   float64
	gnssLon   float64
	gnssAlt   float64
	gnssFix   int32
}

type ReplayMessages []*pb.TraceletToServer

// publish location to server periodically
func (e *Tracelet) locationClient(locationServerAddress string) error {
	e.lsiClient = lsiclient.NewInstance(locationServerAddress)
	go func() {
		switch e.mode {
		case ModeReplay:
			e.replayLocationMessages()
		default:
			e.publishGeneratedMessages()
		}
	}()
	return nil
}

func IPv4ToUint32(s string) (uint32, error) {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return 0, fmt.Errorf("invalid IPv4 address: %q", s)
	}
	return binary.BigEndian.Uint32(ip), nil
}

func (e *Tracelet) makeTraceletToServerMessage(_ int32) *pb.TraceletToServer {
	u := uuid.New()
	msgID := pb.TraceletMessageID{
		Value: u[:],
	}
	ipAsUint32, err := IPv4ToUint32(e.IPv4Address)
	if err != nil {
		log.Printf("failed to convert IPv4 address to uint32: %v\n", err)
		return nil
	}
	return &pb.TraceletToServer{
		TraceletId:  e.deviceID,
		Ignition:    true,
		DeliveryTs:  timestamppb.Now(),
		Uuid:        &msgID,
		Ipv4Address: ipAsUint32,
	}
}

func (e *Tracelet) makeLocationMessage(m *pb.TraceletToServer_Location) {
	e.locMutex.Lock()
	defer e.locMutex.Unlock()
	m.Gnss.Valid = e.loc.gnssValid
	m.Gnss.Latitude = e.loc.gnssLat
	m.Gnss.Longitude = e.loc.gnssLon
	m.Gnss.Altitude = e.loc.gnssAlt
	m.Gnss.Eph = rand.Float64() * 3
	m.Gnss.Epv = rand.Float64() * 5
	m.Gnss.FixType = e.loc.gnssFix

	m.Uwb.Valid = e.loc.uwbValid
	m.Uwb.X = e.loc.uwbX
	m.Uwb.Y = e.loc.uwbY
	m.Uwb.Z = e.loc.uwbZ
	m.Uwb.Eph = rand.Float64() * 0.2
	m.Uwb.FixType = 1

	// For the simulator, treat GNSS as the fused position.
	m.Fused.Valid = true
	m.Fused.Latitude = e.loc.gnssLat
	m.Fused.Longitude = e.loc.gnssLon
	m.Fused.Altitude = e.loc.gnssAlt
	m.Fused.Eph = m.Gnss.Eph

	m.Speed = rand.Float64() * 10
	m.Temperature = rand.Float64()*10 + 29
}

func (e *Tracelet) locationGenerator() {
	go func() {
		for {
			loc := location{
				uwbValid:  true,
				uwbX:      5.0 + rand.Float64()*10,
				uwbY:      6.21 + rand.Float64()*10,
				uwbZ:      7.5 + rand.Float64()*10,
				gnssValid: true,
				gnssLat:   49.425111 + rand.Float64()*0.0001,
				gnssLon:   11.077378 + rand.Float64()*0.0001,
				gnssAlt:   350.0 + rand.Float64()*10,
				gnssFix:   int32(rand.Intn(6)),
			}
			e.locMutex.Lock()
			e.loc = loc
			e.locMutex.Unlock()

			time.Sleep(1000 * time.Millisecond)
		}
	}()
}

func (e *Tracelet) publishGeneratedMessages() {
	metrics := pb.TraceletMetrics{}
	m := pb.TraceletToServer_Location{
		Gnss:  &pb.TraceletToServer_Location_Gnss{},
		Uwb:   &pb.TraceletToServer_Location_Uwb{},
		Fused: &pb.TraceletToServer_Location_Fused{},
	}

	loopCnt := 0
	for {
		e.makeLocationMessage(&m)
		t2s := e.makeTraceletToServerMessage(0)
		if t2s == nil {
			time.Sleep(time.Second)
			continue
		}
		t2s.Type = &pb.TraceletToServer_Location_{Location: &m}
		if loopCnt%3 == 0 {
			makeMetricsMessage(e.lsiClient, loopCnt, &metrics)
			t2s.Metrics = &metrics
		}
		loopCnt++
		e.pushMessage(t2s)
		time.Sleep(1000 * time.Millisecond)
	}
}

func (e *Tracelet) replayLocationMessages() {
	if len(e.replayMessages) == 0 {
		log.Printf("tracelet replay mode: no messages loaded from %s", e.trackFile)
		return
	}

	for {
		var previous time.Time
		for idx, msg := range e.replayMessages {
			if idx > 0 {
				delay := msg.GetDeliveryTs().AsTime().Sub(previous)
				if delay > 0 {
					time.Sleep(delay)
				}
			}
			e.pushMessage(msg)
			previous = msg.GetDeliveryTs().AsTime()
		}
	}
}

func (e *Tracelet) pushMessage(t2s *pb.TraceletToServer) {
	log.Printf("locationClient WriteMessage: %v\n", t2s)

	payload, err := proto.Marshal(t2s)
	if err != nil {
		log.Printf("locationClient: failed to marshal TraceletToServer message: %v\n", err)
		return
	}
	e.lsiClient.PushMessage(payload)
}

func loadReplayMessages(trackFile string, deviceID string, ipv4Address string) (ReplayMessages, error) {
	data, err := os.ReadFile(trackFile)
	if err != nil {
		return nil, fmt.Errorf("read track file %q: %w", trackFile, err)
	}

	var rawMessages []json.RawMessage
	if err := json.Unmarshal(data, &rawMessages); err != nil {
		return nil, fmt.Errorf("decode track file %q: %w", trackFile, err)
	}
	if len(rawMessages) == 0 {
		return nil, fmt.Errorf("track file %q contains no messages", trackFile)
	}

	messages := make(ReplayMessages, 0, len(rawMessages))
	unmarshalOptions := protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}

	var timeShift time.Duration

	for idx, raw := range rawMessages {
		msg := &pb.TraceletToServer{}
		if err := unmarshalOptions.Unmarshal(raw, msg); err != nil {
			return nil, fmt.Errorf("decode replay message %d from %q: %w", idx, trackFile, err)
		}

		if idx == 0 {
			// use timestamp of first message as reference and shift all messages to start from now
			now := time.Now()
			firstMsgTime := msg.GetDeliveryTs().AsTime()
			timeShift = now.Sub(firstMsgTime)
			log.Printf("Shifting replay messages by %s to align first message with current time\n", timeShift)
		}
		msg.DeliveryTs = timestamppb.New(msg.GetDeliveryTs().AsTime().Add(timeShift))

		if msg.GetLocation() == nil {
			return nil, fmt.Errorf("replay message %d from %q has no location payload", idx, trackFile)
		}
		if deviceID != "" {
			msg.TraceletId = deviceID
		}
		if ipv4Address != "" {
			ipAsUint32, err := IPv4ToUint32(ipv4Address)
			if err != nil {
				return nil, err
			}
			msg.Ipv4Address = ipAsUint32
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// generate some random metrics
func makeMetricsMessage(lsiClient *lsiclient.Instance, loop int, m *pb.TraceletMetrics) {
	m.Health__Type__UwbComm = 1
	m.Health__Type__UwbFirmware = 0
	m.Health__Type__GnssComm = 1
	m.FreeHeapBytes = int64(rand.Intn(1000) + 20000)
	m.WifiRssiDbm = 100.0 - rand.Float64()*50
	m.NtripIsConnected = int64(rand.Intn(2))
	m.SntpUpdates += int64(rand.Intn(2))

	if loop%20 == 0 {
		m.WifiAp = 123
	} else {
		m.WifiAp = 456
	}
	m.GnssNumSats__System__Gps = int64(rand.Intn(10) + 3)
	m.GnssNumSats__System__Glonass = int64(rand.Intn(10) + 3)
	m.GnssNumSats__System__Galileo = int64(rand.Intn(10) + 3)
	m.GnssNumSv = m.GnssNumSats__System__Gps + m.GnssNumSats__System__Glonass + m.GnssNumSats__System__Galileo - 1
	m.GnssPga__Block__Rf1 = int64(rand.Intn(5)) + 40
	m.GnssPga__Block__Rf2 = int64(rand.Intn(5)) + 36
	m.CpuLoadPercent__Cpu___0 = int64(rand.Intn(20) + 10)
	m.CpuLoadPercent__Cpu___1 = int64(rand.Intn(20) + 10)
	if lsiClient.IsConnected {
		m.LsiIsConnected = 1
	} else {
		m.LsiIsConnected = 0
	}
	m.LsiAcksMissed = int64(lsiClient.AcksMissed)
	m.ResetCount__Type__Poweron = 1
}
