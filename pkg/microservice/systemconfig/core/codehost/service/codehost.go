/*
Copyright 2021 The KodeRover Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"encoding/base64"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/koderover/zadig/pkg/microservice/systemconfig/core/codehost/repository/models"
	"github.com/koderover/zadig/pkg/microservice/systemconfig/core/codehost/repository/mongodb"
)

func CreateCodeHost(codehost *models.CodeHost, _ *zap.SugaredLogger) (*models.CodeHost, error) {
	if codehost.Type == "codehub" {
		codehost.IsReady = "2"
	}
	if codehost.Type == "gerrit" {
		codehost.IsReady = "2"
		codehost.AccessToken = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", codehost.Username, codehost.Password)))
	}
	codehost.CreatedAt = time.Now().Unix()
	codehost.UpdatedAt = time.Now().Unix()

	list, err := mongodb.NewCodehostColl().CodeHostList()
	if err != nil {
		return nil, err
	}
	codehost.ID = len(list) + 1
	return mongodb.NewCodehostColl().AddCodeHost(codehost)
}

func List(address, owner, source string, _ *zap.SugaredLogger) ([]*models.CodeHost, error) {
	return mongodb.NewCodehostColl().List(&mongodb.ListArgs{
		Address: address,
		Owner:   owner,
		Source:  source,
	})
}

func DeleteCodeHost(id int, _ *zap.SugaredLogger) error {
	return mongodb.NewCodehostColl().DeleteCodeHostByID(id)
}

func UpdateCodeHost(host *models.CodeHost, _ *zap.SugaredLogger) (*models.CodeHost, error) {
	return mongodb.NewCodehostColl().UpdateCodeHost(host)
}

func UpdateCodeHostByToken(host *models.CodeHost, _ *zap.SugaredLogger) (*models.CodeHost, error) {
	return mongodb.NewCodehostColl().UpdateCodeHostByToken(host)
}

func GetCodeHost(id int, _ *zap.SugaredLogger) (*models.CodeHost, error) {
	return mongodb.NewCodehostColl().GetCodeHostByID(id)
}
