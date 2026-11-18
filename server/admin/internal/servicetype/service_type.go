package servicetype

import (
	"time"

	"github.com/gogu-x/ops/model"
	"github.com/google/uuid"
)

func NewServiceType(name string) model.ServiceType {
	now := time.Now().UTC()
	return model.ServiceType{ID: uuid.NewString(), Name: name, CreatedAt: now, UpdatedAt: now}
}
