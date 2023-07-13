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

package handler

import (
	"bytes"
	"io"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/koderover/zadig/pkg/microservice/policy/core/service"
	"github.com/koderover/zadig/pkg/setting"
	internalhandler "github.com/koderover/zadig/pkg/shared/handler"
	e "github.com/koderover/zadig/pkg/tool/errors"
	"github.com/koderover/zadig/pkg/tool/log"
)

type deleteRolesArgs struct {
	Names []string `json:"names"`
}

func CreateRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("CreateRole c.GetRawData() err : %v", err)
		ctx.Err = e.ErrInvalidParam.AddErr(err)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

	args := &service.Role{}
	if err := c.ShouldBindJSON(args); err != nil {
		ctx.Err = err
		return
	}

	projectName := c.Query("projectName")
	if projectName == "" {
		ctx.Err = e.ErrInvalidParam.AddDesc("projectName is empty")
		return
	}

	internalhandler.InsertDetailedOperationLog(c, ctx.UserName, projectName, setting.OperationSceneProject, "创建", "角色", "角色名称："+args.Name, string(data), ctx.Logger, args.Name)

	ctx.Err = service.CreateRole(projectName, args, ctx.Logger)
}

func UpdateRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("UpdateRole c.GetRawData() err : %v", err)
		ctx.Err = e.ErrInvalidParam.AddErr(err)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

	args := &service.Role{}
	if err := c.ShouldBindJSON(args); err != nil {
		ctx.Err = err
		return
	}

	projectName := c.Query("projectName")
	if projectName == "" {
		ctx.Err = e.ErrInvalidParam.AddDesc("projectName is empty")
		return
	}
	name := c.Param("name")
	args.Name = name

	internalhandler.InsertDetailedOperationLog(c, ctx.UserName, projectName, setting.OperationSceneProject, "更新", "角色", "角色名称："+args.Name, string(data), ctx.Logger, args.Name)

	ctx.Err = service.UpdateRole(projectName, args, ctx.Logger)
}

func UpdateOrCreateRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("UpdateOrCreateRole c.GetRawData() err : %v", err)
		ctx.Err = e.ErrInvalidParam.AddErr(err)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

	args := &service.Role{}
	if err := c.ShouldBindJSON(args); err != nil {
		ctx.Err = err
		return
	}

	projectName := c.Query("projectName")
	if projectName == "" {
		ctx.Err = e.ErrInvalidParam.AddDesc("projectName is empty")
		return
	}
	args.Name = c.Param("name")

	internalhandler.InsertDetailedOperationLog(c, ctx.UserName, projectName, setting.OperationSceneProject, "创建或更新", "角色", "角色名称："+args.Name, string(data), ctx.Logger, args.Name)

	ctx.Err = service.UpdateOrCreateRole(projectName, args, ctx.Logger)
}

func UpdatePresetRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("UpdatePresetRole c.GetRawData() err : %v", err)
		ctx.Err = e.ErrInvalidParam.AddErr(err)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

	args := &service.Role{}
	if err := c.ShouldBindJSON(args); err != nil {
		ctx.Err = err
		return
	}
	name := c.Param("name")
	args.Name = name

	internalhandler.InsertDetailedOperationLog(c, ctx.UserName, "", setting.OperationSceneSystem, "更新", "预设角色", "角色名称："+args.Name, string(data), ctx.Logger, args.Name)

	ctx.Err = service.UpdateRole(service.PresetScope, args, ctx.Logger)
}

func UpdateOrCreatePresetRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("UpdateOrCreatePresetRole c.GetRawData() err : %v", err)
		ctx.Err = e.ErrInvalidParam.AddErr(err)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

	args := &service.Role{}
	if err := c.ShouldBindJSON(args); err != nil {
		ctx.Err = err
		return
	}
	name := c.Param("name")
	args.Name = name

	internalhandler.InsertDetailedOperationLog(c, ctx.UserName, "", setting.OperationSceneSystem, "创建或更新", "预设角色", "角色名称："+args.Name, string(data), ctx.Logger, args.Name)

	ctx.Err = service.UpdateOrCreateRole(service.PresetScope, args, ctx.Logger)
}

func ListRoles(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	projectName := c.Query("projectName")
	if projectName == "" {
		ctx.Err = e.ErrInvalidParam.AddDesc("args projectName can't be empty")
		return
	}

	ctx.Resp, ctx.Err = service.ListRoles(projectName, ctx.Logger)
}

func GetRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	projectName := c.Query("projectName")
	if projectName == "" {
		ctx.Err = e.ErrInvalidParam.AddDesc("args projectName can't be empty")
		return
	}

	ctx.Resp, ctx.Err = service.GetRole(projectName, c.Param("name"), ctx.Logger)
}

func CreatePresetRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("CreatePresetRole c.GetRawData() err : %v", err)
		ctx.Err = e.ErrInvalidParam.AddErr(err)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

	args := &service.Role{}
	if err := c.ShouldBindJSON(args); err != nil {
		ctx.Err = err
		return
	}
	args.Type = setting.ResourceTypeSystem

	internalhandler.InsertDetailedOperationLog(c, ctx.UserName, "", setting.OperationSceneSystem, "创建", "预设角色", "角色名称："+args.Name, string(data), ctx.Logger, args.Name)

	ctx.Err = service.CreateRole(service.PresetScope, args, ctx.Logger)
}

func ListPresetRoles(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	ctx.Resp, ctx.Err = service.ListRoles(service.PresetScope, ctx.Logger)
	return
}

func GetPresetRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	ctx.Resp, ctx.Err = service.GetRole(service.PresetScope, c.Param("name"), ctx.Logger)
}

func DeleteRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	name := c.Param("name")
	projectName := c.Query("projectName")
	if projectName == "" {
		ctx.Err = e.ErrInvalidParam.AddDesc("args projectName can't be empty")
		return
	}

	internalhandler.InsertDetailedOperationLog(c, ctx.UserName, projectName, setting.OperationSceneProject, "删除", "角色", "角色名称："+name, "", ctx.Logger, name)

	ctx.Err = service.DeleteRole(name, projectName, ctx.Logger)
}

func DeleteRoles(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("CreatePresetRole c.GetRawData() err : %v", err)
		ctx.Err = e.ErrInvalidParam.AddErr(err)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

	projectName := c.Query("projectName")
	if projectName == "" {
		ctx.Err = e.ErrInvalidParam.AddDesc("args projectName can't be empty")
		return
	}

	args := &deleteRolesArgs{}
	if err := c.ShouldBindJSON(args); err != nil {
		ctx.Err = err
		return
	}

	internalhandler.InsertDetailedOperationLog(c, ctx.UserName, projectName, setting.OperationSceneProject, "删除", "角色", "角色名称："+strings.Join(args.Names, ","), string(data), ctx.Logger, args.Names...)

	ctx.Err = service.DeleteRoles(args.Names, projectName, ctx.Logger)
}

func DeletePresetRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()
	name := c.Param("name")

	internalhandler.InsertDetailedOperationLog(c, ctx.UserName, "", setting.OperationSceneProject, "删除", "预设角色", "", "角色名称："+name, ctx.Logger, name)

	ctx.Err = service.DeleteRole(name, service.PresetScope, ctx.Logger)
	return
}

func CreateSystemRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("CreateSystemRole c.GetRawData() err : %v", err)
		ctx.Err = e.ErrInvalidParam.AddErr(err)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

	args := &service.Role{}
	if err := c.ShouldBindJSON(args); err != nil {
		ctx.Err = err
		return
	}

	internalhandler.InsertDetailedOperationLog(c, ctx.UserName, "", setting.OperationSceneSystem, "创建", "系统角色", "角色名称："+args.Name, string(data), ctx.Logger, args.Name)

	ctx.Err = service.CreateRole(service.SystemScope, args, ctx.Logger)
}

func UpdateOrCreateSystemRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	data, err := c.GetRawData()
	if err != nil {
		log.Errorf("CreateSystemRole c.GetRawData() err : %v", err)
		ctx.Err = e.ErrInvalidParam.AddErr(err)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

	args := &service.Role{}
	if err := c.ShouldBindJSON(args); err != nil {
		ctx.Err = err
		return
	}
	name := c.Param("name")
	args.Name = name

	internalhandler.InsertDetailedOperationLog(c, ctx.UserName, "", setting.OperationSceneSystem, "创建或更新", "系统角色", "角色名称："+args.Name, string(data), ctx.Logger, args.Name)

	ctx.Err = service.UpdateOrCreateRole(service.SystemScope, args, ctx.Logger)
}

func ListSystemRoles(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()

	ctx.Resp, ctx.Err = service.ListRoles(service.SystemScope, ctx.Logger)
	return
}

func DeleteSystemRole(c *gin.Context) {
	ctx := internalhandler.NewContext(c)
	defer func() { internalhandler.JSONResponse(c, ctx) }()
	name := c.Param("name")

	internalhandler.InsertDetailedOperationLog(c, ctx.UserName, "", setting.OperationSceneSystem, "删除", "系统角色", "角色名称："+name, "", ctx.Logger, name)
	ctx.Err = service.DeleteRole(name, service.SystemScope, ctx.Logger)
	return
}
