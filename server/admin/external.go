package admin

import (
	"github.com/gogu-x/ops/admin/internal"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func NewAdminService(mongodbData *mongo.Database) *internal.AdminService {
	return internal.NewAdminService(mongodbData)
}
