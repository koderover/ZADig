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
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	userservice "github.com/koderover/zadig/pkg/microservice/user/core/service/user"
	"github.com/koderover/zadig/pkg/shared/client/user"
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
		Found:       true,
	}
	if len(authorizedProject) == 0 {
		resp.Found = false
	}
	ctx.Resp = resp
}

func ListAuthorizedWorkflows(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	args := &types.ListAuthorizedWorkflowsReq{}
	if err := c.ShouldBindQuery(args); err != nil {
		ctx.Err = err
		return
	}

	authorizedWorkflow, authorizedWorkflowV4, err := userservice.ListAuthorizedWorkflow(args.UID, args.ProjectKey, ctx.Logger)
	if err != nil {
		ctx.Resp = &types.ListAuthorizedWorkflowsResp{
			WorkflowList:       []string{},
			CustomWorkflowList: []string{},
			Error:              err.Error(),
		}
		return
	}
	ctx.Resp = &types.ListAuthorizedWorkflowsResp{
		WorkflowList:       authorizedWorkflow,
		CustomWorkflowList: authorizedWorkflowV4,
		Error:              "",
	}
}

func GenerateUserAuthInfo(ctx *internalhandler.Context) error {
	resourceAuthInfo, err := userservice.GetUserAuthInfo(ctx.UserID, ctx.Logger)
	if err != nil {
		ctx.Logger.Errorf("Failed to generate user auth info for userID: %s, error is: %s", ctx.UserID, err)
		return err
	}
	authInfo := new(user.AuthorizedResources)
	bytes, err := json.Marshal(resourceAuthInfo)
	if err != nil {
		return fmt.Errorf("marshal auth info error: %s", err)
	}

	if err := json.Unmarshal(bytes, authInfo); err != nil {
		return fmt.Errorf("unmarshal auth info error: %s", err)
	}
	ctx.Resources = authInfo
	return nil
}
