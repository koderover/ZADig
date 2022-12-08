/*
 * Copyright 2022 The KodeRover Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package mongodb

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	mongotool "github.com/koderover/zadig/pkg/tool/mongo"
)

type ExternalApprovalColl struct {
	*mongo.Collection

	coll string
}

func NewExternalApprovalColl() *ExternalApprovalColl {
	name := models.ExternalApproval{}.TableName()
	return &ExternalApprovalColl{
		Collection: mongotool.Database(config.MongoDatabase()).Collection(name),
		coll:       name,
	}
}

func (c *ExternalApprovalColl) GetCollectionName() string {
	return c.coll
}

func (c *ExternalApprovalColl) EnsureIndex(ctx context.Context) error {
	mod := []mongo.IndexModel{
		{
			Keys: bson.D{
				bson.E{Key: "app_id", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{
				bson.E{Key: "name", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
	}

	_, err := c.Indexes().CreateMany(ctx, mod)
	return err
}

func (c *ExternalApprovalColl) Create(ctx context.Context, args *models.ExternalApproval) (string, error) {
	if args == nil {
		return "", errors.New("approval is nil")
	}
	args.UpdateTime = time.Now().Unix()

	res, err := c.InsertOne(ctx, args)
	if err != nil {
		return "", err
	}
	return res.InsertedID.(primitive.ObjectID).Hex(), nil
}

func (c *ExternalApprovalColl) List(ctx context.Context, _type string) ([]*models.ExternalApproval, error) {
	query := bson.M{}
	resp := make([]*models.ExternalApproval, 0)

	if _type != "" {
		query["type"] = _type
	}
	cursor, err := c.Collection.Find(ctx, query)
	if err != nil {
		return nil, err
	}

	return resp, cursor.All(ctx, &resp)
}

func (c *ExternalApprovalColl) GetByID(ctx context.Context, idString string) (*models.ExternalApproval, error) {
	id, err := primitive.ObjectIDFromHex(idString)
	if err != nil {
		return nil, err
	}
	query := bson.M{"_id": id}

	resp := new(models.ExternalApproval)
	return resp, c.FindOne(ctx, query).Decode(resp)
}

func (c *ExternalApprovalColl) GetByAppID(ctx context.Context, appID string) (*models.ExternalApproval, error) {
	query := bson.M{"app_id": appID}

	resp := new(models.ExternalApproval)
	return resp, c.FindOne(ctx, query).Decode(resp)
}

func (c *ExternalApprovalColl) Update(ctx context.Context, idString string, arg *models.ExternalApproval) error {
	if arg == nil {
		return fmt.Errorf("nil app")
	}
	id, err := primitive.ObjectIDFromHex(idString)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	arg.UpdateTime = time.Now().Unix()
	filter := bson.M{"_id": id}
	update := bson.M{"$set": arg}

	_, err = c.UpdateOne(ctx, filter, update)
	return err
}

func (c *ExternalApprovalColl) DeleteByID(ctx context.Context, idString string) error {
	id, err := primitive.ObjectIDFromHex(idString)
	if err != nil {
		return err
	}
	query := bson.M{"_id": id}

	_, err = c.DeleteOne(ctx, query)
	return err
}
