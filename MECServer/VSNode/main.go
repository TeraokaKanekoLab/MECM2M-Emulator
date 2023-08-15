package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello from %s!", r.Host)
}

func startServer(port int) {
	mux := http.NewServeMux()                          // 新しいServeMuxインスタンスを作成
	mux.HandleFunc("/primapi/data/past/node", handler) // そのインスタンスにハンドラを登録

	address := fmt.Sprintf(":%d", port)
	log.Printf("Starting server on %s", address)

	server := &http.Server{
		Addr:    address,
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Error starting server on port %d: %v", port, err)
	}
}

func main() {
	var wg sync.WaitGroup

	ports := []int{8080, 8081}

	for _, port := range ports {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			startServer(port)
		}(port)
	}

	wg.Wait()
}
