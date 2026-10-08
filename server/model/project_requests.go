package model

type UpdateProjectHostsRequest struct {
	HostIDs []string `json:"host_ids"`
}

type SetProjectHostsRequest struct {
	ProjectID string
	HostIDs   []string
}
