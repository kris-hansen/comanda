package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/example/sample-api/pkg/models"
)

// GetUsers returns all users
func GetUsers(w http.ResponseWriter, r *http.Request) {
	users := []models.User{
		{ID: 1, Name: "Alice", Email: "alice@example.com"},
		{ID: 2, Name: "Bob", Email: "bob@example.com"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// CreateUser creates a new user
func CreateUser(w http.ResponseWriter, r *http.Request) {
	var user models.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate user
	if err := user.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}
