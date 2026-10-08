package model

type CreateManagedUserRequest struct {
	Username    string   `json:"username" binding:"required"`
	Password    string   `json:"password" binding:"required"`
	Role        string   `json:"role"`
	ProjectIDs  []string `json:"project_ids"`
	Permissions []string `json:"permissions"`
	Disabled    bool     `json:"disabled"`
}

type UpdateManagedUserRequest struct {
	Username    string   `json:"username" binding:"required"`
	Role        string   `json:"role" binding:"required"`
	ProjectIDs  []string `json:"project_ids"`
	Permissions []string `json:"permissions"`
	Disabled    bool     `json:"disabled"`
}

type ResetManagedUserPasswordRequest struct {
	Password string `json:"password" binding:"required"`
}
