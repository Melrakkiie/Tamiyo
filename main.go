package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Poueto!")
	})

	// Écoute sur le port 8080
	http.ListenAndServe(":8080", nil)
}
