/*
Copyright 2023 The KodeRover Authors.

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

package user

import (
	"github.com/gin-gonic/gin"
	userservice "github.com/koderover/zadig/pkg/microservice/user/core/service/user"
	internalhandler "github.com/koderover/zadig/pkg/shared/handler"
	"github.com/koderover/zadig/pkg/types"
)

func GetUserAuthInfo(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	uid := c.Query("uid")

	ctx.Resp, ctx.Err = userservice.GetUserAuthInfo(uid, ctx.Logger)
}

func CheckCollaborationModePermission(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	args := &types.CheckCollaborationModePermissionReq{}
	if err := c.ShouldBindQuery(args); err != nil {
		ctx.Err = err
		return
	}

	hasPermission, err := userservice.CheckCollaborationModePermission(args.UID, args.ProjectKey, args.Resource, args.ResourceName, args.Action)
	resp := &types.CheckCollaborationModePermissionResp{
		HasPermission: hasPermission,
	}
	if err != nil {
		resp.Error = err.Error()
	}

	ctx.Resp = resp
}

func ListAuthorizedProject(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	uid := c.Query("uid")
	if uid == "" {
		ctx.Resp = &types.ListAuthorizedProjectResp{ProjectList: []string{}}
		return
	}

	authorizedProject, err := userservice.ListAuthorizedProject(uid, ctx.Logger)
	if err != nil {
		ctx.Resp = &types.ListAuthorizedProjectResp{
			ProjectList: []string{},
			Error:       err.Error(),
		}
		return
	}
	resp := &types.ListAuthorizedProjectResp{
		ProjectList: authorizedProject,
	}
	if len(authorizedProject) == 0 {
		resp.Found = false
	}
}
