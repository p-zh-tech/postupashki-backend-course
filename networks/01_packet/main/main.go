package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

const (
	MaxLen           = 108
	EtherHeaderLen   = 14
	UDPHeaderLen     = 8
	MinTCPHeaderLen  = 20
	MinIPv4HeaderLen = 20
)

var ErrFrameTooShort = errors.New("ethernet frame too short")
var ErrIPHeaderTooShort = errors.New("ip header too short")
var ErrTCPHeaderTooShort = errors.New("tcp header too short")
var ErrUDPHeaderTooShort = errors.New("udp header too short")

type EtherHeader struct {
	DstMAC    net.HardwareAddr
	SrcMAC    net.HardwareAddr
	EtherType uint16
}

func (e *EtherHeader) ReadHeader(b []byte) error {
	if len(b) < EtherHeaderLen {
		return ErrFrameTooShort
	}

	e.DstMAC = net.HardwareAddr(b[0:6])
	e.SrcMAC = net.HardwareAddr(b[6:12])
	e.EtherType = binary.BigEndian.Uint16(b[12:14])

	return nil
}

type IPv4Header struct {
	Version       uint8 //4bits
	IHL           uint8 //4bits
	TOS           uint8
	TotalLength   uint16
	Ident         uint16
	Flags         uint8  //3 bits
	FragOffset    uint16 //13 bits
	TTL           uint8
	Protocol      uint8
	DstIP         net.IP
	SrcIP         net.IP
	CheckSumValid bool
}

func calculateIPv4Checksum(header []byte) uint16 {
	var sum uint32
	for i := 0; i < len(header); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(header[i : i+2]))
	}
	for sum>>16 > 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func (i *IPv4Header) ReadHeader(b []byte) error {
	if len(b) < MinIPv4HeaderLen {
		return ErrIPHeaderTooShort
	}

	start := 0

	i.Version = b[start] >> 4
	i.IHL = (b[start] & 0x0F) * 4

	if len(b) < int(i.IHL) {
		return ErrIPHeaderTooShort
	}

	start++

	i.TOS = b[start]
	start++

	i.TotalLength = binary.BigEndian.Uint16(b[start : start+2])
	start += 2

	i.Ident = binary.BigEndian.Uint16(b[start : start+2])
	start += 2

	flagsAndOffset := binary.BigEndian.Uint16(b[start : start+2])
	i.Flags = uint8(flagsAndOffset >> 13)
	i.FragOffset = (flagsAndOffset & 0x1FFF) * 8
	start += 2

	i.TTL = b[start]
	start++

	i.Protocol = b[start]
	start++

	i.CheckSumValid = calculateIPv4Checksum(b[:i.IHL]) == 0

	start += 2

	i.SrcIP = net.IP(b[start : start+4])
	start += 4

	i.DstIP = net.IP(b[start : start+4])
	start += 4

	return nil
}

type TCPHeader struct {
	SrcPort    uint16
	DstPort    uint16
	SeqNumber  uint32
	AckNumber  uint32
	DataOffset uint8  //4 bits
	Flags      uint16 //9 bits
	WindowSize uint16
	CheckSum   uint16
	UrgentPtr  uint16
}

func (t *TCPHeader) ReadHeader(b []byte) error {
	if len(b) < MinTCPHeaderLen {
		return ErrTCPHeaderTooShort
	}
	start := 0

	t.SrcPort = binary.BigEndian.Uint16(b[start : start+2])
	start += 2

	t.DstPort = binary.BigEndian.Uint16(b[start : start+2])
	start += 2

	t.SeqNumber = binary.BigEndian.Uint32(b[start : start+4])
	start += 4

	t.AckNumber = binary.BigEndian.Uint32(b[start : start+4])
	start += 4

	flagsAndOffset := binary.BigEndian.Uint16(b[start : start+2])
	t.DataOffset = uint8(flagsAndOffset>>12) * 4
	t.Flags = uint16(flagsAndOffset & 0x03F)
	start += 2

	t.WindowSize = binary.BigEndian.Uint16(b[start : start+2])
	start += 2

	t.CheckSum = binary.BigEndian.Uint16(b[start : start+2])
	start += 2

	t.UrgentPtr = binary.BigEndian.Uint16(b[start : start+2])
	start += 2

	return nil
}

type UDPHeader struct {
	SrcPort  uint16
	DstPort  uint16
	Length   uint16
	CheckSum uint16
}

func (u *UDPHeader) ReadHeader(b []byte) error {
	if len(b) < UDPHeaderLen {
		return ErrUDPHeaderTooShort
	}

	start := 0

	u.SrcPort = binary.BigEndian.Uint16(b[start : start+2])
	start += 2

	u.DstPort = binary.BigEndian.Uint16(b[start : start+2])
	start += 2

	u.Length = binary.BigEndian.Uint16(b[start : start+2])
	start += 2

	u.CheckSum = binary.BigEndian.Uint16(b[start : start+2])

	return nil
}

func formatIPFlags(flags uint8) string {
	df := (flags & 0x02) != 0
	mf := (flags & 0x01) != 0

	if df && mf {
		return "DF,MF"
	}
	if df {
		return "DF"
	}
	if mf {
		return "MF"
	}
	return "none"
}

func formatTCPFlags(flags uint16) string {
	var active []string

	if flags&0x001 != 0 {
		active = append(active, "FIN")
	}
	if flags&0x002 != 0 {
		active = append(active, "SYN")
	}
	if flags&0x004 != 0 {
		active = append(active, "RST")
	}
	if flags&0x008 != 0 {
		active = append(active, "PSH")
	}
	if flags&0x010 != 0 {
		active = append(active, "ACK")
	}
	if flags&0x020 != 0 {
		active = append(active, "URG")
	}

	if len(active) == 0 {
		return "none"
	}
	return strings.Join(active, ",")
}

func ReadBytes(r io.Reader) ([]byte, error) {
	reader := bufio.NewReader(os.Stdin)
	buffer := make([]byte, 0, MaxLen)

	for {
		b, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		if b == ' ' || b == '\n' || b == '\t' || b == '\r' {
			continue
		}
		buffer = append(buffer, b)
	}

	bytes := make([]byte, hex.DecodedLen(len(buffer)))

	_, err := hex.Decode(bytes, buffer)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func main() {
	bytes, err := ReadBytes(os.Stdin)
	if err != nil {
		panic(err)
	}

	var e EtherHeader
	if err := e.ReadHeader(bytes); err != nil {
		panic(err)
	}

	fmt.Printf("eth.dst %s\n", e.DstMAC)
	fmt.Printf("eth.src %s\n", e.SrcMAC)
	fmt.Printf("eth.ethertype 0x%04x\n", e.EtherType)

	if e.EtherType != 0x0800 {
		return
	}

	ipData := bytes[EtherHeaderLen:]
	var ip IPv4Header
	if err := ip.ReadHeader(ipData); err != nil {
		panic(err)
	}

	if int(ip.TotalLength) < len(ipData) {
		ipData = ipData[:ip.TotalLength]
	}

	fmt.Printf("ip.version %d\n", ip.Version)
	fmt.Printf("ip.ihl_bytes %d\n", ip.IHL)
	fmt.Printf("ip.total_length %d\n", ip.TotalLength)
	fmt.Printf("ip.id 0x%04x\n", ip.Ident)
	fmt.Printf("ip.flags %s\n", formatIPFlags(ip.Flags))
	fmt.Printf("ip.frag_offset %d\n", ip.FragOffset)
	fmt.Printf("ip.ttl %d\n", ip.TTL)
	fmt.Printf("ip.protocol %d\n", ip.Protocol)
	fmt.Printf("ip.src %s\n", ip.SrcIP)
	fmt.Printf("ip.dst %s\n", ip.DstIP)
	fmt.Printf("ip.checksum_valid %t\n", ip.CheckSumValid)

	transportData := ipData[ip.IHL:]
	switch ip.Protocol {
	case 6:
		var tcp TCPHeader
		if err := tcp.ReadHeader(transportData); err != nil {
			return
		}

		fmt.Printf("tcp.src_port %d\n", tcp.SrcPort)
		fmt.Printf("tcp.dst_port %d\n", tcp.DstPort)
		fmt.Printf("tcp.seq %d\n", tcp.SeqNumber)
		fmt.Printf("tcp.ack %d\n", tcp.AckNumber)
		fmt.Printf("tcp.data_offset_bytes %d\n", tcp.DataOffset)
		fmt.Printf("tcp.flags %s\n", formatTCPFlags(tcp.Flags))
		fmt.Printf("tcp.window %d\n", tcp.WindowSize)

		payloadLen := len(transportData) - int(tcp.DataOffset)
		if payloadLen < 0 {
			payloadLen = 0
		}
		fmt.Printf("payload.length %d\n", payloadLen)

	case 17:
		var udp UDPHeader
		if err := udp.ReadHeader(transportData); err != nil {
			return
		}

		fmt.Printf("udp.src_port %d\n", udp.SrcPort)
		fmt.Printf("udp.dst_port %d\n", udp.DstPort)
		fmt.Printf("udp.length %d\n", udp.Length)

		payloadLen := len(transportData) - UDPHeaderLen
		if payloadLen < 0 {
			payloadLen = 0
		}
		fmt.Printf("payload.length %d\n", payloadLen)

	default:
		fmt.Printf("payload.length %d\n", len(transportData))
	}
}
