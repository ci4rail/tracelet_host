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
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	"github.com/ci4rail/io4edge-client-go/client"
	pb "github.com/ci4rail/io4edge_api/tracelet/go/tracelet"
	"github.com/google/uuid"
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

// publish location to server periodically
func (e *Tracelet) locationClient(locationServerAddress string) error {
	metrics := pb.TraceletMetrics{}
	m := pb.TraceletToServer_Location{
		Gnss:  &pb.TraceletToServer_Location_Gnss{},
		Uwb:   &pb.TraceletToServer_Location_Uwb{},
		Fused: &pb.TraceletToServer_Location_Fused{},
	}
	go func() {
		loopCnt := 0
		for {
			log.Printf("try to connect to %v\n", locationServerAddress)
			ch, err := channelFromSocketAddress(locationServerAddress)

			if err == nil {
				defer ch.Close()
				for {
					e.makeLocationMessage(&m)
					t2s := e.makeTraceletToServerMessage(0)
					t2s.Type = &pb.TraceletToServer_Location_{Location: &m}
					if loopCnt%3 == 0 {
						makeMetricsMessage(loopCnt, &metrics)
						t2s.Metrics = &metrics
					}
					loopCnt++

					fmt.Printf("locationClient WriteMessage: %v\n", t2s)

					err := ch.WriteMessage(t2s)
					if err != nil {
						log.Printf("locationClient WriteMessage failed, %v\n", err)
						break
					}
					time.Sleep(1000 * time.Millisecond)
				}
			}
			time.Sleep(1000 * time.Millisecond)
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
	uuid := uuid.New()
	msgID := pb.TraceletMessageID{
		Value: uuid[:],
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
				uwbValid:  false,
				uwbX:      5.0,
				uwbY:      6.21,
				uwbZ:      7.5,
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

// generate some random metrics
func makeMetricsMessage(loop int, m *pb.TraceletMetrics) {
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
	m.LsiIsConnected = int64(rand.Intn(2))
	m.ResetCount__Type__Poweron = 1
}

func channelFromSocketAddress(address string) (*client.Channel, error) {
	c, err := client.NewUDPClientFromSocketAddress(address)
	if err != nil {
		return nil, errors.New("can't create UDP client: " + err.Error())
	}

	return c.Ch, nil
}
