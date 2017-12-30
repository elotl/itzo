package main

import (
	"fmt"
	"net/http"
)

func sayHello(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello Milpa!")
}

func main() {
	http.HandleFunc("/", sayHello)
	err := http.ListenAndServe("0.0.0.0:8002", nil)
	if err != nil {
		fmt.Println(err)
	}
}
