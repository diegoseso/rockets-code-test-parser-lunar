package main

// write a simple http proxy server that prints the request method and the payload to the console
import (
	"fmt"
	"io"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("Received %s request\n", r.Method)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Error reading body: %v", err)
			http.Error(w, "Can't read body", http.StatusBadRequest)
			return
		}
		fmt.Printf("Payload: %s\n", string(body))
		w.WriteHeader(http.StatusOK)
	})

	log.Println("Starting proxy server on :8080")
	if err := http.ListenAndServe(":8088", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
