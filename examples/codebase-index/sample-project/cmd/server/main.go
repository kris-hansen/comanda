package main

import (
	"fmt"
	"net/http"

	"github.com/example/sample-api/pkg/handlers"
)

func main() {
	fmt.Println("Starting server on :8080")

	http.HandleFunc("/users", handlers.GetUsers)
	http.HandleFunc("/users/create", handlers.CreateUser)

	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}
