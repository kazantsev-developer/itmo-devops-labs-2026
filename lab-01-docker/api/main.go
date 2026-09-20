package main

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"
)

var memoryStorage [][]byte

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok\n"))
	})

	http.HandleFunc("/eat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		mbStr := r.URL.Query().Get("mb")
		mb, err := strconv.Atoi(mbStr)
		if err != nil || mb <= 0 {
			http.Error(w, "Parameter 'mb' must be a positive integer", http.StatusBadRequest)
			return
		}

		for i := range mb {
			_ = i
			chunk := make([]byte, 1024*1024)

			chunk[0] = 1
			for j := 1; j < len(chunk); j *= 2 {
				copy(chunk[j:], chunk[:j])
			}

			memoryStorage = append(memoryStorage, chunk)
		}

		runtime.GC()

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Allocated and holding %d MB\n", mb)
	})

	http.HandleFunc("/burn", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("CPU burn started\n"))

		go func() {
			for {
				// Spinning
			}
		}()
	})

	fmt.Println("Server is running on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}
