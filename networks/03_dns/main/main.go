package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"
)

var Usage string = "run.sh <адрес сервера> <порт>"

const (
	TypeA     uint16 = 1
	TypeNS    uint16 = 2
	TypeCNAME uint16 = 5
	TypeMX    uint16 = 15
	TypeTXT   uint16 = 16
	TypeAAAA  uint16 = 28
)

var StringToType = map[string]uint16{
	"A":     TypeA,
	"NS":    TypeNS,
	"CNAME": TypeCNAME,
	"MX":    TypeMX,
	"TXT":   TypeTXT,
	"AAAA":  TypeAAAA,
}

var TypeToString = map[uint16]string{
	TypeA:     "A",
	TypeNS:    "NS",
	TypeCNAME: "CNAME",
	TypeMX:    "MX",
	TypeTXT:   "TXT",
	TypeAAAA:  "AAAA",
}

type Query struct {
	Name string
	Type string
}

type CacheEntry struct {
	Resp      string
	ExpiresAt time.Time
}

type DNSHeader struct {
	ID      uint16
	Flags   uint16
	QDCount uint16
	ANCount uint16
	NSCount uint16
	ARCount uint16
}

func (h *DNSHeader) RCode() string {
	rcode := byte(h.Flags & 0x000F)
	switch rcode {
	case 0:
		return "NOERROR"
	case 1:
		return "FORMERR"
	case 2:
		return "SERVFAIL"
	case 3:
		return "NXDOMAIN"
	case 5:
		return "REFUSED"
	default:
		return fmt.Sprintf("RCODE%d", rcode)
	}
}

type DNSQuestion struct {
	Name  string
	Type  uint16
	Class uint16
}

type DNSRecord struct {
	Name     string
	Type     uint16
	Class    uint16
	TTL      uint32
	RDLength uint16
	Value    string
}

type DNSPacket struct {
	Header    DNSHeader
	Questions []DNSQuestion
	Answers   []DNSRecord
}

func MakeDNSRequest(query *Query) ([]byte, uint16) {
	header := DNSHeader{
		ID:      uint16(rand.Intn(65535)),
		Flags:   0x0100,
		QDCount: 1,
		ANCount: 0,
		NSCount: 0,
		ARCount: 0,
	}

	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, header)

	for _, label := range strings.Split(query.Name, ".") {
		if len(label) > 0 {
			buf.WriteByte(byte(len(label)))
			buf.WriteString(label)
		}
	}
	buf.WriteByte(0x00)

	qType := StringToType[query.Type]
	_ = binary.Write(buf, binary.BigEndian, qType)
	_ = binary.Write(buf, binary.BigEndian, uint16(1))

	return buf.Bytes(), header.ID
}

func ParseName(msg []byte, offset int) (string, int, error) {
	var labels []string
	jumped := false
	bytesRead := 0
	initialOffset := offset
	jumpsCount := 0
	const maxJumps = 5

	for {
		if offset >= len(msg) {
			return "", 0, fmt.Errorf("out of bounds")
		}

		b := msg[offset]

		if b&0xC0 == 0xC0 {
			if offset+1 >= len(msg) {
				return "", 0, fmt.Errorf("out of bounds pointer")
			}

			if !jumped {
				bytesRead = (offset + 2) - initialOffset
				jumped = true
			}

			pointer := int(binary.BigEndian.Uint16(msg[offset:offset+2]) & 0x3FFF)
			offset = pointer

			jumpsCount++
			if jumpsCount > maxJumps {
				return "", 0, fmt.Errorf("compression loop detected")
			}
			continue
		}

		if b == 0 {
			if !jumped {
				bytesRead = (offset + 1) - initialOffset
			}
			break
		}

		labelLen := int(b)
		offset++

		if offset+labelLen > len(msg) {
			return "", 0, fmt.Errorf("label out of bounds")
		}

		labels = append(labels, string(msg[offset:offset+labelLen]))
		offset += labelLen
	}

	name := strings.Join(labels, ".")
	if len(name) > 0 {
		name += "."
	}

	return name, bytesRead, nil
}

func ParseDNSPacket(msg []byte) (*DNSPacket, error) {
	if len(msg) < 12 {
		return nil, fmt.Errorf("packet too short")
	}

	packet := &DNSPacket{}

	packet.Header = DNSHeader{
		ID:      binary.BigEndian.Uint16(msg[0:2]),
		Flags:   binary.BigEndian.Uint16(msg[2:4]),
		QDCount: binary.BigEndian.Uint16(msg[4:6]),
		ANCount: binary.BigEndian.Uint16(msg[6:8]),
		NSCount: binary.BigEndian.Uint16(msg[8:10]),
		ARCount: binary.BigEndian.Uint16(msg[10:12]),
	}

	offset := 12

	for i := 0; i < int(packet.Header.QDCount); i++ {
		name, readLen, err := ParseName(msg, offset)
		if err != nil {
			return nil, err
		}
		offset += readLen

		q := DNSQuestion{
			Name:  name,
			Type:  binary.BigEndian.Uint16(msg[offset : offset+2]),
			Class: binary.BigEndian.Uint16(msg[offset+2 : offset+4]),
		}
		packet.Questions = append(packet.Questions, q)
		offset += 4
	}

	for i := 0; i < int(packet.Header.ANCount); i++ {
		if offset >= len(msg) {
			break
		}

		name, nameLen, err := ParseName(msg, offset)
		if err != nil {
			return nil, err
		}
		offset += nameLen

		rec := DNSRecord{
			Name:     name,
			Type:     binary.BigEndian.Uint16(msg[offset : offset+2]),
			Class:    binary.BigEndian.Uint16(msg[offset+2 : offset+4]),
			TTL:      binary.BigEndian.Uint32(msg[offset+4 : offset+8]),
			RDLength: binary.BigEndian.Uint16(msg[offset+8 : offset+10]),
		}
		offset += 10

		rdataStart := offset
		offset += int(rec.RDLength)

		switch rec.Type {
		case TypeA:
			if rec.RDLength == 4 {
				rec.Value = net.IP(msg[rdataStart : rdataStart+4]).String()
			}
		case TypeAAAA:
			if rec.RDLength == 16 {
				rec.Value = net.IP(msg[rdataStart : rdataStart+16]).String()
			}
		case TypeCNAME, TypeNS:
			rec.Value, _, _ = ParseName(msg, rdataStart)
		case TypeMX:
			pref := binary.BigEndian.Uint16(msg[rdataStart : rdataStart+2])
			mxName, _, _ := ParseName(msg, rdataStart+2)
			rec.Value = fmt.Sprintf("%d %s", pref, mxName)
		case TypeTXT:
			var sb strings.Builder
			curr := rdataStart
			for curr < rdataStart+int(rec.RDLength) {
				sLen := int(msg[curr])
				curr++
				sb.Write(msg[curr : curr+sLen])
				curr += sLen
			}
			rec.Value = sb.String()
		}

		packet.Answers = append(packet.Answers, rec)
	}

	return packet, nil
}

func FormatResponse(query Query, packet *DNSPacket) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "status %s\n", packet.Header.RCode())

	for _, ans := range packet.Answers {
		if typeName, exists := TypeToString[ans.Type]; exists && ans.Value != "" {
			fmt.Fprintf(&sb, "answer %s %s %d\n", typeName, ans.Value, ans.TTL)
		}
	}
	return sb.String()
}

func main() {
	if len(os.Args) != 3 {
		log.Fatal(Usage)
	}
	serverIP := os.Args[1]
	port := os.Args[2]

	serverAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(serverIP, port))
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	scanner := bufio.NewScanner(os.Stdin)
	cache := make(map[Query]CacheEntry)

	for scanner.Scan() {
		line := scanner.Text()
		cmd := strings.Fields(line)
		if len(cmd) < 2 {
			continue
		}

		query := Query{
			Name: cmd[0],
			Type: cmd[1],
		}

		normalizedQuery := Query{
			Name: strings.ToLower(query.Name),
			Type: query.Type,
		}

		fmt.Println("query", query.Name, query.Type)

		if entry, ok := cache[normalizedQuery]; ok {
			if time.Now().Before(entry.ExpiresAt) {
				fmt.Print(entry.Resp)
				fmt.Print("end\n")
				continue
			}
			delete(cache, normalizedQuery)
		}

		queryPacket, queryID := MakeDNSRequest(&query)

		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

		_, err = conn.Write(queryPacket)
		if err != nil {
			log.Fatal(err)
		}

		responseBuffer := make([]byte, 4096)

		for {
			n, err := conn.Read(responseBuffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					fmt.Println("status TIMEOUT")
					fmt.Println("end")
					os.Exit(1)
				}
				log.Fatal(err)
			}

			if n < 12 {
				continue
			}

			responseID := binary.BigEndian.Uint16(responseBuffer[0:2])
			if responseID != queryID {
				continue
			}

			packet, err := ParseDNSPacket(responseBuffer[:n])
			if err != nil {
				log.Fatal(err)
			}

			resp := FormatResponse(query, packet)

			if packet.Header.RCode() == "NOERROR" && len(packet.Answers) > 0 {
				minTTL := uint32(0)
				if len(packet.Answers) > 0 {
					minTTL = packet.Answers[0].TTL
					for _, ans := range packet.Answers[1:] {
						if ans.TTL < minTTL {
							minTTL = ans.TTL
						}
					}
				}
				cache[normalizedQuery] = CacheEntry{Resp: resp, ExpiresAt: time.Now().Add(time.Duration(minTTL) * time.Second)}
			}

			fmt.Print(resp)
			break
		}
		fmt.Print("end\n")
	}
}
