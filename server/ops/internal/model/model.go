package model

import "time"

type User struct {
	ID           string    `json:"id" bson:"_id"`
	Username     string    `json:"username" bson:"username"`
	PasswordHash string    `json:"-" bson:"password_hash"`
	Role         string    `json:"role" bson:"role"`
	Disabled     bool      `json:"disabled" bson:"disabled"`
	CreatedAt    time.Time `json:"created_at" bson:"created_at"`
}

type RefreshToken struct {
	ID        string    `json:"id" bson:"_id"`
	UserID    string    `json:"user_id" bson:"user_id"`
	TokenHash string    `json:"-" bson:"token_hash"`
	ExpiresAt time.Time `json:"expires_at" bson:"expires_at"`
	Revoked   bool      `json:"revoked" bson:"revoked"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	User         User   `json:"user"`
}

// Host describes one Docker Engine managed by the ops platform.
type Host struct {
	ID         string    `json:"id" bson:"_id"`
	Name       string    `json:"name" bson:"name"`
	DockerHost string    `json:"docker_host" bson:"docker_host"`
	TLSCA      string    `json:"tls_ca" bson:"tls_ca"`
	TLSCert    string    `json:"tls_cert" bson:"tls_cert"`
	TLSKey     string    `json:"tls_key" bson:"tls_key"`
	Note       string    `json:"note" bson:"note"`
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" bson:"updated_at"`
}

// ServiceType describes a configurable Docker service and its CLI parameters.
type ServiceParam struct {
	Flag  string `json:"flag" bson:"flag"`
	Value string `json:"value" bson:"value"`
}

type ServiceType struct {
	ID        string         `json:"id" bson:"_id"`
	HostID    string         `json:"host_id" bson:"host_id"`
	Name      string         `json:"name" bson:"name"`
	Image     string         `json:"image" bson:"image"`
	Params    []ServiceParam `json:"params" bson:"params"`
	Note      string         `json:"note" bson:"note"`
	CreatedAt time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" bson:"updated_at"`
}
