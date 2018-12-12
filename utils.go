package main

import (
	"encoding/json"
	"log"
	"net"
	"os/exec"
	"strings"
)

func generateAgentName() string {
	return getOSNameAndVersion()
}

func getOSNameAndVersion() string {
	out, err := exec.Command("lsb_release", "-d").Output()
	if err != nil {
		log.Println(err)
		return "Unknown OS"
	}
	return strings.TrimSpace(strings.Replace(string(out), "Description:", "", -1))
}

func getIPV4() string {
	var addresses = []string{}

	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		addrs, _ := i.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if len(ip.To4()) == net.IPv4len {
				if !ip.IsUnspecified() && !ip.IsLoopback() && !ip.IsMulticast() && !ip.IsInterfaceLocalMulticast() && !ip.IsLinkLocalMulticast() && !ip.IsLinkLocalUnicast() {
					addresses = append(addresses, ip.String())
				}
			}
		}
	}

	jsonStr, _ := json.Marshal(addresses)
	return string(jsonStr)
}

func getIPV6() string {
	var addresses = []string{}

	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		addrs, _ := i.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if len(ip.To4()) != net.IPv4len {
				if !ip.IsUnspecified() && !ip.IsLoopback() {
					addresses = append(addresses, ip.String())
				}
			}
		}
	}

	jsonStr, _ := json.Marshal(addresses)
	return string(jsonStr)
}
