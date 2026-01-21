package models

import "errors"

// User represents a user in the system
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Validate checks if the user data is valid
func (u *User) Validate() error {
	if u.Name == "" {
		return errors.New("name is required")
	}
	if u.Email == "" {
		return errors.New("email is required")
	}
	return nil
}

// IsAdmin checks if the user has admin privileges
func (u *User) IsAdmin() bool {
	return u.Email == "admin@example.com"
}
