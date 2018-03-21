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
	// write something to the logs so we can test out logging
	fmt.Println("Got request from", r.RemoteAddr)

	fmt.Fprintf(w, "Hello Milpa from %s - %s\n", hostname, getMyIP())
	fmt.Fprintf(w, "Env Vars:\n")
	for _, v := range os.Environ() {
		fmt.Fprintf(w, "    %s\n", v)
	}
}

func main() {
	http.HandleFunc("/", sayHello)
	err := http.ListenAndServe("0.0.0.0:8002", nil)
	if err != nil {
		fmt.Println(err)
	}
}
