package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/examples/util"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"
)

var sourceIp *string = new(string)
var sourcePort *string = new(string)
var destination *string = new(string)

type httpStreamFactory struct{}

type httpStream struct {
	net, transport gopacket.Flow
	r              tcpreader.ReaderStream
}

func (h *httpStreamFactory) New(net, transport gopacket.Flow) tcpassembly.Stream {
	hstream := &httpStream{
		net:       net,
		transport: transport,
		r:         tcpreader.NewReaderStream(),
	}
	go hstream.run()

	return &hstream.r
}

func (h *httpStream) run() {
	buf := bufio.NewReader(&h.r)
	for {
		req, err := http.ReadRequest(buf)
		if err == io.EOF {
			return
		} else if err != nil {
			log.Println("Error reading stream", h.net, h.transport, ":", err)
		} else {
			reqSourceIP := h.net.Src().String()
			reqDestionationPort := h.transport.Dst().String()
			body, bErr := io.ReadAll(req.Body)
			if bErr != nil {
				return
			}
			req.Body.Close()
			go forwardRequest(req, reqSourceIP, reqDestionationPort, body)
		}
	}
}

func forwardRequest(req *http.Request, reqSourceIP string, reqDestionationPort string, body []byte) {
	url := fmt.Sprintf("http://%s%s", string(*destination), req.RequestURI)
	forwardReq, err := http.NewRequest(req.Method, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("Error forwarding request %v\n", err)
		return
	}

	for header, values := range req.Header {
		for _, value := range values {
			forwardReq.Header.Add(header, value)
		}
	}

	forwardReq.Header.Add("X-Forwarded-For", reqSourceIP)
	if forwardReq.Header.Get("X-Forwarded-Port") == "" {
		forwardReq.Header.Set("X-Forwarded-Port", reqDestionationPort)
	}
	if forwardReq.Header.Get("X-Forwarded-Proto") == "" {
		forwardReq.Header.Set("X-Forwarded-Proto", "http")
	}
	if forwardReq.Header.Get("X-Forwarded-Host") == "" {
		forwardReq.Header.Set("X-Forwarded-Host", req.Host)
	}

	httpClient := &http.Client{}
	resp, rErr := httpClient.Do(forwardReq)
	if rErr != nil {
		log.Println("Forward request error", ":", err)
		return
	}

	defer resp.Body.Close()
}

func main() {
	*sourceIp = os.Getenv("SOURCE_IP")
	*sourcePort = os.Getenv("SOURCE_PORT")
	destinationIP := os.Getenv("DESTINATION_IP")
	destinationPort := os.Getenv("DESTINATION_PORT")
	*destination = destinationIP + ":" + destinationPort

	defer util.Run()()
	var handle *pcap.Handle
	var err error

	log.Printf("Starting capture on interface vxlan0")
	handle, err = pcap.OpenLive("any", 8951, true, pcap.BlockForever)
	if err != nil {
		log.Fatal(err)
	}

	if err := handle.SetBPFFilter("host " + *sourceIp + " and tcp and port " + *sourcePort); err != nil {
		log.Fatal(err)
	}

	streamFactory := &httpStreamFactory{}
	streamPool := tcpassembly.NewStreamPool(streamFactory)
	assembler := tcpassembly.NewAssembler(streamPool)

	log.Println("Reading in packets")
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packets := packetSource.Packets()
	ticker := time.Tick(time.Minute)

	for {
		select {
		case packet := <-packets:
			if packet == nil {
				return
			}
			if packet.NetworkLayer() == nil || packet.TransportLayer() == nil || packet.TransportLayer().LayerType() != layers.LayerTypeTCP {
				log.Println("Unusable packet")
				continue
			}
			tcp := packet.TransportLayer().(*layers.TCP)
			assembler.AssembleWithTimestamp(packet.NetworkLayer().NetworkFlow(), tcp, packet.Metadata().Timestamp)

		case <-ticker:
			assembler.FlushOlderThan(time.Now().Add(time.Minute * -1))
		}
	}
}
