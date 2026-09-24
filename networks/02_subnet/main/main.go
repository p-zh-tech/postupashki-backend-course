package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
)

const UsageSubnet string = "usage: run.sh subnet <адрес>/<префикс>"
const UsageRoute string = "usage: run.sh route <файл таблицы> <адрес назначения>"
const PrefixErr string = "prefix should be <= 32"

type Subnet struct {
	Network   uint32
	Broadcast string
	Netmask   uint32
	Prefix    int
	First     uint32
	Last      uint32
	Hosts     int64
}

func MakeSubnet(ip uint32, prefix int) Subnet {
	var s Subnet
	s.Prefix = prefix

	if s.Prefix == 0 {
		s.Netmask = 0
	} else {
		s.Netmask = uint32(0xFFFFFFFF) << uint32(32-s.Prefix)
	}

	s.Network = ip & s.Netmask

	switch s.Prefix {
	case 31:
		s.First = s.Network
		s.Last = s.Network + 1
		s.Broadcast = "none"

	case 32:
		s.First = s.Network
		s.Last = s.Network
		s.Broadcast = "none"
	default:
		broadcast := s.Network | ^s.Netmask
		s.First = s.Network + 1
		s.Last = broadcast - 1
		s.Broadcast = ParseIPToString(broadcast)
	}
	s.Hosts = int64(s.Last - s.First + 1)
	return s
}

func ParseIpToInt(ip string) uint32 {
	var res, oct uint32
	for i := 0; i < len(ip); i++ {
		c := ip[i]
		if c == '.' {
			res = (res << 8) | (oct & 0xFF)
			oct = 0
		} else {
			oct = oct*10 + uint32(c-'0')
		}
	}
	return (res << 8) | (oct & 0xFF)
}

func ParseIPToString(ipInt uint32) string {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, ipInt)
	return ip.String()
}

type NamedNet struct {
	Name   string
	IP     uint32
	Prefix int
}

func FindBestMatch(networks []NamedNet, ourIP uint32) *NamedNet {
	var bestNet *NamedNet
	for i, net := range networks {
		netmask := uint32(0xFFFFFFFF) << uint32(32-net.Prefix)
		network := net.IP & netmask
		ourIPMasked := ourIP & netmask
		if network == ourIPMasked {
			if bestNet == nil || bestNet.Prefix < net.Prefix {
				bestNet = &networks[i]
			}
		}
	}
	return bestNet
}

func main() {
	args := os.Args
	if len(args) == 0 {
		log.Fatal("Недостаточно аргументов")
	}
	mode := args[1]
	switch mode {
	case "subnet":
		if len(args) < 3 {
			log.Fatal(UsageSubnet)
		}
		rawIP := args[2]
		slashIdx := strings.IndexByte(rawIP, '/')
		if slashIdx == -1 {
			log.Fatal(UsageSubnet)
		}

		ip := ParseIpToInt(rawIP[:slashIdx])
		prefix, err := strconv.Atoi(rawIP[slashIdx+1:])
		if err != nil {
			log.Fatal(err)
		}
		if prefix > 32 {
			log.Fatal(PrefixErr)
		}
		s := MakeSubnet(ip, prefix)

		fmt.Println("network", ParseIPToString(s.Network))
		fmt.Println("broadcast", s.Broadcast)
		fmt.Println("netmask", ParseIPToString(s.Netmask))
		fmt.Println("prefix", s.Prefix)
		fmt.Println("first", ParseIPToString(s.First))
		fmt.Println("last", ParseIPToString(s.Last))
		fmt.Println("hosts", s.Hosts)
	case "route":
		if len(args) < 4 {
			log.Fatal(UsageRoute)
		}

		filepath := args[2]
		ourIP := ParseIpToInt(args[3])

		file, err := os.Open(filepath)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		var networks []NamedNet
		for scanner.Scan() {
			line := scanner.Text()
			if len(line) == 0 {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}

			rawIp := parts[0]
			name := parts[1]

			slashIdx := strings.IndexByte(rawIp, '/')
			if slashIdx == -1 {
				continue
			}
			ip := ParseIpToInt(rawIp[:slashIdx])
			prefix, err := strconv.Atoi(rawIp[slashIdx+1:])
			if err != nil {
				log.Fatal(err)
			}
			if prefix > 32 {
				log.Fatal(PrefixErr)
			}
			networks = append(networks, NamedNet{Name: name, IP: ip, Prefix: prefix})
		}

		netPtr := FindBestMatch(networks, ourIP)
		if netPtr != nil {
			fmt.Println("via", netPtr.Name)
			fmt.Println("prefix", netPtr.Prefix)
		} else {
			fmt.Println("unreachable true")
			os.Exit(1)
		}
	}
}
