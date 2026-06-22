package events

type UserRegistered struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}
