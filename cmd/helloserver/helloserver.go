package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
)

func getMyIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		fmt.Println("Error getting ip address")
		return ""
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func sayHello(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	fmt.Fprintf(w, "Hello Milpa from %s - %s", hostname, getMyIP())
}

func main() {
	http.HandleFunc("/", sayHello)
	err := http.ListenAndServe("0.0.0.0:8002", nil)
	if err != nil {
		fmt.Println(err)
	}
}
