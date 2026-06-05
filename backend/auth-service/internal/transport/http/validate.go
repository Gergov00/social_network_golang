package http

import (
	"errors"
	"net/mail"
	"strings"
)

func normalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", errors.New("Invalid email address")
	}
	return email, nil
}

func (req *registerRequest) validate() error {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return err
	}
	req.Email = email

	if len(req.Password) < 8 {
		return errors.New("Password must be at least 8 characters long")
	}

	if len(req.Password) > 72 {
		return errors.New("Password must be at most 72 characters long")
	}

	return nil
}

func (req *loginRequest) validate() error {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return err
	}
	req.Email = email

	if req.Password == "" {
		return errors.New("Password is empty")
	}

	return nil
}
