/*
Copyright 2022 The KodeRover Authors.

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

package handler

import (
	"bytes"
	"encoding/json"
	"io/ioutil"

	"github.com/gin-gonic/gin"

	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/workflow/service/workflow"
	internalhandler "github.com/koderover/zadig/pkg/shared/handler"
	"github.com/koderover/zadig/pkg/tool/errors"
	"github.com/koderover/zadig/pkg/tool/log"
)

func ListPluginTemplates(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	ctx.Resp, ctx.Err = workflow.ListPluginTemplates(ctx.Logger)
}

func ListPluginRepositories(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	ctx.Resp, ctx.Err = workflow.ListPluginRepositories(ctx.Logger)
}

func DeletePluginRepo(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	ctx.Err = workflow.DeletePluginRepo(c.Param("id"), ctx.Logger)
}

func UpsertUserPluginRepository(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	req := &commonmodels.PluginRepo{}
	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("UpsertUserPluginRepository err: %s", err)
	}
	if err = json.Unmarshal(data, req); err != nil {
		log.Errorf("UpsertUserPluginRepository unmarshal json err: %s", err)
	}
	c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(data))

	if err := c.ShouldBindJSON(&req); err != nil {
		ctx.Err = errors.ErrInvalidParam.AddDesc(err.Error())
		return
	}

	go workflow.UpsertUserPluginRepository(req, ctx.Logger)
}
